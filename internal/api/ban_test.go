package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/crowdsec"
)

type fakeBlocker struct {
	banned   []string
	unbanned []string
	err      error
}

func (f *fakeBlocker) Ban(_ context.Context, ip netip.Addr, d time.Duration, reason, by string) error {
	if f.err != nil {
		return f.err
	}
	f.banned = append(f.banned, ip.String()+" "+d.String()+" "+reason+" "+by)
	return nil
}

func (f *fakeBlocker) Unban(_ context.Context, ip netip.Addr, by string) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.unbanned = append(f.unbanned, ip.String()+" "+by)
	return 2, nil
}

func blockServer(t *testing.T, b Blocker) *Server {
	s, _ := newServer(t, func(c *Config) {
		c.Blocker = b
		c.AllowedHosts = []string{"192.168.1.2:8080"}
		c.Protected = func(ip netip.Addr) string {
			if ip == netip.MustParseAddr("203.0.113.10") {
				return "nommée Maison"
			}
			return ""
		}
	})
	return s
}

func write(s *Server, method, url, body string, headers ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, url, strings.NewReader(body))
	req.Host = "192.168.1.2:8080"
	req.RemoteAddr = "192.168.1.20:5000"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(WriteHeader, "1")
	req.Header.Set("Origin", "http://192.168.1.2:8080")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	for i := 0; i+1 < len(headers); i += 2 {
		if headers[i+1] == "" {
			req.Header.Del(headers[i])
		} else {
			req.Header.Set(headers[i], headers[i+1])
		}
	}
	if len(headers) > 0 && headers[0] == "Host" {
		req.Host = headers[1]
	}
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	return rec
}

func TestBan(t *testing.T) {
	b := &fakeBlocker{}
	s := blockServer(t, b)
	rec := write(s, http.MethodPost, "/api/ip/198.51.100.1/ban", `{"duration":"24h","reason":" scan\n  wp-admin "}`)
	if rec.Code != 200 {
		t.Fatalf("HTTP %d : %s", rec.Code, rec.Body)
	}
	if len(b.banned) != 1 || b.banned[0] != "198.51.100.1 24h0m0s manuel (tableau de bord) : scan wp-admin 192.168.1.20" {
		t.Errorf("bans : %q", b.banned)
	}
	rec = write(s, http.MethodDelete, "/api/ip/198.51.100.1/ban", "")
	if rec.Code != 200 || len(b.unbanned) != 1 || !strings.Contains(rec.Body.String(), `"deleted":2`) {
		t.Errorf("levée : HTTP %d %s %q", rec.Code, rec.Body, b.unbanned)
	}
}

func TestBanRefused(t *testing.T) {
	b := &fakeBlocker{}
	s := blockServer(t, b)
	cases := []struct {
		name, url, body string
		headers         []string
		code            int
	}{
		{"IP protégée", "/api/ip/203.0.113.10/ban", `{"duration":"4h"}`, nil, 409},
		{"durée inconnue", "/api/ip/198.51.100.1/ban", `{"duration":"1y"}`, nil, 400},
		{"corps illisible", "/api/ip/198.51.100.1/ban", `pas du json`, nil, 400},
		{"IP invalide", "/api/ip/x/ban", `{"duration":"4h"}`, nil, 400},
		{"sans en-tête de la page", "/api/ip/198.51.100.1/ban", `{"duration":"4h"}`, []string{WriteHeader, ""}, 403},
		{"autre site", "/api/ip/198.51.100.1/ban", `{"duration":"4h"}`, []string{"Sec-Fetch-Site", "cross-site"}, 403},
		{"autre origine", "/api/ip/198.51.100.1/ban", `{"duration":"4h"}`, []string{"Origin", "http://evil.example"}, 403},
		{"DNS rebinding", "/api/ip/198.51.100.1/ban", `{"duration":"4h"}`, []string{"Host", "evil.example"}, 403},
	}
	for _, c := range cases {
		if rec := write(s, http.MethodPost, c.url, c.body, c.headers...); rec.Code != c.code {
			t.Errorf("%s : HTTP %d (attendu %d) %s", c.name, rec.Code, c.code, rec.Body)
		}
	}
	if len(b.banned) != 0 {
		t.Errorf("bans posés malgré le refus : %q", b.banned)
	}
}

func TestBanDisabledAndFailing(t *testing.T) {
	s := blockServer(t, nil)
	if rec := write(s, http.MethodPost, "/api/ip/198.51.100.1/ban", `{"duration":"4h"}`); rec.Code != 503 {
		t.Errorf("sans compte machine : HTTP %d", rec.Code)
	}
	var got struct{ Block blockState }
	json.Unmarshal(get(t, s, "/api/ip/203.0.113.10?window=1h").Body.Bytes(), &got)
	if got.Block.Enabled || got.Block.Refused != "nommée Maison" {
		t.Errorf("état du blocage : %+v", got.Block)
	}

	s = blockServer(t, &fakeBlocker{err: errors.New("LAPI CrowdSec : HTTP 500")})
	if rec := write(s, http.MethodPost, "/api/ip/198.51.100.1/ban", `{"duration":"4h"}`); rec.Code != 502 {
		t.Errorf("LAPI en erreur : HTTP %d", rec.Code)
	}
	got.Block = blockState{}
	json.Unmarshal(get(t, s, "/api/ip/198.51.100.1?window=1h").Body.Bytes(), &got)
	if !got.Block.Enabled || got.Block.Refused != "" {
		t.Errorf("état du blocage : %+v", got.Block)
	}
}

func TestCrowdSecBlocks(t *testing.T) {
	s, _ := newServer(t, func(c *Config) {
		c.CrowdSec = func(from, to time.Time) *crowdsec.View {
			return &crowdsec.View{Enabled: true, Error: to.Sub(from).String()}
		}
		c.IPAlerts = func(ip netip.Addr) []crowdsec.Alert {
			return []crowdsec.Alert{{ID: 1, IP: ip.String(), Kind: "waf"}}
		}
	})
	var sum struct {
		Window   string
		CrowdSec *crowdsec.View
	}
	json.Unmarshal(get(t, s, "/api/summary?window=1h").Body.Bytes(), &sum)
	if sum.Window != "1h" || sum.CrowdSec == nil || !sum.CrowdSec.Enabled || sum.CrowdSec.Error != "1h1m0s" {
		t.Errorf("résumé : %+v", sum)
	}
	var d struct {
		IP     string
		Alerts []crowdsec.Alert
	}
	json.Unmarshal(get(t, s, "/api/ip/198.51.100.1?window=1h").Body.Bytes(), &d)
	if d.IP != "198.51.100.1" || len(d.Alerts) != 1 || d.Alerts[0].IP != "198.51.100.1" {
		t.Errorf("détail : %+v", d)
	}
	// Intervalle choisi : les alertes de l'intervalle arrondi aux minutes.
	json.Unmarshal(get(t, s, fmt.Sprintf("/api/summary?from=%d&to=%d", now.Add(-10*time.Minute).Unix(), now.Unix())).Body.Bytes(), &sum)
	if sum.CrowdSec == nil || sum.CrowdSec.Error != "10m0s" {
		t.Errorf("intervalle : %+v", sum.CrowdSec)
	}
}
