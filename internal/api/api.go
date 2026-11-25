// Package api sert le front embarqué, l'API JSON, le flux en direct (SSE) et
// l'endpoint de santé.
package api

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/lilgallon/traefik-dashboard/internal/crowdsec"
	"github.com/lilgallon/traefik-dashboard/internal/store"
)

const (
	staleAfter     = 15 * time.Minute // sans nouvelle ligne : données en retard, /health en erreur
	backfillBudget = 5 * time.Minute  // relecture du démarrage : au-delà, /health en erreur
)

type Config struct {
	Store *store.Store
	// Static : le front compilé (index.html à la racine).
	Static fs.FS
	// Bans : bans CrowdSec en cours (dernier état connu) ; erreur si injoignable.
	Bans func() (map[netip.Addr]store.Ban, error)
	// Warnings : avertissements à afficher en tête de page (écarts de listes,
	// names.yaml invalide, géolocalisation absente…).
	Warnings func() []string
	// Ready : la relecture des archives est terminée.
	Ready     func() bool
	Now       func() time.Time
	StartedAt time.Time
	TickEvery time.Duration // 1 s par défaut

	// Blocker pose et lève les bans demandés depuis la page ; nil = blocage
	// désactivé (aucun compte machine CrowdSec configuré).
	Blocker Blocker
	// Protected dit pourquoi une IP ne doit pas être bannie ("" = autorisé).
	Protected func(ip netip.Addr) string
	// CrowdSec : bloc CrowdSec des alertes de [from, to] (plus les bans en cours) ;
	// nil = absent.
	CrowdSec func(from, to time.Time) *crowdsec.View
	// IPAlerts : historique CrowdSec d'une IP ; nil = absent.
	IPAlerts func(ip netip.Addr) []crowdsec.Alert
	// AllowedHosts : en-têtes Host acceptés pour une écriture (parade au DNS
	// rebinding : une page tierce qui résoudrait son nom vers ce service).
	// Vide = tous.
	AllowedHosts []string
}

// Blocker est l'écriture des bans. by décrit l'auteur (IP du navigateur), pour
// l'alerte Telegram : la page n'a pas d'authentification.
type Blocker interface {
	Ban(ctx context.Context, ip netip.Addr, d time.Duration, reason, by string) error
	Unban(ctx context.Context, ip netip.Addr, by string) (int, error)
}

// BanDurations : durées proposées par la page, seules acceptées.
var BanDurations = map[string]time.Duration{
	"4h":  4 * time.Hour,
	"24h": 24 * time.Hour,
	"7d":  7 * 24 * time.Hour,
	"30d": 30 * 24 * time.Hour,
}

// WriteHeader est l'en-tête que la page envoie avec chaque écriture. Une page
// d'un autre site ne peut pas le poser sans requête préalable CORS, que ce
// service ne valide jamais : c'est la parade aux requêtes forgées (CSRF).
const WriteHeader = "X-Traefik-Dashboard"

type Server struct {
	cfg  Config
	mux  *http.ServeMux
	stop chan struct{}
	once sync.Once

	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func New(cfg Config) *Server {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.TickEvery == 0 {
		cfg.TickEvery = time.Second
	}
	if cfg.Ready == nil {
		cfg.Ready = func() bool { return true }
	}
	if cfg.Warnings == nil {
		cfg.Warnings = func() []string { return nil }
	}
	if cfg.StartedAt.IsZero() {
		cfg.StartedAt = cfg.Now()
	}
	s := &Server{cfg: cfg, mux: http.NewServeMux(), stop: make(chan struct{}), subs: map[chan []byte]struct{}{}}
	s.mux.HandleFunc("GET /api/summary", s.handleSummary)
	s.mux.HandleFunc("GET /api/ip/{ip}", s.handleIP)
	s.mux.HandleFunc("GET /api/stream", s.handleStream)
	s.mux.HandleFunc("POST /api/ip/{ip}/ban", s.handleBan)
	s.mux.HandleFunc("DELETE /api/ip/{ip}/ban", s.handleUnban)
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		if !readOnly(w, r) {
			return
		}
		writeError(w, http.StatusNotFound, "route inconnue")
	})
	s.mux.HandleFunc("/", s.handleStatic)
	go s.broadcast()
	return s
}

// Close arrête le flux en direct.
func (s *Server) Close() { s.once.Do(func() { close(s.stop) }) }

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	// Tout est servi par ce binaire : aucune ressource externe n'est permise.
	h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	s.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")
		w.WriteHeader(status)
		zw, _ := gzip.NewWriterLevel(w, gzip.BestSpeed)
		_ = json.NewEncoder(zw).Encode(v)
		_ = zw.Close()
		return
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// period est ce que demande la page : une période glissante (?window=24h) ou un
// intervalle choisi sur le graphique (?from=…&to=…, secondes Unix).
type period struct {
	win store.Window
	rng *store.Range
}

const periodError = "période inconnue : window=1h, 24h ou 7d, ou from et to (secondes Unix)"

func parsePeriod(r *http.Request) (period, bool) {
	q := r.URL.Query()
	if q.Has("from") || q.Has("to") {
		from, err1 := strconv.ParseInt(q.Get("from"), 10, 64)
		to, err2 := strconv.ParseInt(q.Get("to"), 10, 64)
		if err1 != nil || err2 != nil || to <= from {
			return period{}, false
		}
		return period{rng: &store.Range{From: from, To: to}}, true
	}
	if !q.Has("window") {
		return period{win: store.W24h}, true
	}
	w, ok := store.ParseWindow(q.Get("window"))
	return period{win: w}, ok
}

// Summary assemble le résumé d'une période : agrégats du Store, bans CrowdSec et
// avertissements. Sert aussi au résumé quotidien.
func (s *Server) Summary(w store.Window) store.Summary {
	now := s.cfg.Now()
	return s.decorate(s.cfg.Store.Summary(w, now), now)
}

func (s *Server) decorate(sum store.Summary, now time.Time) store.Summary {
	sum.Loading = !s.cfg.Ready()

	var bans map[netip.Addr]store.Ban
	var bansErr error
	if s.cfg.Bans != nil {
		bans, bansErr = s.cfg.Bans()
	}
	if s.cfg.Bans == nil || bansErr != nil {
		sum.KPIs.ActiveBans = -1
		sum.Warnings = append(sum.Warnings, "CrowdSec injoignable : les bans en cours ne sont pas affichés.")
	} else {
		sum.KPIs.ActiveBans = len(bans)
		for i := range sum.IPs {
			if ip, err := netip.ParseAddr(sum.IPs[i].IP); err == nil {
				if b, ok := bans[ip]; ok {
					sum.IPs[i].Ban = &b
				}
			}
		}
	}

	sum.Warnings = append(sum.Warnings, s.cfg.Warnings()...)
	if !sum.Loading {
		if sum.LastLine.IsZero() {
			sum.Warnings = append(sum.Warnings, "Aucune ligne lue dans access.log : rien n'est surveillé.")
		} else if gap := now.Sub(sum.LastLine); gap > staleAfter {
			sum.Warnings = append(sum.Warnings, fmt.Sprintf(
				"Données en retard : la dernière ligne lue date d'il y a %d min. Le vert ci-dessous ne prouve rien.", int(gap/time.Minute)))
		}
	}
	return sum
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	p, ok := parsePeriod(r)
	if !ok {
		writeError(w, http.StatusBadRequest, periodError)
		return
	}
	now := s.cfg.Now()
	var resp summaryResponse
	csFrom, csTo := now.Add(-p.win.Duration()), now.Add(time.Minute)
	if p.rng != nil {
		sum, err := s.cfg.Store.SummaryRange(*p.rng, now)
		if err != nil {
			writeError(w, http.StatusBadRequest, "intervalle vide ou hors des 7 derniers jours")
			return
		}
		resp.Summary = s.decorate(sum, now)
		csFrom, csTo = time.Unix(sum.Range.From, 0), time.Unix(sum.Range.To, 0)
	} else {
		resp.Summary = s.decorate(s.cfg.Store.Summary(p.win, now), now)
	}
	if s.cfg.CrowdSec != nil {
		resp.CrowdSec = s.cfg.CrowdSec(csFrom, csTo)
	}
	writeJSON(w, r, http.StatusOK, resp)
}

type summaryResponse struct {
	store.Summary
	CrowdSec *crowdsec.View `json:"crowdsec"`
}

func (s *Server) handleIP(w http.ResponseWriter, r *http.Request) {
	p, ok := parsePeriod(r)
	if !ok {
		writeError(w, http.StatusBadRequest, periodError)
		return
	}
	ip, err := netip.ParseAddr(r.PathValue("ip"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "adresse IP invalide")
		return
	}
	ip = ip.Unmap()
	var d store.IPDetail
	var found bool
	if p.rng != nil {
		var err error
		if d, found, err = s.cfg.Store.IPDetailRange(ip, *p.rng, s.cfg.Now()); err != nil {
			writeError(w, http.StatusBadRequest, "intervalle vide ou hors des 7 derniers jours")
			return
		}
	} else {
		d, found = s.cfg.Store.IPDetail(ip, p.win, s.cfg.Now())
	}
	if !found {
		writeError(w, http.StatusNotFound, "aucune requête de cette IP sur la période")
		return
	}
	if s.cfg.Bans != nil {
		if bans, err := s.cfg.Bans(); err == nil {
			if b, ok := bans[ip]; ok {
				d.Ban = &b
			}
		}
	}
	resp := ipResponse{IPDetail: d, Block: s.blockState(ip), Alerts: []crowdsec.Alert{}}
	if s.cfg.IPAlerts != nil {
		resp.Alerts = s.cfg.IPAlerts(ip)
	}
	writeJSON(w, r, http.StatusOK, resp)
}

type ipResponse struct {
	store.IPDetail
	Block  blockState       `json:"block"`
	Alerts []crowdsec.Alert `json:"alerts"` // historique CrowdSec (7 jours)
}

// blockState dit à la page si elle peut proposer le blocage de cette IP.
type blockState struct {
	Enabled bool   `json:"enabled"`           // compte machine CrowdSec configuré
	Refused string `json:"refused,omitempty"` // pourquoi cette IP ne peut pas être bannie
}

func (s *Server) blockState(ip netip.Addr) blockState {
	b := blockState{Enabled: s.cfg.Blocker != nil}
	if s.cfg.Protected != nil {
		b.Refused = s.cfg.Protected(ip)
	}
	return b
}

// writeAllowed vérifie qu'une écriture vient bien de la page elle-même.
func (s *Server) writeAllowed(w http.ResponseWriter, r *http.Request) bool {
	if len(s.cfg.AllowedHosts) > 0 && !slices.Contains(s.cfg.AllowedHosts, r.Host) {
		writeError(w, http.StatusForbidden, "hôte non autorisé : ouvrir la page par son adresse habituelle")
		return false
	}
	if r.Header.Get(WriteHeader) == "" {
		writeError(w, http.StatusForbidden, "en-tête "+WriteHeader+" absent")
		return false
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" {
		writeError(w, http.StatusForbidden, "requête d'un autre site refusée")
		return false
	}
	if o := r.Header.Get("Origin"); o != "" && o != "http://"+r.Host && o != "https://"+r.Host {
		writeError(w, http.StatusForbidden, "origine refusée")
		return false
	}
	if s.cfg.Blocker == nil {
		writeError(w, http.StatusServiceUnavailable, "blocage désactivé : aucun compte machine CrowdSec configuré")
		return false
	}
	return true
}

func clientIP(r *http.Request) string {
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}

// cleanReason garde un motif lisible : une ligne, sans caractères de contrôle,
// 120 caractères au plus.
func cleanReason(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 120 {
		s = string(r[:120])
	}
	return s
}

func (s *Server) handleBan(w http.ResponseWriter, r *http.Request) {
	if !s.writeAllowed(w, r) {
		return
	}
	ip, err := netip.ParseAddr(r.PathValue("ip"))
	if err != nil || ip.Zone() != "" {
		writeError(w, http.StatusBadRequest, "adresse IP invalide")
		return
	}
	ip = ip.Unmap()
	if why := s.blockState(ip).Refused; why != "" {
		writeError(w, http.StatusConflict, why)
		return
	}
	var req struct {
		Duration string `json:"duration"`
		Reason   string `json:"reason"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "corps JSON attendu : {\"duration\": \"24h\", \"reason\": \"…\"}")
		return
	}
	d, ok := BanDurations[req.Duration]
	if !ok {
		writeError(w, http.StatusBadRequest, "durée inconnue : 4h, 24h, 7d ou 30d")
		return
	}
	reason := "manuel (tableau de bord)"
	if m := cleanReason(req.Reason); m != "" {
		reason += " : " + m
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.cfg.Blocker.Ban(ctx, ip, d, reason, clientIP(r)); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"ip": ip.String(), "until": s.cfg.Now().Add(d).UTC().Truncate(time.Second)})
}

func (s *Server) handleUnban(w http.ResponseWriter, r *http.Request) {
	if !s.writeAllowed(w, r) {
		return
	}
	ip, err := netip.ParseAddr(r.PathValue("ip"))
	if err != nil || ip.Zone() != "" {
		writeError(w, http.StatusBadRequest, "adresse IP invalide")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	n, err := s.cfg.Blocker.Unban(ctx, ip.Unmap(), clientIP(r))
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"ip": ip.Unmap().String(), "deleted": n})
}

// tick est l'événement envoyé chaque seconde sur le flux.
type tick struct {
	store.LiveTick
	Red     map[store.Window]bool `json:"red"` // verdict par période
	Loading bool                  `json:"loading"`
	Hits    []store.Hit           `json:"hits,omitempty"` // requêtes lues depuis le tick précédent
}

const (
	// Au plus tant de requêtes par tick : au-delà (rafale, relecture du démarrage),
	// seules les plus récentes partent.
	hitsPerTick = 40
	// Envoyées à la connexion, pour que le bloc « en direct » ne démarre pas vide.
	hitsOnConnect = 200
)

func (s *Server) broadcast() {
	t := time.NewTicker(s.cfg.TickEvery)
	defer t.Stop()
	var lastSeq uint64
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
		}
		s.mu.Lock()
		n := len(s.subs)
		s.mu.Unlock()
		if n == 0 {
			continue
		}
		now := s.cfg.Now()
		ev := tick{LiveTick: s.cfg.Store.Second(now), Red: make(map[store.Window]bool, 3), Loading: !s.cfg.Ready()}
		for _, w := range store.Windows {
			ev.Red[w] = s.cfg.Store.Red(w, now)
		}
		ev.Hits = s.cfg.Store.Recent(lastSeq, hitsPerTick)
		if n := len(ev.Hits); n > 0 {
			lastSeq = ev.Hits[n-1].Seq
		}
		data, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		msg := append(append([]byte("event: tick\ndata: "), data...), '\n', '\n')
		s.mu.Lock()
		for ch := range s.subs {
			select {
			case ch <- msg:
			default: // client trop lent : il rattrapera avec /api/summary
			}
		}
		s.mu.Unlock()
	}
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	// Le navigateur se reconnecte seul après 2 s.
	fmt.Fprint(w, "retry: 2000\n\n")
	if err := rc.Flush(); err != nil {
		return
	}

	ch := make(chan []byte, 8)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.subs, ch)
		s.mu.Unlock()
	}()

	// Inscrit avant de lire l'instantané : aucune requête ne tombe entre les deux
	// (un doublon éventuel est écarté par son numéro côté page).
	if data, err := json.Marshal(s.cfg.Store.Recent(0, hitsOnConnect)); err == nil {
		if _, err := fmt.Fprintf(w, "event: hits\ndata: %s\n\n", data); err != nil {
			return
		}
		if err := rc.Flush(); err != nil {
			return
		}
	}

	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.stop:
			return
		case msg := <-ch:
			_ = rc.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := w.Write(msg); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	now := s.cfg.Now()
	last := s.cfg.Store.LastLine()
	body := map[string]any{"status": "ok", "lastLine": nil}
	if !last.IsZero() {
		body["lastLine"] = last.UTC().Format(time.RFC3339)
	}
	status := http.StatusOK
	switch {
	case !s.cfg.Ready():
		body["status"] = "relecture des archives en cours"
		if now.Sub(s.cfg.StartedAt) > backfillBudget {
			status = http.StatusServiceUnavailable
		}
	case last.IsZero():
		body["status"] = "aucune ligne lue dans access.log"
		status = http.StatusServiceUnavailable
	case now.Sub(last) >= staleAfter:
		body["status"] = fmt.Sprintf("dernière ligne lue il y a %d min", int(now.Sub(last)/time.Minute))
		status = http.StatusServiceUnavailable
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// readOnly refuse tout ce qui n'est pas une lecture (seuls les bans s'écrivent,
// par leurs routes dédiées).
func readOnly(w http.ResponseWriter, r *http.Request) bool {
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		return true
	}
	w.Header().Set("Allow", "GET, HEAD")
	writeError(w, http.StatusMethodNotAllowed, "lecture seule")
	return false
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if !readOnly(w, r) {
		return
	}
	if s.cfg.Static == nil {
		http.NotFound(w, r)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	if st, err := fs.Stat(s.cfg.Static, name); err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	if strings.HasPrefix(name, "assets/") {
		// Noms de fichiers hachés par Vite.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFileFS(w, r, s.cfg.Static, name)
}
