// traefik-dashboard : tableau de bord en direct des logs d'accès de Traefik.
//
// Relit 7 jours de access.log au démarrage, suit le fichier en direct, nomme les
// IP avec names.yaml, sert la page et alerte sur Telegram.
package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/alert"
	"github.com/lilgallon/traefik-dashboard/internal/api"
	"github.com/lilgallon/traefik-dashboard/internal/crowdsec"
	"github.com/lilgallon/traefik-dashboard/internal/geo"
	"github.com/lilgallon/traefik-dashboard/internal/logline"
	"github.com/lilgallon/traefik-dashboard/internal/names"
	"github.com/lilgallon/traefik-dashboard/internal/store"
	"github.com/lilgallon/traefik-dashboard/internal/tail"
	"github.com/lilgallon/traefik-dashboard/web"
)

// version est posée au build (-ldflags "-X main.version=…").
var version = "dev"

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// state regroupe ce que les boucles de fond mettent à jour et que l'API lit.
type state struct {
	mu          sync.RWMutex
	bans        map[netip.Addr]store.Ban
	bansErr     error
	listIssues  []string
	fixed       []string // avertissements posés une fois au démarrage
	backfillErr string
}

func (s *state) setBans(b map[netip.Addr]store.Ban, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// En erreur, on n'affiche plus de bans : un état périmé serait trompeur.
	s.bans, s.bansErr = b, err
}

func (s *state) getBans() (map[netip.Addr]store.Ban, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bans, s.bansErr
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	log.Printf("traefik-dashboard %s", version)

	var (
		listen       = env("LISTEN", ":8080")
		logDir       = env("LOG_DIR", "/logs")
		namesFile    = env("NAMES_FILE", "/config/names.yaml")
		csWhitelist  = os.Getenv("CROWDSEC_WHITELIST") // facultatif : contrôle de la liste « crowdsec »
		dynamicFile  = os.Getenv("TRAEFIK_DYNAMIC")    // facultatif : contrôle des middlewares ipAllowList
		geoCity      = env("GEO_CITY", "/geo/GeoLite2-City.mmdb")
		geoASN       = env("GEO_ASN", "/geo/GeoLite2-ASN.mmdb")
		stateDir     = env("STATE_DIR", "/state")
		lapiURL      = env("CROWDSEC_LAPI_URL", "http://crowdsec:8080")
		bouncerKey   = os.Getenv("CROWDSEC_BOUNCER_KEY")
		machineID    = env("CROWDSEC_MACHINE_ID", "traefik-dashboard")
		machinePass  = os.Getenv("CROWDSEC_MACHINE_PASSWORD")
		tgToken      = os.Getenv("TELEGRAM_BOT_TOKEN")
		tgChat       = os.Getenv("TELEGRAM_CHAT_ID")
		publicURL    = env("PUBLIC_URL", "http://192.168.1.2:8080")
		dailyAt      = env("DAILY_AT", "08:00")
		startedAt    = time.Now()
		st           = store.New()
		shared       = &state{}
		ready        atomic.Bool
		skippedLines atomic.Int64
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Géolocalisation ─────────────────────────────────────────────────────────
	var lookup geo.Lookuper = geo.None{}
	geoOK := true
	if db, err := geo.Open(geoCity, geoASN); err != nil {
		geoOK = false
		log.Printf("géolocalisation indisponible : %v", err)
		shared.fixed = append(shared.fixed, "Bases GeoLite2 illisibles : pays et opérateurs ne sont pas affichés, les règles par ASN ne s'appliquent pas, et les alertes Telegram sont suspendues pour ne pas alerter à tort.")
	} else {
		defer db.Close()
		lookup = db
	}

	// ── Règles de noms ──────────────────────────────────────────────────────────
	rules, err := names.NewReloader(namesFile)
	if err != nil {
		log.Printf("names.yaml : %v", err)
	}
	classify := func(e logline.Entry) store.Identity {
		g := lookup.Lookup(e.IP)
		r := rules.Rules()
		// Les règles voient le chemin normalisé, à l'ingestion comme au recalcul.
		e.Path = store.NormalizePath(e.Path)
		id := store.Identity{Geo: g, Anonymous: r.Anonymous(e)}
		if name, ok := r.Identify(e, g.ASN); ok {
			id.Name = name
		} else {
			id.LooksLike = r.LooksLike(e.IP, g.ASN)
		}
		return id
	}
	st.Enrich = func(ip netip.Addr) (geo.Info, string) {
		g := lookup.Lookup(ip)
		return g, rules.Rules().LooksLike(ip, g.ASN)
	}
	readSource := func(path string) names.Source {
		src := names.Source{Path: path}
		if path != "" {
			src.Data, src.Err = os.ReadFile(path)
		}
		return src
	}
	checkLists := func() {
		var issues []string
		if rules.Loaded() {
			issues = names.CheckLists(rules.Rules(), readSource(csWhitelist), readSource(dynamicFile))
		}
		shared.mu.Lock()
		shared.listIssues = issues
		shared.mu.Unlock()
	}
	checkLists()

	// ── Alertes ─────────────────────────────────────────────────────────────────
	var notifier alert.Notifier
	if tgToken != "" && tgChat != "" {
		notifier = alert.NewAsync(alert.NewTelegram(tgToken, tgChat), log.Printf)
	} else {
		notifier = alert.Log{Logf: log.Printf}
		shared.fixed = append(shared.fixed, "Telegram n'est pas configuré (TELEGRAM_BOT_TOKEN, TELEGRAM_CHAT_ID) : aucune alerte ne part.")
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		log.Printf("dossier d'état : %v", err)
	}
	watcher := alert.NewWatcher(notifier, filepath.Join(stateDir, "alerts.json"), publicURL)

	// ── CrowdSec : lecture des bans, et blocage depuis la page ──────────────────
	cs := crowdsec.NewClient(lapiURL, bouncerKey)
	refreshBans := func() {
		c, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		bans, err := cs.Bans(c)
		if _, prevErr := shared.getBans(); err != nil && prevErr == nil {
			log.Printf("CrowdSec : %v", err)
		}
		shared.setBans(bans, err)
	}
	var blocker api.Blocker
	csWriter := crowdsec.NewWriter(lapiURL, machineID, machinePass)
	feed := crowdsec.NewFeed(csWriter != nil)
	// Alertes : lecture complète au démarrage puis tous les quarts d'heure,
	// dernières minutes toutes les 30 s.
	var alertsLoaded atomic.Bool // une lecture complète a réussi
	refreshAlerts := func(full bool) {
		if csWriter == nil {
			return
		}
		full = full || !alertsLoaded.Load()
		since := 5 * time.Minute
		if full {
			since = crowdsec.Keep
		}
		c, cancel := context.WithTimeout(ctx, 45*time.Second)
		defer cancel()
		alerts, err := csWriter.Alerts(c, since)
		if err != nil {
			log.Printf("alertes CrowdSec : %v", err)
		}
		feed.SetAlerts(alerts, full, err, time.Now())
		if full && err == nil {
			alertsLoaded.Store(true)
		}
	}
	refreshBlocklist := func() {
		c, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		b, err := cs.Blocklist(c)
		if err != nil {
			log.Printf("blocklist CrowdSec : %v", err)
			return
		}
		feed.SetBlocklist(b)
	}
	if csWriter != nil {
		blocker = &banner{w: csWriter, refresh: func() {
			refreshBans()
			refreshAlerts(false)
		}}
	} else {
		log.Printf("blocage et alertes CrowdSec désactivés : CROWDSEC_MACHINE_PASSWORD absent")
	}
	protected := func(ip netip.Addr) string {
		switch {
		case !ip.IsGlobalUnicast() || ip.IsPrivate():
			return "Adresse privée ou réservée : un ban ne servirait à rien et pourrait couper le réseau local."
		}
		if rules.Loaded() {
			if name := rules.Rules().NamedIP(ip); name != "" {
				return fmt.Sprintf("IP nommée « %s » dans names.yaml : retirer d'abord la règle pour pouvoir la bannir.", name)
			}
		} else {
			return "names.yaml n'est pas chargé : impossible de vérifier que cette IP n'est pas de confiance."
		}
		return ""
	}
	var allowedHosts []string
	if u, err := url.Parse(publicURL); err == nil && u.Host != "" {
		allowedHosts = append(allowedHosts, u.Host)
	}
	for _, h := range strings.Split(os.Getenv("ALLOWED_HOSTS"), ",") {
		if h = strings.TrimSpace(h); h != "" {
			allowedHosts = append(allowedHosts, h)
		}
	}

	// ── API ─────────────────────────────────────────────────────────────────────
	static, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		log.Fatal(err)
	}
	srv := api.New(api.Config{
		Store:        st,
		Static:       static,
		Bans:         shared.getBans,
		Ready:        ready.Load,
		StartedAt:    startedAt,
		Blocker:      blocker,
		Protected:    protected,
		AllowedHosts: allowedHosts,
		CrowdSec: func(from, to time.Time) *crowdsec.View {
			bans, _ := shared.getBans()
			v := feed.ViewRange(from, to, bans, lookup.Lookup)
			return &v
		},
		IPAlerts: feed.ForIP,
		Warnings: func() []string {
			shared.mu.RLock()
			defer shared.mu.RUnlock()
			out := append([]string(nil), shared.fixed...)
			if err := rules.Err(); err != nil {
				msg := "names.yaml est invalide"
				if rules.Loaded() {
					msg += " : les règles précédentes restent appliquées"
				} else {
					msg += " : aucune IP n'est nommée, et les alertes de succès sont suspendues"
				}
				out = append(out, fmt.Sprintf("%s (%v).", msg, err))
			}
			for _, issue := range shared.listIssues {
				out = append(out, "Listes de confiance : "+issue+".")
			}
			if shared.backfillErr != "" {
				out = append(out, "Relecture incomplète des archives : "+shared.backfillErr)
			}
			return out
		},
	})
	defer srv.Close()
	httpSrv := &http.Server{
		Addr:              listen,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		// Pas de WriteTimeout : le flux SSE reste ouvert (échéance posée par écriture).
	}
	go func() {
		log.Printf("écoute sur %s", listen)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("serveur HTTP : %v", err)
		}
	}()

	var wg sync.WaitGroup
	every := func(d time.Duration, fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			t := time.NewTicker(d)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					fn()
				}
			}
		}()
	}

	// ── Relecture puis suivi en direct ──────────────────────────────────────────
	wg.Add(1)
	go func() {
		defer wg.Done()
		cutoff := startedAt.Add(-7*24*time.Hour - time.Hour)
		var kept int64
		offset, err := tail.Backfill(logDir, func(line []byte) {
			e, ok := logline.ParseLine(line)
			if !ok {
				skippedLines.Add(1)
				return
			}
			if e.Time.Before(cutoff) {
				return
			}
			kept++
			st.Add(e, classify(e))
		})
		if err != nil {
			log.Printf("relecture des archives : %v", err)
			shared.mu.Lock()
			shared.backfillErr = err.Error()
			shared.mu.Unlock()
		}
		// Les règles ont pu changer pendant la relecture.
		if changed, _ := rules.Poll(); changed {
			st.Reclassify(classify)
		}
		st.Prune(time.Now())
		ready.Store(true)
		runtime.GC()
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		stats := st.Stats()
		log.Printf("relecture terminée en %s : %d lignes gardées, %d illisibles, %d minutes, %d cellules, tas %d Mo",
			time.Since(startedAt).Round(time.Millisecond), kept, skippedLines.Load(), stats.Minutes, stats.Cells, ms.HeapAlloc>>20)

		err = tail.Follow(ctx, filepath.Join(logDir, "access.log"), offset, func(line []byte) {
			e, ok := logline.ParseLine(line)
			if !ok {
				skippedLines.Add(1)
				return
			}
			id := classify(e)
			st.Add(e, id)
			// Sans règles valides ou sans géolocalisation (règles par ASN), des IP
			// connues passeraient pour inconnues : ne pas alerter à tort.
			if rules.Loaded() && geoOK {
				watcher.Observe(e, id)
			}
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("suivi de access.log : %v", err)
		}
	}()

	// ── Boucles de fond ─────────────────────────────────────────────────────────
	every(10*time.Second, func() {
		changed, err := rules.Poll()
		if err != nil && changed {
			log.Printf("names.yaml : %v", err)
		}
		if changed {
			log.Printf("names.yaml rechargé : recalcul des verdicts")
			st.Reclassify(classify)
		}
		checkLists()
	})

	refreshBans()
	every(30*time.Second, refreshBans)
	wg.Add(1)
	go func() {
		defer wg.Done()
		refreshAlerts(true)
		refreshBlocklist()
	}()
	every(30*time.Second, func() { refreshAlerts(false) })
	every(15*time.Minute, func() { refreshAlerts(true) })
	every(30*time.Minute, refreshBlocklist)

	every(time.Minute, func() { st.Prune(time.Now()) })

	wg.Add(1)
	go func() {
		defer wg.Done()
		alert.RunDaily(ctx, dailyAt, func() string {
			return alert.DailyText(srv.Summary(store.W24h), publicURL)
		}, notifier)
	}()

	<-ctx.Done()
	log.Printf("arrêt")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Close()
	_ = httpSrv.Shutdown(shutdownCtx)
	wg.Wait()
}

// banner pose et lève les bans demandés depuis la page et rafraîchit aussitôt la
// liste des bans. Pas de notification : chaque action est journalisée avec l'IP
// du navigateur (docker logs traefik-dashboard) et visible dans le bloc CrowdSec.
type banner struct {
	w       *crowdsec.Writer
	refresh func()
}

func (b *banner) Ban(ctx context.Context, ip netip.Addr, d time.Duration, reason, by string) error {
	if err := b.w.Ban(ctx, ip, d, reason); err != nil {
		log.Printf("ban de %s refusé : %v", ip, err)
		return err
	}
	log.Printf("ban de %s pour %s demandé par %s (%s)", ip, d, by, reason)
	b.refresh()
	return nil
}

func (b *banner) Unban(ctx context.Context, ip netip.Addr, by string) (int, error) {
	n, err := b.w.Unban(ctx, ip)
	if err != nil {
		log.Printf("levée du ban de %s refusée : %v", ip, err)
		return 0, err
	}
	log.Printf("ban de %s levé (%d décisions) par %s", ip, n, by)
	b.refresh()
	return n, nil
}
