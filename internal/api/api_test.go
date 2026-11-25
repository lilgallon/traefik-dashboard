package api

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/i18n"
	"github.com/lilgallon/traefik-dashboard/internal/logline"
	"github.com/lilgallon/traefik-dashboard/internal/store"
)

var now = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func entry(at time.Time, ip, path string, status int) logline.Entry {
	return logline.Entry{Time: at, IP: netip.MustParseAddr(ip), Status: status, Router: "homeassistant",
		Host: "home.example.com", Path: path, Method: "GET", BytesOut: 10}
}

func newServer(t *testing.T, mod func(*Config)) (*Server, *store.Store) {
	t.Helper()
	st := store.New()
	st.Add(entry(now.Add(-time.Minute), "198.51.100.1", "/api/states/x", 200), store.Identity{})
	st.Add(entry(now.Add(-time.Minute), "203.0.113.10", "/api/states/x", 200), store.Identity{Name: "Maison"})
	cfg := Config{
		Store: st,
		Now:   func() time.Time { return now },
		Ready: func() bool { return true },
		Bans: func() (map[netip.Addr]store.Ban, error) {
			return map[netip.Addr]store.Ban{
				netip.MustParseAddr("198.51.100.1"): {Scenario: "crowdsecurity/http-probing"},
				netip.MustParseAddr("203.0.113.1"):  {Scenario: "autre"},
			}, nil
		},
		Warnings: func() []i18n.Msg {
			return []i18n.Msg{i18n.M("warn.lists", i18n.M("lists.missing_in_names", "1.2.3.4/32", "dynamic.yml", "app-allowlist"))}
		},
		Static: fstest.MapFS{
			"index.html":    {Data: []byte("<html>tableau</html>")},
			"assets/app.js": {Data: []byte("console.log(1)")},
		},
		StartedAt: now.Add(-time.Hour),
	}
	if mod != nil {
		mod(&cfg)
	}
	s := New(cfg)
	t.Cleanup(s.Close)
	return s, st
}

func get(t *testing.T, s *Server, url string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, url, nil)
	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestSummaryEndpoint(t *testing.T) {
	s, _ := newServer(t, nil)
	rec := get(t, s, "/api/summary?window=24h")
	if rec.Code != 200 {
		t.Fatalf("HTTP %d : %s", rec.Code, rec.Body)
	}
	var got struct {
		Window  string `json:"window"`
		Verdict struct {
			Red       bool `json:"red"`
			Offenders []struct {
				IP string `json:"ip"`
			} `json:"offenders"`
		} `json:"verdict"`
		KPIs struct {
			ActiveBans int              `json:"activeBans"`
			ByClass    map[string]int64 `json:"byClass"`
		} `json:"kpis"`
		IPs []struct {
			IP, Name, Verdict string
			Ban               *struct{ Scenario string }
		} `json:"ips"`
		Warnings []string `json:"warnings"`
		Loading  bool     `json:"loading"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Window != "24h" || !got.Verdict.Red || len(got.Verdict.Offenders) != 1 {
		t.Errorf("verdict : %+v", got.Verdict)
	}
	if got.IPs[0].Verdict != "known" || got.IPs[0].Name != "Maison" || got.IPs[0].Ban != nil {
		t.Errorf("première ligne : %+v", got.IPs[0])
	}
	if got.IPs[1].Verdict != "success" || got.IPs[1].Ban == nil || got.IPs[1].Ban.Scenario != "crowdsecurity/http-probing" {
		t.Errorf("deuxième ligne : %+v", got.IPs[1])
	}
	if got.KPIs.ActiveBans != 2 || got.KPIs.ByClass["2xx"] != 2 {
		t.Errorf("indicateurs : %+v", got.KPIs)
	}
	if len(got.Warnings) != 1 || got.Loading {
		t.Errorf("avertissements : %v, loading %v", got.Warnings, got.Loading)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q", cc)
	}

	// Période par défaut : 24 h.
	if rec := get(t, s, "/api/summary"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `"window":"24h"`) {
		t.Errorf("sans paramètre : HTTP %d", rec.Code)
	}
}

func TestSummaryGzip(t *testing.T) {
	s, _ := newServer(t, nil)
	rec := get(t, s, "/api/summary?window=1h", "Accept-Encoding", "gzip, br")
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatal("réponse non compressée")
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(zr)
	if !json.Valid(b) {
		t.Fatal("JSON invalide après décompression")
	}
}

func TestSummaryBadWindow(t *testing.T) {
	s, _ := newServer(t, nil)
	for _, w := range []string{"2h", "", "7d%20"} {
		if rec := get(t, s, "/api/summary?window="+w); rec.Code != 400 {
			t.Errorf("window=%q : HTTP %d", w, rec.Code)
		}
	}
}

func TestSummaryDegraded(t *testing.T) {
	// CrowdSec injoignable, relecture en cours, données en retard : la page répond
	// quand même, avec des avertissements.
	s, _ := newServer(t, func(c *Config) {
		c.Bans = func() (map[netip.Addr]store.Ban, error) { return nil, errors.New("connexion refusée") }
		c.Ready = func() bool { return false }
		c.Now = func() time.Time { return now.Add(20 * time.Minute) }
	})
	rec := get(t, s, "/api/summary?window=1h")
	if rec.Code != 200 {
		t.Fatalf("HTTP %d", rec.Code)
	}
	var got struct {
		KPIs     struct{ ActiveBans int }
		Warnings []string
		Loading  bool
		IPs      []struct{ Ban *struct{} }
	}
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.KPIs.ActiveBans != -1 || !got.Loading {
		t.Errorf("bans=%d loading=%v", got.KPIs.ActiveBans, got.Loading)
	}
	joined := strings.Join(got.Warnings, "\n")
	if !strings.Contains(joined, "CrowdSec") {
		t.Errorf("avertissement CrowdSec absent : %v", got.Warnings)
	}
	// Pendant la relecture, pas d'avertissement « données en retard » (normal).
	if strings.Contains(joined, "late") {
		t.Errorf("avertissement de retard pendant la relecture : %v", got.Warnings)
	}

	s, _ = newServer(t, func(c *Config) { c.Now = func() time.Time { return now.Add(20 * time.Minute) } })
	rec = get(t, s, "/api/summary?window=1h")
	if !strings.Contains(rec.Body.String(), "Data is late") {
		t.Errorf("avertissement de retard absent : %s", rec.Body)
	}
}

// La langue suit Accept-Language, puis la langue du service.
func TestLanguage(t *testing.T) {
	s, _ := newServer(t, nil)
	var got struct{ Warnings []string }
	json.Unmarshal(get(t, s, "/api/summary?window=1h").Body.Bytes(), &got)
	if len(got.Warnings) != 1 || got.Warnings[0] != "Trust lists: 1.2.3.4/32 is in dynamic.yml but not in names.yaml (app-allowlist)." {
		t.Errorf("en : %q", got.Warnings)
	}
	json.Unmarshal(get(t, s, "/api/summary?window=1h", "Accept-Language", "fr-FR,fr;q=0.9").Body.Bytes(), &got)
	if len(got.Warnings) != 1 || !strings.HasPrefix(got.Warnings[0], "Listes de confiance : 1.2.3.4/32 est dans dynamic.yml") {
		t.Errorf("fr : %q", got.Warnings)
	}
	if rec := get(t, s, "/api/summary?window=2h", "Accept-Language", "fr"); !strings.Contains(rec.Body.String(), "période inconnue") {
		t.Errorf("erreur fr : %s", rec.Body)
	}

	s, _ = newServer(t, func(c *Config) { c.Lang = i18n.FR })
	if rec := get(t, s, "/api/summary?window=2h", "Accept-Language", "de"); !strings.Contains(rec.Body.String(), "période inconnue") {
		t.Errorf("langue du service : %s", rec.Body)
	}
}

func TestIPEndpoint(t *testing.T) {
	s, _ := newServer(t, nil)
	rec := get(t, s, "/api/ip/198.51.100.1?window=1h")
	if rec.Code != 200 {
		t.Fatalf("HTTP %d : %s", rec.Code, rec.Body)
	}
	var got struct {
		IP, Verdict string
		Ban         *struct{ Scenario string }
		URLs        []struct{ Host, Path string }
		Routers     map[string]int64
	}
	json.Unmarshal(rec.Body.Bytes(), &got)
	if got.IP != "198.51.100.1" || got.Verdict != "success" || got.Ban == nil || len(got.URLs) != 1 || got.Routers["homeassistant"] != 1 {
		t.Errorf("détail : %+v", got)
	}
	if rec := get(t, s, "/api/ip/203.0.113.99?window=1h"); rec.Code != 404 {
		t.Errorf("IP inconnue : HTTP %d", rec.Code)
	}
	if rec := get(t, s, "/api/ip/pas-une-ip"); rec.Code != 400 {
		t.Errorf("IP invalide : HTTP %d", rec.Code)
	}
	if rec := get(t, s, "/api/ip/198.51.100.1?window=x"); rec.Code != 400 {
		t.Errorf("période invalide : HTTP %d", rec.Code)
	}
}

func TestStreamSendsTicks(t *testing.T) {
	s, _ := newServer(t, func(c *Config) { c.TickEvery = 50 * time.Millisecond })
	srv := httptest.NewServer(s)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q", ct)
	}
	type result struct {
		ticks int
		last  string
		hits  string
	}
	done := make(chan result, 1)
	go func() {
		var r result
		sc := bufio.NewScanner(resp.Body)
		event := ""
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "event: "):
				event = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: ") && event == "hits":
				r.hits = strings.TrimPrefix(line, "data: ")
			case strings.HasPrefix(line, "data: ") && event == "tick":
				r.ticks++
				r.last = strings.TrimPrefix(line, "data: ")
				if r.ticks == 2 {
					done <- r
					return
				}
			}
		}
		done <- r
	}()
	select {
	case r := <-done:
		if r.ticks != 2 {
			t.Fatalf("%d ticks reçus", r.ticks)
		}
		var tick struct {
			T       int64            `json:"t"`
			ByClass map[string]int64 `json:"byClass"`
			Red     map[string]bool  `json:"red"`
		}
		if err := json.Unmarshal([]byte(r.last), &tick); err != nil {
			t.Fatalf("tick illisible : %v (%s)", err, r.last)
		}
		if tick.T == 0 || len(tick.ByClass) != 6 || !tick.Red["24h"] || !tick.Red["1h"] {
			t.Errorf("tick : %s", r.last)
		}
		// Les dernières requêtes arrivent à la connexion, avant le premier tick.
		var hits []store.Hit
		if err := json.Unmarshal([]byte(r.hits), &hits); err != nil {
			t.Fatalf("hits illisible : %v (%s)", err, r.hits)
		}
		if len(hits) != 2 || hits[0].Seq != 1 || !hits[0].Red || hits[1].Name != "Maison" || hits[1].Red {
			t.Errorf("hits : %s", r.hits)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pas 2 ticks en 3 s")
	}
}

func TestHealth(t *testing.T) {
	s, _ := newServer(t, nil)
	rec := get(t, s, "/health")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"lastLine":"2026-11-10T11:59:00Z"`) {
		t.Fatalf("sain : HTTP %d %s", rec.Code, rec.Body)
	}
}

func TestHealthStale(t *testing.T) {
	s, _ := newServer(t, func(c *Config) { c.Now = func() time.Time { return now.Add(15 * time.Minute) } })
	if rec := get(t, s, "/health"); rec.Code != 503 {
		t.Fatalf("dernière ligne il y a 16 min : HTTP %d %s", rec.Code, rec.Body)
	}

	// Relecture en cours : sain pendant 5 min, en erreur au-delà.
	s, _ = newServer(t, func(c *Config) {
		c.Ready = func() bool { return false }
		c.StartedAt = now.Add(-time.Minute)
	})
	if rec := get(t, s, "/health"); rec.Code != 200 {
		t.Fatalf("démarrage depuis 1 min : HTTP %d", rec.Code)
	}
	s, _ = newServer(t, func(c *Config) {
		c.Ready = func() bool { return false }
		c.StartedAt = now.Add(-6 * time.Minute)
	})
	if rec := get(t, s, "/health"); rec.Code != 503 {
		t.Fatalf("relecture depuis 6 min : HTTP %d", rec.Code)
	}

	// Aucune ligne jamais lue (access.log vide ou illisible).
	s, _ = newServer(t, func(c *Config) { c.Store = store.New() })
	if rec := get(t, s, "/health"); rec.Code != 503 {
		t.Fatalf("aucune ligne : HTTP %d", rec.Code)
	}
}

func TestStatic(t *testing.T) {
	s, _ := newServer(t, nil)
	rec := get(t, s, "/")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "tableau") {
		t.Fatalf("/ : HTTP %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("index : Cache-Control = %q", cc)
	}
	if rec.Header().Get("Content-Security-Policy") == "" || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("en-têtes de sécurité absents")
	}
	rec = get(t, s, "/assets/app.js")
	if rec.Code != 200 || !strings.Contains(rec.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("asset : HTTP %d, Cache-Control %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	if rec := get(t, s, "/nexiste/pas"); rec.Code != 404 {
		t.Errorf("chemin inconnu : HTTP %d", rec.Code)
	}
	if rec := get(t, s, "/api/nexiste/pas"); rec.Code != 404 {
		t.Errorf("API inconnue : HTTP %d", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/summary", nil)
	rr := httptest.NewRecorder()
	s.ServeHTTP(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST : HTTP %d", rr.Code)
	}
}

func TestSummaryRangeEndpoint(t *testing.T) {
	s, _ := newServer(t, nil)
	from, to := now.Add(-10*time.Minute).Unix(), now.Unix()
	rec := get(t, s, fmt.Sprintf("/api/summary?from=%d&to=%d", from, to))
	if rec.Code != 200 {
		t.Fatalf("HTTP %d : %s", rec.Code, rec.Body)
	}
	var body struct {
		Window string       `json:"window"`
		Range  *store.Range `json:"range"`
		IPs    []store.IPRow
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Window != "" || body.Range == nil || body.Range.From != from || len(body.IPs) != 2 {
		t.Fatalf("réponse : %+v", body)
	}
	if rec := get(t, s, fmt.Sprintf("/api/ip/198.51.100.1?from=%d&to=%d", from, to)); rec.Code != 200 {
		t.Errorf("détail d'IP : HTTP %d", rec.Code)
	}
	for _, q := range []string{"from=10&to=5", "from=x&to=5", "from=10", fmt.Sprintf("from=%d&to=%d", from-30*86400, to-20*86400)} {
		if rec := get(t, s, "/api/summary?"+q); rec.Code != 400 {
			t.Errorf("%s : HTTP %d", q, rec.Code)
		}
	}
}
