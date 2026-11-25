package logline

import (
	"net/netip"
	"testing"
	"time"
)

// Ligne réelle de access.log (champs inutiles retirés pour la lisibilité).
const realLine = `{"ClientAddr":"172.30.0.2:49926","ClientHost":"203.0.113.10","ClientPort":"49926","ClientUsername":"-","DownstreamContentSize":28,"DownstreamStatus":200,"Duration":2474769,"OriginContentSize":28,"OriginDuration":2045040,"OriginStatus":200,"Overhead":429729,"RequestAddr":"home.example.com","RequestContentSize":12,"RequestCount":412,"RequestHost":"home.example.com","RequestMethod":"GET","RequestPath":"/api/states?x=1","RequestPort":"-","RequestProtocol":"HTTP/1.1","RequestScheme":"https","RetryAttempts":0,"RouterName":"homeassistant@file","ServiceAddr":"192.168.1.10:8123","ServiceName":"homeassistant@file","StartUTC":"2026-11-10T22:25:14.081958843Z","entryPointName":"websecure","level":"info","msg":"","request_User-Agent":"Home Assistant/2026.11","time":"2026-11-10T22:25:14Z"}`

func TestParseLine(t *testing.T) {
	e, ok := ParseLine([]byte(realLine))
	if !ok {
		t.Fatal("ligne réelle refusée")
	}
	want := Entry{
		Time:     time.Date(2026, 11, 10, 22, 25, 14, 0, time.UTC),
		IP:       netip.MustParseAddr("203.0.113.10"),
		Status:   200,
		Router:   "homeassistant",
		Host:     "home.example.com",
		Path:     "/api/states?x=1",
		Method:   "GET",
		UA:       "Home Assistant/2026.11",
		BytesIn:  12,
		BytesOut: 28,
		Duration: 2474769 * time.Nanosecond,
	}
	if e != want {
		t.Fatalf("obtenu  %+v\nattendu %+v", e, want)
	}
}

func TestParseLineEdgeCases(t *testing.T) {
	if _, ok := ParseLine([]byte(`time="2026-11-10" level=error msg="pas du JSON"`)); ok {
		t.Error("non-JSON accepté")
	}
	if _, ok := ParseLine([]byte(``)); ok {
		t.Error("ligne vide acceptée")
	}
	if _, ok := ParseLine([]byte(`{"ClientHost":"1.2.3.4","time":"2026-11-10T22:25:14Z"`)); ok {
		t.Error("JSON tronqué accepté")
	}

	e, ok := ParseLine([]byte(`{"ClientHost":"1.2.3.4","DownstreamStatus":404,"time":"2026-11-10T22:25:14Z"}`))
	if !ok || e.Router != "" {
		t.Errorf("RouterName absent : ok=%v router=%q", ok, e.Router)
	}

	e, ok = ParseLine([]byte(`{"ClientHost":"1.2.3.4","DownstreamStatus":0,"time":"2026-11-10T22:25:14Z"}`))
	if !ok || ClassOf(e.Status) != ClassAbandon {
		t.Errorf("statut 0 : ok=%v classe=%q", ok, ClassOf(e.Status))
	}

	e, ok = ParseLine([]byte(`{"ClientHost":"2001:db8:ffff:3094::15a1:a47a","DownstreamStatus":200,"time":"2026-11-10T22:25:14Z"}`))
	if !ok || !e.IP.Is6() {
		t.Errorf("IPv6 : ok=%v ip=%v", ok, e.IP)
	}

	if _, ok := ParseLine([]byte(`{"ClientHost":"","DownstreamStatus":200,"time":"2026-11-10T22:25:14Z"}`)); ok {
		t.Error("ClientHost vide accepté")
	}
	if _, ok := ParseLine([]byte(`{"ClientHost":"1.2.3.4","DownstreamStatus":200}`)); ok {
		t.Error("ligne sans time acceptée")
	}
	// Un type inattendu sur un champ secondaire ne doit pas faire planter.
	if _, ok := ParseLine([]byte(`{"ClientHost":"1.2.3.4","DownstreamStatus":"x","time":"2026-11-10T22:25:14Z"}`)); ok {
		t.Error("statut non numérique accepté")
	}
	// X-Forwarded-For forgé, journalisé en entier : la dernière adresse fait foi.
	e, ok = ParseLine([]byte(`{"ClientHost":"localhost,203.0.113.104","DownstreamStatus":403,"time":"2026-10-31T04:40:04Z"}`))
	if !ok || e.IP.String() != "203.0.113.104" {
		t.Errorf("liste d'adresses : ok=%v ip=%v", ok, e.IP)
	}
	if _, ok := ParseLine([]byte(`{"ClientHost":"1.2.3.4,localhost","DownstreamStatus":403,"time":"2026-10-31T04:40:04Z"}`)); ok {
		t.Error("liste finissant par un nom acceptée")
	}
	// IPv4 notée en IPv6 : ramenée à l'IPv4 pour que les règles CIDR s'appliquent.
	e, ok = ParseLine([]byte(`{"ClientHost":"::ffff:10.0.0.1","DownstreamStatus":200,"time":"2026-11-10T22:25:14Z"}`))
	if !ok || !e.IP.Is4() {
		t.Errorf("IPv4 mappée : ok=%v ip=%v", ok, e.IP)
	}
}

func TestClassOf(t *testing.T) {
	cases := map[int]Class{0: ClassAbandon, 101: Class1xx, 200: Class2xx, 304: Class3xx, 403: Class4xx, 499: Class4xx, 502: Class5xx, 700: ClassAbandon}
	for status, want := range cases {
		if got := ClassOf(status); got != want {
			t.Errorf("ClassOf(%d) = %q, attendu %q", status, got, want)
		}
	}
}

func TestSuccessLongLivedSocket(t *testing.T) {
	// WebSocket accepté : Traefik journalise 0, la durée fait la différence.
	if !(Entry{Status: 0, Duration: 45 * time.Second}).Success() {
		t.Error("connexion de 45 s sans statut : succès attendu")
	}
	if (Entry{Status: 0, Duration: 3 * time.Second}).Success() {
		t.Error("abandon de 3 s compté comme succès")
	}
	if (Entry{Status: 403, Duration: time.Hour}).Success() {
		t.Error("403 lent compté comme succès")
	}
}

func TestSuccess(t *testing.T) {
	for status, want := range map[int]bool{101: true, 204: true, 200: true, 301: false, 403: false, 0: false, 100: false, 500: false} {
		if got := (Entry{Status: status}).Success(); got != want {
			t.Errorf("Success(%d) = %v", status, got)
		}
	}
}
