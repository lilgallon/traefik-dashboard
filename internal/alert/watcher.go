package alert

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/logline"
	"github.com/lilgallon/traefik-dashboard/internal/store"
)

// Seule alerte en direct : le succès d'une IP inconnue (hors URL anonymes).
// Le reste (bans, WAF, volume, 5xx…) se consulte dans la page et dans le résumé
// quotidien, sans notification.
const successEvery = 24 * time.Hour // 1 alerte par IP

// Watcher décide quand alerter. Sûr pour un usage concurrent.
type Watcher struct {
	n         Notifier
	publicURL string

	mu sync.Mutex
	cd *cooldowns
}

func NewWatcher(n Notifier, statePath, publicURL string) *Watcher {
	return &Watcher{n: n, publicURL: strings.TrimRight(publicURL, "/"), cd: loadCooldowns(statePath)}
}

// alert envoie si le délai anti-répétition le permet. Un envoi raté n'est pas
// mémorisé : la prochaine occurrence réessaiera.
func (w *Watcher) alert(key string, at time.Time, every time.Duration, text func() string) {
	if !w.cd.ready(key, at, every) {
		return
	}
	if err := w.n.Send(context.Background(), text()); err != nil {
		log.Printf("alerte « %s » non envoyée : %v", key, err)
		return
	}
	if err := w.cd.mark(key, at); err != nil {
		log.Printf("état des alertes non écrit : %v", err)
	}
}

func (w *Watcher) link(ip string) string {
	u := w.publicURL
	if ip != "" {
		u += "/#ip=" + ip
	}
	return fmt.Sprintf(`<a href="%s">Ouvrir le tableau de bord</a>`, esc(u))
}

func whois(id store.Identity) string {
	var parts []string
	if id.Geo.Country != "" {
		parts = append(parts, esc(id.Geo.Country))
	}
	if id.Geo.Org != "" || id.Geo.ASN != 0 {
		org := esc(id.Geo.Org)
		if id.Geo.ASN != 0 {
			org = strings.TrimSpace(fmt.Sprintf("%s (AS%d)", org, id.Geo.ASN))
		}
		parts = append(parts, org)
	}
	if len(parts) == 0 {
		return "pays et opérateur inconnus"
	}
	return strings.Join(parts, " · ")
}

func requestLine(e logline.Entry) string {
	path := e.Path
	if len(path) > 200 {
		path = path[:200] + "…"
	}
	return fmt.Sprintf("%s <code>%s</code> → <b>%d</b>", esc(e.Method), esc(e.Host+path), e.Status)
}

// Observe examine une ligne lue EN DIRECT (jamais celles de la relecture du
// démarrage).
func (w *Watcher) Observe(e logline.Entry, id store.Identity) {
	if id.Name != "" || id.Anonymous || !e.Success() {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	ip := e.IP.String()
	w.alert("success:"+ip, e.Time, successEvery, func() string {
		var b strings.Builder
		b.WriteString("🔴 <b>Succès d'une IP inconnue</b>\n\n")
		fmt.Fprintf(&b, "<code>%s</code> · %s\n", esc(ip), whois(id))
		if id.LooksLike != "" {
			fmt.Fprintf(&b, "Ressemble à : %s (hors des routers ou chemins de sa règle)\n", esc(id.LooksLike))
		}
		b.WriteString(requestLine(e) + "\n")
		if store.IsSensitive(e.Path) {
			b.WriteString("⚠️ Chemin recherché par les scanners.\n")
		}
		b.WriteString("\nSi c'est légitime, nommer cette IP dans <code>names.yaml</code>.\n")
		b.WriteString(w.link(ip))
		return b.String()
	})
}
