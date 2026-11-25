package crowdsec

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"sync/atomic"
	"testing"
	"time"
)

func TestWriter(t *testing.T) {
	var logins atomic.Int32
	var alertBody []map[string]any
	var deleted string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/watchers/login":
			var in map[string]any
			json.NewDecoder(r.Body).Decode(&in)
			if in["machine_id"] != "traefik-dashboard" || in["password"] != "pw" {
				w.WriteHeader(401)
				return
			}
			n := logins.Add(1)
			json.NewEncoder(w).Encode(map[string]any{"token": "jeton" + string(rune('0'+n)), "expire": time.Now().Add(time.Hour)})
		case r.Header.Get("Authorization") == "Bearer jeton1" && logins.Load() == 1 && r.Method == http.MethodDelete:
			// Premier jeton refusé (LAPI redémarrée) : le client doit se reconnecter.
			w.WriteHeader(401)
		case r.Header.Get("Authorization") == "":
			w.WriteHeader(401)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/alerts":
			b, _ := io.ReadAll(r.Body)
			json.Unmarshal(b, &alertBody)
			w.Write([]byte(`["1"]`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/decisions":
			deleted = r.URL.Query().Get("ip")
			w.Write([]byte(`{"nbDeleted":"2"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	if NewWriter(srv.URL, "traefik-dashboard", "") != nil {
		t.Error("Writer créé sans mot de passe")
	}
	wr := NewWriter(srv.URL, "traefik-dashboard", "pw")
	ctx := context.Background()
	if err := wr.Ban(ctx, netip.MustParseAddr("198.51.100.7"), 24*time.Hour, "manuel"); err != nil {
		t.Fatal(err)
	}
	if len(alertBody) != 1 {
		t.Fatalf("alerte : %+v", alertBody)
	}
	d := alertBody[0]["decisions"].([]any)[0].(map[string]any)
	if d["value"] != "198.51.100.7" || d["duration"] != "24h0m0s" || d["origin"] != "cscli" || d["type"] != "ban" || d["scope"] != "Ip" {
		t.Errorf("décision : %+v", d)
	}
	n, err := wr.Unban(ctx, netip.MustParseAddr("2001:db8::1"))
	if err != nil || n != 2 || deleted != "2001:db8::1" {
		t.Errorf("levée : n=%d err=%v ip=%q", n, err, deleted)
	}
	if logins.Load() != 2 {
		t.Errorf("%d connexions (attendu 2 : jeton refusé puis renouvelé)", logins.Load())
	}

	bad := NewWriter(srv.URL, "traefik-dashboard", "faux")
	if err := bad.Ban(ctx, netip.MustParseAddr("198.51.100.7"), time.Hour, "x"); err == nil {
		t.Error("mot de passe faux accepté")
	}
}
