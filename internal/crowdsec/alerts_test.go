package crowdsec

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/geo"
	"github.com/lilgallon/traefik-dashboard/internal/store"
)

// Alertes au format de la LAPI (v1.8), abrégées.
const alertsJSON = `[
 {"id":1,"created_at":"2026-11-11T04:45:38Z","scenario":"crowdsecurity/http-probing","machine_id":"localhost","events_count":11,
  "message":"Ip 203.0.113.50 performed 'crowdsecurity/http-probing' (11 events over 3s)",
  "source":{"ip":"203.0.113.50","value":"203.0.113.50","scope":"Ip","cn":"SG","as_name":"GOOGLE-CLOUD-PLATFORM"},
  "decisions":[{"type":"ban","origin":"crowdsec","value":"203.0.113.50"}],
  "meta":[{"key":"target_uri","value":"[\"/\",\"/.git/config\",\"/.env\",\"/.env.local\",\"/.env.bak\"]"},{"key":"target_fqdn","value":"[\"app.example.com\"]"}],
  "events":[{"meta":[{"key":"http_path","value":"/"}]}]},
 {"id":2,"created_at":"2026-11-11T08:57:36Z","scenario":"crowdsecurity/vpatch-env-access","machine_id":"localhost","events_count":1,
  "message":"WAF block: crowdsecurity/vpatch-env-access from 203.0.113.66 (172.25.0.2)",
  "source":{"ip":"203.0.113.66","value":"203.0.113.66","scope":"Ip","cn":"US","as_name":"Datacamp Limited"},
  "decisions":null,
  "meta":[{"key":"target_uri","value":"[\"/.env\"]"},{"key":"msg","value":"[\"Detect access to .env files\"]"}],
  "events":[{"meta":[{"key":"uri","value":"/.env"},{"key":"target_fqdn","value":"app.example.com"}]}]},
 {"id":3,"created_at":"2026-11-11T09:00:00Z","scenario":"crowdsecurity/vpatch-env-access","machine_id":"localhost","events_count":1,
  "message":"WAF block: crowdsecurity/vpatch-env-access from 203.0.113.67 (172.25.0.2)",
  "source":{"ip":"203.0.113.67","value":"203.0.113.67","scope":"Ip"},
  "decisions":[],"meta":null,
  "events":[{"meta":[{"key":"message","value":"Detect access to .env files"},{"key":"uri","value":"/.env"},{"key":"target_fqdn","value":"home.example.com"}]}]},
 {"id":4,"created_at":"2026-11-11T19:43:02Z","scenario":"manuel (tableau de bord) : test","machine_id":"traefik-dashboard","events_count":1,
  "message":"manuel (tableau de bord) : test","source":{"ip":"192.0.2.123","value":"192.0.2.123","scope":"Ip"},
  "decisions":[{"type":"ban","origin":"cscli","value":"192.0.2.123"}],"meta":null,"events":null},
 {"id":5,"created_at":"2026-11-11T05:46:15Z","scenario":"crowdsecurity/http-technology-probing","machine_id":"localhost","events_count":10,
  "message":"Ip 203.0.113.50 performed …","source":{"ip":"203.0.113.50","value":"203.0.113.50","scope":"Ip"},
  "decisions":[],"meta":null,"events":null}
]`

func alertsServer(t *testing.T, gotQuery *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/watchers/login":
			w.Write([]byte(`{"token":"t","expire":"2099-01-01T00:00:00Z"}`))
		case "/v1/alerts":
			*gotQuery = r.URL.RawQuery
			w.Write([]byte(alertsJSON))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestAlerts(t *testing.T) {
	var q string
	srv := alertsServer(t, &q)
	alerts, err := NewWriter(srv.URL, "traefik-dashboard", "pw").Alerts(context.Background(), 2*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(q, "include_capi=false") || !strings.Contains(q, "since=2h0m0s") || !strings.Contains(q, "limit=0") {
		t.Errorf("requête : %s", q)
	}
	if len(alerts) != 5 {
		t.Fatalf("%d alertes", len(alerts))
	}
	a := alerts[0]
	if a.Kind != "ban" || a.Manual || a.Host != "app.example.com" || len(a.Paths) != 3 || a.More != 2 || a.Country != "SG" || a.Events != 11 {
		t.Errorf("scénario : %+v", a)
	}
	if b := alerts[1]; b.Kind != "waf" || b.Detail != "Detect access to .env files" || b.Host != "app.example.com" || b.Paths[0] != "/.env" {
		t.Errorf("WAF : %+v", b)
	}
	if c := alerts[2]; c.Kind != "waf" || c.Detail != "Detect access to .env files" || c.Host != "home.example.com" {
		t.Errorf("WAF (méta d'événement seule) : %+v", c)
	}
	if d := alerts[3]; d.Kind != "ban" || !d.Manual {
		t.Errorf("ban manuel : %+v", d)
	}
	if e := alerts[4]; e.Kind != "alert" {
		t.Errorf("sans décision : %+v", e)
	}
}

func TestFeedView(t *testing.T) {
	var q string
	srv := alertsServer(t, &q)
	alerts, _ := NewWriter(srv.URL, "traefik-dashboard", "pw").Alerts(context.Background(), time.Hour)
	now := time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC)
	f := NewFeed(true)
	f.SetAlerts(alerts, true, nil, now)
	f.SetBlocklist(Blocklist{Community: 15000, Lists: 57000})
	bans := map[netip.Addr]store.Ban{
		netip.MustParseAddr("203.0.113.50"): {Scenario: "crowdsecurity/http-probing", Until: now.Add(time.Hour)},
		netip.MustParseAddr("192.0.2.123"):  {Scenario: "manuel", Until: now.Add(3 * time.Hour), Manual: true},
	}
	lookup := func(ip netip.Addr) geo.Info { return geo.Info{Country: "XX", Org: "Org " + ip.String()} }

	v := f.View(24*time.Hour, now, bans, lookup)
	if !v.Enabled || v.Blocklist == nil || v.Blocklist.Lists != 57000 {
		t.Errorf("vue : %+v", v)
	}
	if len(v.Bans) != 2 || v.Bans[0].IP != "192.0.2.123" || !v.Bans[0].Manual || v.Bans[1].Country != "XX" {
		t.Errorf("bans (expiration la plus lointaine d'abord) : %+v", v.Bans)
	}
	c := v.Counts
	if c.Bans != 2 || c.WAF != 2 || c.Other != 1 || c.IPs != 4 || c.Manual != 1 {
		t.Errorf("compteurs : %+v", c)
	}
	s := v.Scenarios[0]
	if s.Scenario != "crowdsecurity/vpatch-env-access" || s.Kind != "waf" || s.Alerts != 2 || s.IPs != 2 || len(s.Hosts) != 2 || s.Detail == "" {
		t.Errorf("premier scénario : %+v", s)
	}
	if len(v.Recent) != 5 || v.Recent[0].ID != 4 {
		t.Errorf("dernières alertes : %+v", v.Recent)
	}

	// Sur la dernière heure, seule l'alerte manuelle (19:43) reste.
	if v := f.View(time.Hour, now, nil, nil); len(v.Recent) != 1 || v.Counts.Bans != 1 || len(v.Bans) != 0 {
		t.Errorf("1 h : %+v", v)
	}

	if h := f.ForIP(netip.MustParseAddr("203.0.113.50")); len(h) != 2 || h[0].ID != 5 {
		t.Errorf("historique d'IP : %+v", h)
	}

	// Lecture incrémentale : fusion ; erreur : l'historique reste, l'erreur s'affiche.
	f.SetAlerts([]Alert{{ID: 99, At: now, IP: "198.51.100.1", Kind: "waf"}}, false, nil, now)
	f.SetAlerts(nil, false, errors.New("LAPI CrowdSec : HTTP 500"), now)
	if v := f.View(24*time.Hour, now, nil, nil); len(v.Recent) != 6 || v.Error == "" {
		t.Errorf("après fusion puis erreur : %d alertes, erreur %q", len(v.Recent), v.Error)
	}
	// Au-delà de 7 jours, l'alerte est oubliée.
	f.SetAlerts(nil, false, nil, now.Add(8*24*time.Hour))
	if h := f.ForIP(netip.MustParseAddr("203.0.113.50")); len(h) != 0 {
		t.Errorf("alerte périmée gardée : %+v", h)
	}
}

func TestBlocklist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "k" || !strings.Contains(r.URL.RawQuery, "origins=CAPI,lists") {
			w.WriteHeader(403)
			return
		}
		w.Write([]byte(`[{"origin":"CAPI"},{"origin":"lists"},{"origin":"lists"}]`))
	}))
	defer srv.Close()
	b, err := NewClient(srv.URL, "k").Blocklist(context.Background())
	if err != nil || b.Community != 1 || b.Lists != 2 {
		t.Errorf("blocklist : %+v %v", b, err)
	}
}
