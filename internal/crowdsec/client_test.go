package crowdsec

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
	"time"
)

func TestBans(t *testing.T) {
	var gotKey, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey, gotQuery = r.Header.Get("X-Api-Key"), r.URL.RawQuery
		if r.URL.Path != "/v1/decisions" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte(`[
		 {"duration":"3h59m12.5s","id":1,"origin":"crowdsec","scenario":"crowdsecurity/http-probing","scope":"Ip","type":"ban","value":"198.51.100.7"},
		 {"duration":"1h","id":2,"origin":"cscli","scenario":"manual","scope":"Range","type":"ban","value":"203.0.113.0/24"},
		 {"duration":"1h","id":3,"origin":"crowdsec","scenario":"x","scope":"Ip","type":"captcha","value":"198.51.100.8"},
		 {"duration":"n'importe quoi","id":4,"origin":"crowdsec","scenario":"y","scope":"ip","type":"ban","value":"2001:db8::1"},
		 {"duration":"1h","id":5,"origin":"crowdsec","scenario":"z","scope":"Ip","type":"ban","value":"pas une ip"}
		]`))
	}))
	defer srv.Close()

	bans, err := NewClient(srv.URL, "clé-secrète").Bans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if gotKey != "clé-secrète" {
		t.Errorf("X-Api-Key = %q", gotKey)
	}
	if gotQuery == "" {
		t.Error("aucun filtre dans la requête")
	}
	if len(bans) != 2 {
		t.Fatalf("%d bans : %+v", len(bans), bans)
	}
	b := bans[netip.MustParseAddr("198.51.100.7")]
	if b.Scenario != "crowdsecurity/http-probing" {
		t.Errorf("scénario = %q", b.Scenario)
	}
	if d := time.Until(b.Until); d < 3*time.Hour+58*time.Minute || d > 4*time.Hour {
		t.Errorf("fin du ban dans %v", d)
	}
	if _, ok := bans[netip.MustParseAddr("2001:db8::1")]; !ok {
		t.Error("ban IPv6 (durée illisible) absent")
	}
}

func TestBansNone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("null"))
	}))
	defer srv.Close()
	bans, err := NewClient(srv.URL, "k").Bans(context.Background())
	if err != nil || len(bans) != 0 {
		t.Fatalf("bans=%v err=%v", bans, err)
	}
}

func TestBansUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // serveur fermé : connexion refusée
	if _, err := NewClient(url, "k").Bans(context.Background()); err == nil {
		t.Fatal("erreur attendue")
	}

	forbidden := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"access forbidden"}`, http.StatusForbidden)
	}))
	defer forbidden.Close()
	if _, err := NewClient(forbidden.URL, "mauvaise").Bans(context.Background()); err == nil {
		t.Fatal("403 : erreur attendue")
	}

	garbage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>pas du JSON"))
	}))
	defer garbage.Close()
	if _, err := NewClient(garbage.URL, "k").Bans(context.Background()); err == nil {
		t.Fatal("réponse illisible : erreur attendue")
	}

	// Sans clé : erreur immédiate, aucune requête.
	if _, err := NewClient(forbidden.URL, "").Bans(context.Background()); err == nil {
		t.Fatal("clé vide : erreur attendue")
	}
}
