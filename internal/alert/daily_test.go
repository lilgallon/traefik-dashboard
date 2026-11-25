package alert

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/i18n"
	"github.com/lilgallon/traefik-dashboard/internal/logline"
	"github.com/lilgallon/traefik-dashboard/internal/store"
)

func sampleSummary() store.Summary {
	return store.Summary{
		Window: "24h",
		KPIs: store.KPIs{AvgRPS: 1.26, PeakRPS: 9, BytesIn: 1536, BytesOut: 1_200_000_000, ActiveBans: 3,
			ByClass: store.ClassCounts{logline.Class2xx: 110742, logline.Class3xx: 12, logline.Class4xx: 950,
				logline.Class5xx: 4, logline.ClassAbandon: 7, logline.Class1xx: 2}},
		Verdict: store.Verdict{Red: true, Offenders: []store.Offender{
			{IP: "198.51.100.1", Country: "FR", Org: "Orange", Count: 3, URLs: []string{"home.example.com/api/states/<script>"}}}},
		IPs: []store.IPRow{
			{IP: "198.51.100.238", Name: "Partenaire GCP 2", Verdict: store.VerdictKnown, ByClass: store.ClassCounts{logline.Class2xx: 100000}},
			{IP: "198.51.100.1", Country: "FR", Org: "Orange", Verdict: store.VerdictSuccess, ByClass: store.ClassCounts{logline.Class2xx: 3}},
			{IP: "203.0.113.9", Country: "US", Org: "Evil <b>Corp</b>", Verdict: store.VerdictBlocked, ByClass: store.ClassCounts{logline.Class4xx: 420},
				Ban: &store.Ban{Scenario: "crowdsecurity/http-probing"}},
		},
		Services: []store.ServiceRow{
			{Router: "app", Requests: 110000, Err5xx: 0},
			{Router: "n8n", Requests: 300, Err5xx: 4},
		},
		Sensitive: []store.Sensitive{{IP: "203.0.113.9", Host: "status.example.com", Path: "/.env?<script>", Status: 200, Count: 2}},
		Counts:    store.Counts{IPs: 40, NamedIPs: 12, UnknownIPs: 28},
	}
}

func TestDailyText(t *testing.T) {
	txt := DailyText(sampleSummary(), pageURL, i18n.FR)
	for _, want := range []string{
		"111\u202f717",     // total des requêtes, séparateur de milliers
		"2xx 110\u202f742", // statuts
		"abandons 7",       //
		"n8n : 4",          // 5xx par router
		"198.51.100.1",     // IP en succès
		"203.0.113.9",      // top IP non nommées
		"http-probing",     // ban
		"3 ban",            // bans en cours
		pageURL,            // lien
		"28 non nommées",   //
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("résumé sans %q :\n%s", want, txt)
		}
	}
	if strings.Contains(txt, "Partenaire GCP 2</code>") {
		t.Errorf("une IP nommée figure dans le top des inconnues :\n%s", txt)
	}
	if len(txt) > 4096 {
		t.Errorf("résumé trop long pour Telegram : %d", len(txt))
	}
}

func TestDailyTextEnglish(t *testing.T) {
	txt := DailyText(sampleSummary(), pageURL, i18n.EN)
	for _, want := range []string{
		"last 24 h", "1 unknown IP got a success", "111,717", "2xx 110,742", "aborted 7",
		"n8n: 4", "28 unnamed", "3 current bans", "blocked, banned (crowdsecurity/http-probing)",
		"Volume: 1.5 kB in, 1.2 GB out", "Open the dashboard",
	} {
		if !strings.Contains(txt, want) {
			t.Errorf("summary without %q:\n%s", want, txt)
		}
	}
}

func TestDailyTextEscapes(t *testing.T) {
	txt := DailyText(sampleSummary(), pageURL, i18n.FR)
	if strings.Contains(txt, "<script>") || strings.Contains(txt, "<b>Corp</b>") {
		t.Fatalf("HTML non échappé :\n%s", txt)
	}
	if !strings.Contains(txt, "&lt;script&gt;") || !strings.Contains(txt, "Evil &lt;b&gt;Corp&lt;/b&gt;") {
		t.Fatalf("valeurs échappées absentes :\n%s", txt)
	}
}

func TestDailyTextQuietDay(t *testing.T) {
	s := sampleSummary()
	s.Verdict = store.Verdict{}
	s.Sensitive = nil
	s.Services[1].Err5xx = 0
	s.KPIs.ByClass[logline.Class5xx] = 0
	s.KPIs.ActiveBans = -1
	txt := DailyText(s, pageURL, i18n.FR)
	for _, want := range []string{"Aucune IP inconnue n'a obtenu de succès", "Aucune erreur 5xx", "CrowdSec injoignable"} {
		if !strings.Contains(txt, want) {
			t.Errorf("résumé calme sans %q :\n%s", want, txt)
		}
	}
}

func TestNextRun(t *testing.T) {
	at := time.Date(2026, 11, 10, 7, 59, 0, 0, time.UTC)
	next, err := nextRun("08:00", at)
	if err != nil || !next.Equal(time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("avant 8 h : %v %v", next, err)
	}
	next, _ = nextRun("08:00", time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC))
	if !next.Equal(time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("à 8 h pile : %v", next)
	}
	if _, err := nextRun("8h", at); err == nil {
		t.Fatal("heure invalide acceptée")
	}
}

func TestRunDailyStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		RunDaily(ctx, "08:00", func() string { return "x" }, &fakeNotifier{})
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("RunDaily ne s'arrête pas")
	}
}
