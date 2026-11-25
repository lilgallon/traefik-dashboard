package alert

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/i18n"
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
	lang      i18n.Lang

	mu sync.Mutex
	cd *cooldowns
}

func NewWatcher(n Notifier, statePath, publicURL string, lang i18n.Lang) *Watcher {
	return &Watcher{n: n, publicURL: strings.TrimRight(publicURL, "/"), lang: lang, cd: loadCooldowns(statePath)}
}

// alert envoie si le délai anti-répétition le permet. Un envoi raté n'est pas
// mémorisé : la prochaine occurrence réessaiera.
func (w *Watcher) alert(key string, at time.Time, every time.Duration, text func() string) {
	if !w.cd.ready(key, at, every) {
		return
	}
	if err := w.n.Send(context.Background(), text()); err != nil {
		log.Printf("alert %q not sent: %v", key, err)
		return
	}
	if err := w.cd.mark(key, at); err != nil {
		log.Printf("alert state not written: %v", err)
	}
}

func (w *Watcher) link(ip string) string {
	u := w.publicURL
	if ip != "" {
		u += "/#ip=" + ip
	}
	return fmt.Sprintf(`<a href="%s">%s</a>`, esc(u), i18n.T(w.lang, "tg.open"))
}

func whois(l i18n.Lang, id store.Identity) string {
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
		return i18n.T(l, "tg.whois_unknown")
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
		b.WriteString(i18n.T(w.lang, "tg.success_title") + "\n\n")
		fmt.Fprintf(&b, "<code>%s</code> · %s\n", esc(ip), whois(w.lang, id))
		if id.LooksLike != "" {
			b.WriteString(i18n.T(w.lang, "tg.looks_like", esc(id.LooksLike)) + "\n")
		}
		b.WriteString(requestLine(e) + "\n")
		if store.IsSensitive(e.Path) {
			b.WriteString(i18n.T(w.lang, "tg.sensitive") + "\n")
		}
		b.WriteString("\n" + i18n.T(w.lang, "tg.name_it") + "\n")
		b.WriteString(w.link(ip))
		return b.String()
	})
}
