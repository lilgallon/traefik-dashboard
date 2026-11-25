package store

import (
	"fmt"
	"math"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/geo"
	"github.com/lilgallon/traefik-dashboard/internal/logline"
)

var t0 = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

func req(at time.Time, ip, router, path string, status int) logline.Entry {
	return logline.Entry{
		Time: at, IP: netip.MustParseAddr(ip), Status: status, Router: router,
		Host: router + ".test", Path: path, Method: "GET", UA: "ua-" + ip,
		BytesIn: 10, BytesOut: 100, Duration: 5 * time.Millisecond,
	}
}

var unknown = Identity{}

func rowOf(t *testing.T, s Summary, ip string) IPRow {
	t.Helper()
	for _, r := range s.IPs {
		if r.IP == ip {
			return r
		}
	}
	t.Fatalf("IP %s absente du résumé (%d lignes)", ip, len(s.IPs))
	return IPRow{}
}

func TestVerdicts(t *testing.T) {
	s := New()
	at := t0.Add(-10 * time.Minute)
	// connue : toutes ses requêtes sont nommées.
	s.Add(req(at, "10.0.0.1", "ha", "/api/states", 200), Identity{Name: "LAN"})
	// succès : un succès hors URL anonyme.
	s.Add(req(at, "198.51.100.1", "ha", "/api/states", 200), unknown)
	s.Add(req(at, "198.51.100.1", "ha", "/", 200), Identity{Anonymous: true})
	// succès par WebSocket (101).
	s.Add(req(at, "198.51.100.6", "ha", "/api/websocket", 101), unknown)
	// public : succès uniquement sur des URL anonymes.
	s.Add(req(at, "198.51.100.2", "ha", "/", 200), Identity{Anonymous: true})
	s.Add(req(at, "198.51.100.2", "ha", "/nope", 404), unknown)
	// bloquée : seulement des 403.
	s.Add(req(at, "198.51.100.3", "app", "/api", 403), unknown)
	s.Add(req(at, "198.51.100.3", "app", "/ocpi", 403), unknown)
	// tâtonne : des 404, aucun succès.
	s.Add(req(at, "198.51.100.4", "ha", "/wp-login.php", 404), unknown)
	s.Add(req(at, "198.51.100.4", "ha", "/x", 403), unknown)
	// autre : ni succès ni 4xx (redirection, abandon).
	s.Add(req(at, "198.51.100.5", "ha", "/", 301), unknown)
	s.Add(req(at, "198.51.100.5", "ha", "/", 0), unknown)
	// en partie couverte : nommée sur une requête, inconnue en succès sur l'autre.
	s.Add(req(at, "20.0.0.1", "ha", "/api/webhook/x", 200), Identity{Name: "Netatmo"})
	s.Add(req(at, "20.0.0.1", "ha", "/api/states", 200), unknown)
	// WebSocket accepté (journalisé 0 par Traefik, mais long) : succès.
	ws := req(at, "198.51.100.8", "ha", "/api/websocket", 0)
	ws.Duration = 10 * time.Minute
	s.Add(ws, unknown)
	// 3xx seul : ne déclenche pas le rouge.
	s.Add(req(at, "198.51.100.7", "ha", "/redirect", 302), unknown)

	sum := s.Summary(W24h, t0)
	want := map[string]string{
		"10.0.0.1":     VerdictKnown,
		"198.51.100.1": VerdictSuccess,
		"198.51.100.6": VerdictSuccess,
		"198.51.100.2": VerdictPublic,
		"198.51.100.3": VerdictBlocked,
		"198.51.100.4": VerdictProbing,
		"198.51.100.5": VerdictOther,
		"20.0.0.1":     VerdictSuccess,
		"198.51.100.7": VerdictOther,
		"198.51.100.8": VerdictSuccess,
	}
	for ip, v := range want {
		if got := rowOf(t, sum, ip).Verdict; got != v {
			t.Errorf("%s : verdict %q, attendu %q", ip, got, v)
		}
	}
	if got := rowOf(t, sum, "10.0.0.1").Name; got != "LAN" {
		t.Errorf("nom = %q", got)
	}
	if got := rowOf(t, sum, "20.0.0.1").Name; got != "" {
		t.Errorf("IP en partie couverte affichée sous le nom %q", got)
	}

	if !sum.Verdict.Red {
		t.Fatal("verdict global : rouge attendu")
	}
	if len(sum.Verdict.Offenders) != 4 {
		t.Fatalf("offenders : %+v", sum.Verdict.Offenders)
	}
	for _, o := range sum.Verdict.Offenders {
		if o.Count != 1 || len(o.URLs) != 1 {
			t.Errorf("offender %+v", o)
		}
		if o.IP == "198.51.100.1" && o.URLs[0] != "ha.test/api/states" {
			t.Errorf("URL en cause : %v", o.URLs)
		}
	}
	if !s.Red(W24h, t0) {
		t.Error("Red() devrait être vrai")
	}

	// Ordre : nommées d'abord, puis inconnues par succès décroissants.
	if sum.IPs[0].Verdict != VerdictKnown {
		t.Errorf("première ligne : %+v", sum.IPs[0])
	}
	if v := sum.IPs[1].Verdict; v != VerdictSuccess {
		t.Errorf("deuxième ligne : verdict %q", v)
	}
}

func TestVerdictGreenWhenNothingRed(t *testing.T) {
	s := New()
	s.Add(req(t0.Add(-time.Minute), "198.51.100.2", "ha", "/", 200), Identity{Anonymous: true})
	s.Add(req(t0.Add(-time.Minute), "198.51.100.3", "ha", "/x", 403), unknown)
	sum := s.Summary(W1h, t0)
	if sum.Verdict.Red || len(sum.Verdict.Offenders) != 0 || s.Red(W1h, t0) {
		t.Fatalf("vert attendu : %+v", sum.Verdict)
	}
}

func TestSummaryKPIs(t *testing.T) {
	s := New()
	base := t0.Add(-30 * time.Second)
	// 10 requêtes sur 10 s, dont 3 dans la même seconde.
	for i := 0; i < 8; i++ {
		s.Add(req(base.Add(time.Duration(i)*time.Second), "10.0.0.1", "ha", "/", 200), Identity{Name: "LAN"})
	}
	s.Add(req(base.Add(3*time.Second), "10.0.0.1", "ha", "/", 404), Identity{Name: "LAN"})
	s.Add(req(base.Add(3*time.Second), "10.0.0.1", "ha", "/", 502), Identity{Name: "LAN"})

	for _, w := range []Window{W1h, W24h, W7d} {
		sum := s.Summary(w, t0)
		k := sum.KPIs
		wantAvg := 10 / w.Duration().Seconds()
		if math.Abs(k.AvgRPS-wantAvg) > 1e-9 {
			t.Errorf("%s : AvgRPS = %v, attendu %v", w, k.AvgRPS, wantAvg)
		}
		if k.PeakRPS != 3 {
			t.Errorf("%s : PeakRPS = %v, attendu 3", w, k.PeakRPS)
		}
		if k.BytesIn != 100 || k.BytesOut != 1000 {
			t.Errorf("%s : octets %d / %d", w, k.BytesIn, k.BytesOut)
		}
		if k.ByClass[logline.Class2xx] != 8 || k.ByClass[logline.Class4xx] != 1 || k.ByClass[logline.Class5xx] != 1 {
			t.Errorf("%s : statuts %v", w, k.ByClass)
		}
		if len(k.ByClass) != len(logline.Classes) {
			t.Errorf("%s : toutes les classes doivent être présentes : %v", w, k.ByClass)
		}
	}
}

func TestSeries(t *testing.T) {
	s := New()
	for i := 0; i < 120; i++ { // 2 req/s pendant 60 s, il y a 10 min
		s.Add(req(t0.Add(-10*time.Minute).Add(time.Duration(i)*500*time.Millisecond), "10.0.0.1", "ha", "/", 200), Identity{Name: "LAN"})
	}
	sum := s.Summary(W1h, t0)
	se := sum.Series
	if se.StepSeconds != 1 || len(se.T) != 3541-int(LiveLag/time.Second) || len(se.ByClass[logline.Class2xx]) != len(se.T) {
		t.Fatalf("1h : pas %d, %d points", se.StepSeconds, len(se.T))
	}
	var total float64
	for _, v := range se.ByClass[logline.Class2xx] {
		total += v
	}
	if total != 120 {
		t.Errorf("1h : somme de la série = %v", total)
	}
	if se.T[1]-se.T[0] != 1 {
		t.Errorf("1h : pas des abscisses = %d", se.T[1]-se.T[0])
	}

	sum = s.Summary(W24h, t0)
	se = sum.Series
	if se.StepSeconds != 60 || len(se.T) != 1440 {
		t.Fatalf("24h : pas %d, %d points", se.StepSeconds, len(se.T))
	}
	var max float64
	for _, v := range se.ByClass[logline.Class2xx] {
		max = math.Max(max, v)
	}
	if max != 2 {
		t.Errorf("24h : pic de la série = %v req/s, attendu 2", max)
	}

	se = s.Summary(W7d, t0).Series
	if se.StepSeconds != 3600 || len(se.T) != 168 {
		t.Fatalf("7d : pas %d, %d points", se.StepSeconds, len(se.T))
	}
}

func TestWindows(t *testing.T) {
	s := New()
	s.Add(req(t0.Add(-2*time.Hour), "198.51.100.1", "ha", "/api/states", 200), unknown)
	s.Add(req(t0.Add(-3*24*time.Hour), "198.51.100.2", "ha", "/api/states", 200), unknown)
	s.Add(req(t0.Add(-8*24*time.Hour), "198.51.100.3", "ha", "/api/states", 200), unknown)

	if sum := s.Summary(W1h, t0); len(sum.IPs) != 0 || sum.Verdict.Red || sum.KPIs.ByClass[logline.Class2xx] != 0 {
		t.Errorf("1h : %+v", sum.IPs)
	}
	if sum := s.Summary(W24h, t0); len(sum.IPs) != 1 || !sum.Verdict.Red {
		t.Errorf("24h : %d IP", len(sum.IPs))
	}
	if sum := s.Summary(W7d, t0); len(sum.IPs) != 2 {
		t.Errorf("7d : %d IP", len(sum.IPs))
	}
	if s.Red(W1h, t0) || !s.Red(W24h, t0) {
		t.Error("Red() ne suit pas la fenêtre")
	}

	s.Prune(t0)
	if got := s.Stats().Minutes; got != 2 {
		t.Errorf("après Prune : %d minutes gardées, attendu 2", got)
	}
}

func TestStoreCardinalityCaps(t *testing.T) {
	s := New()
	at := t0.Add(-5 * time.Minute)
	ip := netip.MustParseAddr("100.0.0.0")
	const n = 5000
	for i := 0; i < n; i++ {
		e := req(at, "1.1.1.1", "ha", fmt.Sprintf("/p/%c%c/x", 'a'+i%26, 'a'+(i/26)%26), 404)
		e.IP = ip
		ip = ip.Next()
		s.Add(e, unknown)
	}
	if got := s.Stats().Cells; got > maxIPsPerMinute+1 {
		t.Fatalf("%d cellules pour %d IP : plafond dépassé", got, n)
	}
	sum := s.Summary(W1h, t0)
	if got := sum.KPIs.ByClass[logline.Class4xx]; got != n {
		t.Fatalf("total 4xx = %d, attendu %d", got, n)
	}
	var rows, others int64
	for _, r := range sum.IPs {
		rows += r.ByClass[logline.Class4xx]
		if r.IP == OthersIP {
			others = r.ByClass[logline.Class4xx]
		}
	}
	if others != n-maxIPsPerMinute {
		t.Errorf("ligne « autres » : %d requêtes, attendu %d", others, n-maxIPsPerMinute)
	}
	if sum.Counts.IPs != maxIPsPerMinute+1 {
		t.Errorf("Counts.IPs = %d", sum.Counts.IPs)
	}
	if len(sum.IPs) > maxRows {
		t.Errorf("%d lignes renvoyées", len(sum.IPs))
	}

	// Rafale d'URL distinctes depuis une seule IP : les URL sont regroupées, le
	// compte de l'IP reste exact.
	s = New()
	for i := 0; i < n; i++ {
		s.Add(req(at, "198.51.100.9", "ha", fmt.Sprintf("/scan/%c%c%c", 'a'+i%26, 'a'+(i/26)%26, 'a'+(i/676)%26), 404), unknown)
	}
	if got := s.Stats().Cells; got > maxURLsPerMinute+1 {
		t.Fatalf("%d cellules pour %d URL", got, n)
	}
	sum = s.Summary(W1h, t0)
	if got := rowOf(t, sum, "198.51.100.9").ByClass[logline.Class4xx]; got != n {
		t.Errorf("IP du scan : %d requêtes, attendu %d", got, n)
	}
	var grouped bool
	for _, u := range sum.URLs {
		if u.Path == OthersPath {
			grouped = u.ByClass[logline.Class4xx] == n-maxURLsPerMinute
		}
	}
	if !grouped {
		t.Error("URL hors plafond non regroupées sous « (autres) »")
	}

	// Un succès noyé dans la rafale reste rouge.
	s.Add(req(at, "198.51.100.9", "ha", "/scan/gagné", 200), unknown)
	if !s.Summary(W1h, t0).Verdict.Red {
		t.Error("succès regroupé : le rouge est perdu")
	}
}

func TestGlobalCellBudget(t *testing.T) {
	s := New()
	s.maxCells = 500
	ip := netip.MustParseAddr("100.0.0.0")
	total := 0
	for m := 0; m < 20; m++ {
		at := t0.Add(-time.Duration(30-m) * time.Minute)
		for i := 0; i < 100; i++ {
			e := req(at, "1.1.1.1", "ha", "/", 404)
			e.IP = ip
			ip = ip.Next()
			s.Add(e, unknown)
			total++
		}
	}
	if got := s.Stats().Cells; got > 500+20*minCellsPerMinute {
		t.Fatalf("%d cellules malgré un budget de 500", got)
	}
	sum := s.Summary(W1h, t0)
	if got := sum.KPIs.ByClass[logline.Class4xx]; got != int64(total) {
		t.Fatalf("total = %d, attendu %d", got, total)
	}
	if !sum.Capped {
		t.Error("plafond atteint mais non signalé")
	}
}

func containsWarning(ws []string, sub string) bool {
	for _, w := range ws {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

func TestReclassify(t *testing.T) {
	s := New()
	at := t0.Add(-3 * time.Hour)
	s.Add(req(at, "198.51.100.1", "ha", "/api/states", 200), unknown)
	s.Add(req(at, "198.51.100.1", "ha", "/api/config", 200), unknown)
	if sum := s.Summary(W24h, t0); !sum.Verdict.Red || rowOf(t, sum, "198.51.100.1").Verdict != VerdictSuccess {
		t.Fatal("rouge attendu avant le renommage")
	}

	s.Reclassify(func(e logline.Entry) Identity {
		if e.IP.String() == "198.51.100.1" && e.Router == "ha" && e.Path == "/api/states" {
			return Identity{Name: "Proche"}
		}
		return Identity{}
	})
	sum := s.Summary(W24h, t0)
	if r := rowOf(t, sum, "198.51.100.1"); r.Verdict != VerdictSuccess || len(sum.Verdict.Offenders[0].URLs) != 1 {
		t.Fatalf("règle partielle : %+v", r)
	}

	s.Reclassify(func(e logline.Entry) Identity { return Identity{Name: "Proche"} })
	sum = s.Summary(W24h, t0)
	if r := rowOf(t, sum, "198.51.100.1"); r.Verdict != VerdictKnown || r.Name != "Proche" {
		t.Fatalf("après renommage : %+v", r)
	}
	if sum.Verdict.Red || s.Red(W24h, t0) {
		t.Fatal("le rouge passé n'a pas disparu")
	}

	// Et dans l'autre sens : retirer la règle refait apparaître le rouge.
	s.Reclassify(func(e logline.Entry) Identity { return Identity{} })
	if !s.Red(W24h, t0) {
		t.Fatal("règle retirée : rouge attendu")
	}
}

func TestP95(t *testing.T) {
	s := New()
	at := t0.Add(-time.Minute)
	for i := 1; i <= 100; i++ { // 1 ms … 100 ms : p95 ≈ 95 ms
		e := req(at, "10.0.0.1", "svc", "/", 200)
		e.Duration = time.Duration(i) * time.Millisecond
		s.Add(e, Identity{Name: "LAN"})
	}
	e := req(at, "10.0.0.1", "svc", "/", 503)
	e.Duration = 2 * time.Millisecond
	s.Add(e, Identity{Name: "LAN"})
	s.Add(req(at, "10.0.0.1", "", "/", 404), Identity{Name: "LAN"})

	sum := s.Summary(W1h, t0)
	var svc, none *ServiceRow
	for i := range sum.Services {
		switch sum.Services[i].Router {
		case "svc":
			svc = &sum.Services[i]
		case NoRouter:
			none = &sum.Services[i]
		}
	}
	if svc == nil || none == nil {
		t.Fatalf("services : %+v", sum.Services)
	}
	if svc.Requests != 101 || svc.Err5xx != 1 {
		t.Errorf("svc : %+v", svc)
	}
	// Buckets 50–100 ms : la valeur doit tomber dans le bon bucket ou son voisin.
	if svc.P95Ms < 50 || svc.P95Ms > 200 {
		t.Errorf("p95 = %v ms, attendu ≈ 95", svc.P95Ms)
	}
}

func TestIPDetail(t *testing.T) {
	s := New()
	at := t0.Add(-10 * time.Minute)
	s.Add(req(at, "198.51.100.1", "ha", "/api/states/x?token=abc&p=2", 200), unknown)
	s.Add(req(at, "198.51.100.1", "ha", "/api/states/x?token=abc&p=2", 200), unknown)
	s.Add(req(at, "198.51.100.1", "gotify", "/message", 401), unknown)
	s.Add(req(at, "10.0.0.1", "ha", "/secret?x=1", 200), Identity{Name: "LAN"})
	s.Enrich = func(ip netip.Addr) (geo.Info, string) {
		return geo.Info{Country: "FR", ASN: 51207, Org: "Free Mobile SAS"}, "Mon téléphone"
	}

	d, ok := s.IPDetail(netip.MustParseAddr("198.51.100.1"), W1h, t0)
	if !ok {
		t.Fatal("détail introuvable")
	}
	if d.Verdict != VerdictSuccess || d.Country != "FR" || d.ASN != 51207 || d.LooksLike != "Mon téléphone" {
		t.Errorf("ligne : %+v", d.IPRow)
	}
	if len(d.URLs) != 2 || d.URLs[0].Path != "/api/states/x" || d.URLs[0].ByClass[logline.Class2xx] != 2 {
		t.Errorf("URL : %+v", d.URLs)
	}
	if d.URLs[0].Sockets != 0 {
		t.Errorf("sockets : %+v", d.URLs[0])
	}
	if d.Routers["ha"] != 2 || d.Routers["gotify"] != 1 {
		t.Errorf("routers : %v", d.Routers)
	}
	if len(d.UserAgents) != 1 || d.UserAgents[0] != "ua-198.51.100.1" {
		t.Errorf("user-agents : %v", d.UserAgents)
	}
	// Chemins bruts avec query, dédoublonnés quand ils se suivent.
	if len(d.RawPaths) != 2 || d.RawPaths[0].Path != "/message" || d.RawPaths[1].Path != "/api/states/x?token=…&p=2" {
		t.Errorf("chemins bruts : %+v", d.RawPaths)
	}

	// WebSocket accepté : compté à part des abandons.
	ws := req(at, "198.51.100.7", "ha", "/api/websocket", 0)
	ws.Duration = time.Hour
	s.Add(ws, unknown)
	s.Add(req(at, "198.51.100.7", "ha", "/api/websocket", 0), unknown)
	if d, _ := s.IPDetail(netip.MustParseAddr("198.51.100.7"), W1h, t0); len(d.URLs) != 1 || d.URLs[0].Sockets != 1 || d.URLs[0].ByClass[logline.ClassAbandon] != 2 {
		t.Errorf("WebSocket : %+v", d.URLs)
	}

	// IP nommée : pas de chemins bruts, mais la règle qui l'a nommée.
	d, ok = s.IPDetail(netip.MustParseAddr("10.0.0.1"), W1h, t0)
	if !ok || len(d.RawPaths) != 0 || d.Name != "LAN" || d.Rules["LAN"] != 1 {
		t.Errorf("IP nommée : %+v", d)
	}

	if _, ok := s.IPDetail(netip.MustParseAddr("203.0.113.1"), W1h, t0); ok {
		t.Error("IP jamais vue : détail renvoyé")
	}
	if _, ok := s.IPDetail(netip.MustParseAddr("198.51.100.1"), W1h, t0.Add(3*time.Hour)); ok {
		t.Error("IP hors fenêtre : détail renvoyé")
	}
}

func TestSecondAndLastLine(t *testing.T) {
	s := New()
	if !s.LastLine().IsZero() {
		t.Fatal("LastLine devrait être zéro")
	}
	at := t0.Add(-LiveLag)
	s.Add(req(at, "10.0.0.1", "ha", "/", 200), Identity{Name: "LAN"})
	s.Add(req(at, "10.0.0.1", "ha", "/", 500), Identity{Name: "LAN"})
	s.Add(req(at.Add(-time.Second), "10.0.0.1", "ha", "/", 200), Identity{Name: "LAN"})

	tick := s.Second(t0)
	if tick.T != at.Unix() || tick.ByClass[logline.Class2xx] != 1 || tick.ByClass[logline.Class5xx] != 1 || tick.BytesOut != 200 {
		t.Fatalf("tick : %+v", tick)
	}
	if !tick.LastLine.Equal(at) || !s.LastLine().Equal(at) {
		t.Errorf("LastLine = %v", s.LastLine())
	}
	// Une seconde sans trafic donne un tick à zéro, pas les compteurs d'il y a une heure.
	tick = s.Second(t0.Add(time.Hour))
	if tick.ByClass[logline.Class2xx] != 0 {
		t.Errorf("tick périmé : %+v", tick)
	}
}

func TestOutOfOrderAndSealedMinutes(t *testing.T) {
	s := New()
	// Avancer de plusieurs minutes scelle les anciennes ; une ligne en retard doit
	// quand même être comptée, sans doublon de cellule.
	for m := 0; m < 6; m++ {
		s.Add(req(t0.Add(time.Duration(m-10)*time.Minute), "198.51.100.1", "ha", "/a", 404), unknown)
	}
	before := s.Stats().Cells
	s.Add(req(t0.Add(-10*time.Minute), "198.51.100.1", "ha", "/a", 404), unknown)
	if got := s.Stats().Cells; got != before {
		t.Errorf("ligne en retard : %d cellules au lieu de %d", got, before)
	}
	if got := rowOf(t, s.Summary(W1h, t0), "198.51.100.1").ByClass[logline.Class4xx]; got != 7 {
		t.Errorf("total = %d, attendu 7", got)
	}
}

func TestSensitiveHits(t *testing.T) {
	s := New()
	at := t0.Add(-time.Hour)
	s.Add(req(at, "198.51.100.1", "kuma", "/.env", 200), Identity{Anonymous: true})
	s.Add(req(at, "198.51.100.1", "kuma", "/.env", 404), Identity{Anonymous: true})
	s.Add(req(at, "10.0.0.1", "kuma", "/.env", 200), Identity{Name: "LAN"})
	sum := s.Summary(W24h, t0)
	if len(sum.Sensitive) != 1 || sum.Sensitive[0].IP != "198.51.100.1" || sum.Sensitive[0].Path != "/.env" {
		t.Fatalf("chemins sensibles : %+v", sum.Sensitive)
	}
	if got := s.Summary(W24h, t0.Add(48*time.Hour)).Sensitive; len(got) != 0 {
		t.Errorf("hors fenêtre : %+v", got)
	}
}

func BenchmarkSummary7d(b *testing.B) {
	s := New()
	for m := 0; m < 7*24*60; m++ {
		at := t0.Add(-time.Duration(m) * time.Minute)
		for i := 0; i < 10; i++ {
			s.Add(req(at, fmt.Sprintf("34.0.0.%d", i%4), "app", fmt.Sprintf("/ocpi/p%d", i), 200), Identity{Name: "GCP"})
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Summary(W7d, t0)
	}
}

func TestRecent(t *testing.T) {
	s := New()
	if got := s.Recent(0, 10); len(got) != 0 {
		t.Fatalf("magasin vide : %d requêtes", len(got))
	}
	e := req(t0, "198.51.100.1", "gotify", "/stream?token=SECRET", 0)
	e.Duration = 2 * time.Hour
	s.Add(e, Identity{Geo: geo.Info{Country: "BR"}})
	s.Add(req(t0.Add(time.Second), "203.0.113.10", "ha", "/api/webhook/abcdef0123456789abcdef", 200), Identity{Name: "Maison"})
	s.Add(req(t0.Add(2*time.Second), "203.0.113.9", "ha", "/auth/login?x=1", 200), Identity{Anonymous: true})
	s.Add(req(t0.Add(3*time.Second), "203.0.113.9", "ha", "/.env", 404), unknown)

	got := s.Recent(0, 10)
	if len(got) != 4 {
		t.Fatalf("%d requêtes, attendu 4", len(got))
	}
	for i, h := range got {
		if h.Seq != uint64(i+1) {
			t.Fatalf("ordre : %+v", got)
		}
	}
	ws := got[0]
	if !ws.Red || !ws.Socket || ws.Country != "BR" || ws.IP != "198.51.100.1" || ws.Host != "gotify.test" || ws.Method != "GET" {
		t.Errorf("WebSocket d'inconnue : %+v", ws)
	}
	if strings.Contains(ws.Path, "SECRET") || ws.Path != "/stream?token=…" {
		t.Errorf("secret non masqué : %q", ws.Path)
	}
	// IP nommée : chemin normalisé, les identifiants ne sont pas affichés.
	if h := got[1]; h.Name != "Maison" || h.Red || h.Path != NormalizePath("/api/webhook/abcdef0123456789abcdef") || h.Ms != 5 {
		t.Errorf("IP nommée : %+v", h)
	}
	if h := got[2]; h.Red || h.Name != "" {
		t.Errorf("URL anonyme : %+v", h)
	}
	if h := got[3]; h.Red || h.Status != 404 {
		t.Errorf("404 : %+v", h)
	}

	if got := s.Recent(2, 10); len(got) != 2 || got[0].Seq != 3 {
		t.Errorf("après 2 : %+v", got)
	}
	if got := s.Recent(0, 1); len(got) != 1 || got[0].Seq != 4 {
		t.Errorf("limite 1 : %+v", got)
	}
	if got := s.Recent(4, 10); len(got) != 0 {
		t.Errorf("rien de neuf : %+v", got)
	}

	// Nommer l'IP après coup : la requête n'est plus rouge.
	s.Reclassify(func(e logline.Entry) Identity {
		if e.IP.String() == "198.51.100.1" && e.Router == "gotify" {
			return Identity{Name: "Téléphone"}
		}
		return Identity{}
	})
	if h := s.Recent(0, 10)[0]; h.Red || h.Name != "Téléphone" || strings.Contains(h.Path, "SECRET") {
		t.Errorf("après renommage : %+v", h)
	}

	// L'anneau ne garde que les dernières.
	for i := 0; i < maxRecent+50; i++ {
		s.Add(req(t0.Add(time.Minute), "203.0.113.9", "ha", "/", 200), unknown)
	}
	got = s.Recent(0, 1000)
	if len(got) != maxRecent || got[len(got)-1].Seq != uint64(4+maxRecent+50) || got[0].Seq != got[len(got)-1].Seq-maxRecent+1 {
		t.Errorf("anneau : %d requêtes, de %d à %d", len(got), got[0].Seq, got[len(got)-1].Seq)
	}
}

func TestSummaryRange(t *testing.T) {
	s := New()
	// Deux heures avant : hors de l'intervalle. Puis trois requêtes dedans.
	s.Add(req(t0.Add(-2*time.Hour), "198.51.100.1", "ha", "/api/states", 200), unknown)
	s.Add(req(t0.Add(-40*time.Minute), "198.51.100.2", "ha", "/x", 404), unknown)
	s.Add(req(t0.Add(-35*time.Minute), "198.51.100.2", "ha", "/y", 404), unknown)
	s.Add(req(t0.Add(-30*time.Minute), "10.0.0.1", "ha", "/api/states", 200), Identity{Name: "LAN"})

	// 12:00 - 45 min 30 s → 12:00 - 29 min 10 s : arrondi aux minutes entières.
	r := Range{From: t0.Add(-45*time.Minute - 30*time.Second).Unix(), To: t0.Add(-29*time.Minute - 10*time.Second).Unix()}
	sum, err := s.SummaryRange(r, t0)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Window != "" || sum.Range == nil {
		t.Fatalf("fenêtre %q, intervalle %+v", sum.Window, sum.Range)
	}
	if want := (Range{From: t0.Add(-46 * time.Minute).Unix(), To: t0.Add(-29 * time.Minute).Unix()}); *sum.Range != want {
		t.Errorf("intervalle %+v, attendu %+v", *sum.Range, want)
	}
	if sum.Verdict.Red {
		t.Error("le succès d'inconnue est hors de l'intervalle : vert attendu")
	}
	if len(sum.IPs) != 2 || sum.KPIs.ByClass[logline.Class4xx] != 2 || sum.KPIs.ByClass[logline.Class2xx] != 1 {
		t.Fatalf("IP %+v, statuts %+v", sum.IPs, sum.KPIs.ByClass)
	}
	// Moins d'une heure, encore dans l'anneau : série à la seconde.
	if sum.Series.StepSeconds != 1 || len(sum.Series.T) != 17*60 || sum.Series.T[0] != sum.Range.From {
		t.Errorf("série : pas %d, %d points", sum.Series.StepSeconds, len(sum.Series.T))
	}
	if got, want := sum.KPIs.AvgRPS, 3.0/(17*60); math.Abs(got-want) > 1e-9 {
		t.Errorf("moyenne %v, attendue %v", got, want)
	}

	// Plus long : à la minute, et le succès de 10 h y entre.
	sum, _ = s.SummaryRange(Range{From: t0.Add(-3 * time.Hour).Unix(), To: t0.Unix()}, t0)
	if sum.Series.StepSeconds != 60 || len(sum.Series.T) != 180 || !sum.Verdict.Red {
		t.Errorf("3 h : pas %d, %d points, rouge %v", sum.Series.StepSeconds, len(sum.Series.T), sum.Verdict.Red)
	}
	// Au-delà de 48 h : à l'heure.
	sum, _ = s.SummaryRange(Range{From: t0.Add(-72 * time.Hour).Unix(), To: t0.Unix()}, t0)
	if sum.Series.StepSeconds != 3600 || len(sum.Series.T) != 72 {
		t.Errorf("72 h : pas %d, %d points", sum.Series.StepSeconds, len(sum.Series.T))
	}

	if _, err := s.SummaryRange(Range{From: t0.Unix(), To: t0.Unix()}, t0); err == nil {
		t.Error("intervalle vide accepté")
	}
	if _, err := s.SummaryRange(Range{From: t0.Add(-30 * 24 * time.Hour).Unix(), To: t0.Add(-20 * 24 * time.Hour).Unix()}, t0); err == nil {
		t.Error("intervalle hors des 7 jours accepté")
	}

	d, ok, err := s.IPDetailRange(netip.MustParseAddr("198.51.100.1"), r, t0)
	if err != nil || ok {
		t.Errorf("IP hors de l'intervalle trouvée : %+v %v", d, err)
	}
	d, ok, _ = s.IPDetailRange(netip.MustParseAddr("198.51.100.2"), Range{From: t0.Add(-38 * time.Minute).Unix(), To: t0.Unix()}, t0)
	if !ok || len(d.RawPaths) != 1 || d.RawPaths[0].Path != "/y" {
		t.Errorf("détail limité à l'intervalle : %+v", d.RawPaths)
	}
}

func TestSensitivePeriodAndRedaction(t *testing.T) {
	s := New()
	for _, age := range []time.Duration{25 * time.Hour, time.Minute} {
		s.Add(req(t0.Add(-age), "192.0.2.1", "web", "/config.json?token=dummy", 200), Identity{})
	}
	for _, tc := range []struct {
		w     Window
		count int64
	}{{W1h, 1}, {W7d, 2}} {
		rows := s.Summary(tc.w, t0).Sensitive
		if len(rows) != 1 || rows[0].Count != tc.count || rows[0].Path != "/config.json?token=…" {
			t.Fatalf("%s: %+v", tc.w, rows)
		}
	}
}

func TestSensitiveEvictionByPeriod(t *testing.T) {
	s := New()
	for i := maxSensitive; i >= 0; i-- {
		s.Add(req(t0.Add(-time.Duration(i)*time.Minute), "192.0.2.1", "web", "/.env", 200), Identity{})
	}
	if !s.Summary(W24h, t0).SensitiveIncomplete || s.Summary(W1h, t0).SensitiveIncomplete {
		t.Fatal("expected eviction warning only in the longer window")
	}
}
