package alert

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/geo"
	"github.com/lilgallon/traefik-dashboard/internal/logline"
	"github.com/lilgallon/traefik-dashboard/internal/store"
)

type fakeNotifier struct {
	mu   sync.Mutex
	sent []string
	err  error
}

func (f *fakeNotifier) Send(_ context.Context, html string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, html)
	return f.err
}

func (f *fakeNotifier) messages() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.sent...)
}

var now = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

const pageURL = "http://192.168.1.2:8080"

func live(at time.Time, ip, router, path string, status int) logline.Entry {
	return logline.Entry{Time: at, IP: netip.MustParseAddr(ip), Status: status, Router: router,
		Host: "home.example.com", Path: path, Method: "GET"}
}

var mobile = store.Identity{LooksLike: "Mon téléphone", Geo: geo.Info{Country: "FR", ASN: 51207, Org: "Free Mobile <SAS>"}}

func TestWatcherUnknownSuccessOncePer24h(t *testing.T) {
	state := filepath.Join(t.TempDir(), "alerts.json")
	n := &fakeNotifier{}
	w := NewWatcher(n, state, pageURL)

	w.Observe(live(now, "198.51.100.1", "homeassistant", "/api/states/<x>?a=1&b=2", 200), mobile)
	w.Observe(live(now.Add(time.Minute), "198.51.100.1", "homeassistant", "/api/config", 200), mobile)
	msgs := n.messages()
	if len(msgs) != 1 {
		t.Fatalf("%d envois, attendu 1 : %v", len(msgs), msgs)
	}
	m := msgs[0]
	for _, want := range []string{"198.51.100.1", "FR", "Free Mobile &lt;SAS&gt;", "AS51207", "Mon téléphone",
		"home.example.com/api/states/&lt;x&gt;?a=1&amp;b=2", "200", pageURL} {
		if !strings.Contains(m, want) {
			t.Errorf("message sans %q :\n%s", want, m)
		}
	}
	if strings.Contains(m, "<x>") {
		t.Errorf("chemin non échappé :\n%s", m)
	}

	// Une autre IP alerte de son côté.
	w.Observe(live(now, "198.51.100.2", "homeassistant", "/api/websocket", 101), store.Identity{})
	if got := len(n.messages()); got != 2 {
		t.Fatalf("autre IP : %d envois", got)
	}

	// Redémarrage : l'état sur disque évite de réalerter.
	n2 := &fakeNotifier{}
	w2 := NewWatcher(n2, state, pageURL)
	w2.Observe(live(now.Add(2*time.Hour), "198.51.100.1", "homeassistant", "/api/states/x", 200), mobile)
	if got := len(n2.messages()); got != 0 {
		t.Fatalf("après redémarrage : %d envois", got)
	}
	// 24 h plus tard, l'alerte repart.
	w2.Observe(live(now.Add(25*time.Hour), "198.51.100.1", "homeassistant", "/api/states/x", 200), mobile)
	if got := len(n2.messages()); got != 1 {
		t.Fatalf("après 25 h : %d envois", got)
	}
}

func TestWatcherIgnoresNamedAndAnonymous(t *testing.T) {
	n := &fakeNotifier{}
	w := NewWatcher(n, filepath.Join(t.TempDir(), "alerts.json"), pageURL)
	w.Observe(live(now, "203.0.113.10", "homeassistant", "/api/states/x", 200), store.Identity{Name: "Maison"})
	w.Observe(live(now, "198.51.100.1", "homeassistant", "/", 200), store.Identity{Anonymous: true})
	w.Observe(live(now, "198.51.100.1", "homeassistant", "/api/states/x", 401), store.Identity{})
	w.Observe(live(now, "198.51.100.1", "homeassistant", "/api/states/x", 302), store.Identity{})
	w.Observe(live(now, "198.51.100.1", "homeassistant", "/api/states/x", 0), store.Identity{})
	if msgs := n.messages(); len(msgs) != 0 {
		t.Fatalf("envois inattendus : %v", msgs)
	}
}

func TestWatcherSurvivesNotifierAndStateErrors(t *testing.T) {
	n := &fakeNotifier{err: errors.New("telegram injoignable")}
	// État dans un dossier inexistant : rien ne doit planter.
	w := NewWatcher(n, "/nexiste/pas/alerts.json", pageURL)
	w.Observe(live(now, "198.51.100.1", "homeassistant", "/api/states/x", 200), store.Identity{})
	if got := len(n.messages()); got != 1 {
		t.Fatalf("%d tentatives", got)
	}

	// État corrompu : repart de zéro.
	state := filepath.Join(t.TempDir(), "alerts.json")
	os.WriteFile(state, []byte("{pas du json"), 0o644)
	n2 := &fakeNotifier{}
	w2 := NewWatcher(n2, state, pageURL)
	w2.Observe(live(now, "198.51.100.1", "homeassistant", "/api/states/x", 200), store.Identity{})
	if got := len(n2.messages()); got != 1 {
		t.Fatalf("état corrompu : %d envois", got)
	}
}

func TestWatcherOnlyUnknownSuccess(t *testing.T) {
	n := &fakeNotifier{}
	w := NewWatcher(n, filepath.Join(t.TempDir(), "alerts.json"), pageURL)
	// Chemin sensible servi sur une URL anonyme, rafale de 404, pic de 5xx : rien.
	w.Observe(live(now, "198.51.100.1", "gotify", "/.env", 200), store.Identity{Anonymous: true})
	for i := 0; i < 500; i++ {
		w.Observe(live(now.Add(time.Duration(i)*time.Second), "198.51.100.2", "homeassistant", "/x", 404), store.Identity{})
		w.Observe(live(now.Add(time.Duration(i)*time.Second), "198.51.100.3", "homeassistant", "/x", 502), store.Identity{})
	}
	if got := n.messages(); len(got) != 0 {
		t.Fatalf("alertes inattendues : %q", got)
	}
}
