package names

import (
	"net/netip"
	"os"
	"testing"

	"github.com/lilgallon/traefik-dashboard/internal/logline"
)

const sample = `
names:
  - name: Maison
    ip: [203.0.113.10, "2001:db8:1:f700::/64"]
    lists: [crowdsec, app-allowlist]
  - name: LAN
    ip: [192.168.0.0/16]
    lists: [crowdsec]
  - name: Mon téléphone
    asn: [51207]
    routers: [homeassistant, gotify]
  - name: Netatmo
    asn: [8075]
    routers: [homeassistant]
    paths: [/api/webhook/]
  - name: Bureau
    ip: [203.0.113.7]
    asn: [64500]
anonymous:
  - router: homeassistant
    exact: [/, /manifest.json]
    prefixes: [/frontend_latest/, /static/, /auth/authorize]
  - router: uptime-kuma
  - router: gotify
    prefixes: [/message]
`

func mustParse(t *testing.T, s string) *Rules {
	t.Helper()
	r, err := Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func entry(ip, router, path string) logline.Entry {
	return logline.Entry{IP: netip.MustParseAddr(ip), Router: router, Path: path}
}

func TestIdentifyPerRequest(t *testing.T) {
	r := mustParse(t, sample)
	azure := "20.0.0.231"

	if name, ok := r.Identify(entry(azure, "homeassistant", "/api/webhook/x"), 8075); !ok || name != "Netatmo" {
		t.Errorf("webhook Azure → %q %v", name, ok)
	}
	// Même IP, autre chemin : la requête est celle d'une inconnue.
	if name, ok := r.Identify(entry(azure, "homeassistant", "/api/states/x"), 8075); ok {
		t.Errorf("/api/states Azure nommée %q", name)
	}
	// Même chemin, autre router.
	if _, ok := r.Identify(entry(azure, "n8n", "/api/webhook/x"), 8075); ok {
		t.Error("webhook sur un autre router nommé")
	}
	if got := r.LooksLike(netip.MustParseAddr(azure), 8075); got != "Netatmo" {
		t.Errorf("LooksLike = %q", got)
	}
	if got := r.LooksLike(netip.MustParseAddr("198.51.100.1"), 14061); got != "" {
		t.Errorf("LooksLike d'une inconnue = %q", got)
	}
	// La query string ne compte pas.
	if name, _ := r.Identify(entry(azure, "homeassistant", "/api/webhook/x?a=/api/states"), 8075); name != "Netatmo" {
		t.Errorf("query string : %q", name)
	}
	// Router sans condition de chemin.
	if name, _ := r.Identify(entry("2001:db8:ffff:3094::15a1:a47a", "gotify", "/message"), 51207); name != "Mon téléphone" {
		t.Errorf("téléphone → %q", name)
	}
}

func TestIdentifyCIDR(t *testing.T) {
	r := mustParse(t, sample)
	for ip, want := range map[string]string{
		"2001:db8:1:f700::1":      "Maison",
		"2001:db8:1:f700:1:2:3:4": "Maison",
		"203.0.113.10":            "Maison",
		"192.168.1.10":            "LAN",
	} {
		if name, ok := r.Identify(entry(ip, "nimporte", "/x"), 0); !ok || name != want {
			t.Errorf("%s → %q %v, attendu %q", ip, name, ok, want)
		}
	}
	for _, ip := range []string{"203.0.113.11", "2001:db8:1:f701::1"} {
		if name, ok := r.Identify(entry(ip, "x", "/"), 0); ok {
			t.Errorf("%s nommée %q", ip, name)
		}
	}
}

func TestIdentifyIPOrASN(t *testing.T) {
	r := mustParse(t, sample)
	if name, _ := r.Identify(entry("203.0.113.7", "x", "/"), 1); name != "Bureau" {
		t.Errorf("par IP → %q", name)
	}
	if name, _ := r.Identify(entry("198.51.100.9", "x", "/"), 64500); name != "Bureau" {
		t.Errorf("par ASN → %q", name)
	}
	// ASN 0 (IP absente de la base) ne correspond jamais à une règle.
	if _, ok := r.Identify(entry("198.51.100.9", "x", "/"), 0); ok {
		t.Error("ASN 0 nommé")
	}
}

func TestAnonymous(t *testing.T) {
	r := mustParse(t, sample)
	cases := []struct {
		router, path string
		want         bool
	}{
		{"homeassistant", "/", true},
		{"homeassistant", "/?x=1", true},
		{"homeassistant", "/manifest.json", true},
		{"homeassistant", "/static/icons/a.png", true},
		{"homeassistant", "/auth/authorize?client_id=x", true},
		{"homeassistant", "/api/states", false},
		{"homeassistant", "/auth/token", false},
		{"homeassistant", "/api/websocket", false},
		{"homeassistant", "/manifest.json.bak", false},
		{"uptime-kuma", "/x", true},
		{"gotify", "/message?limit=1", true},
		{"gotify", "/application", false},
		{"n8n", "/", false},
		{"", "/", false},
	}
	for _, c := range cases {
		if got := r.Anonymous(logline.Entry{Router: c.router, Path: c.path}); got != c.want {
			t.Errorf("%s %s → %v", c.router, c.path, got)
		}
	}
}

func TestListPrefixes(t *testing.T) {
	r := mustParse(t, sample)
	got := r.ListPrefixes("app-allowlist")
	if len(got) != 2 || got[0].String() != "203.0.113.10/32" || got[1].String() != "2001:db8:1:f700::/64" {
		t.Errorf("app-allowlist = %v", got)
	}
	if l := r.UsedLists(); len(l) != 2 || l[0] != "app-allowlist" || l[1] != "crowdsec" {
		t.Errorf("UsedLists = %v", l)
	}
	if n := len(r.ListPrefixes("crowdsec")); n != 3 {
		t.Errorf("crowdsec : %d préfixes", n)
	}
}

func TestParseErrors(t *testing.T) {
	bad := map[string]string{
		"yaml invalide":      "names: [",
		"sans nom":           "names:\n  - ip: [1.2.3.4]",
		"sans ip ni asn":     "names:\n  - name: X\n    routers: [a]",
		"ip invalide":        "names:\n  - name: X\n    ip: [1.2.3]",
		"liste avec @file":   "names:\n  - name: X\n    ip: [1.2.3.4]\n    lists: [allow@file]",
		"liste avec asn":     "names:\n  - name: X\n    asn: [1]\n    lists: [crowdsec]",
		"champ inconnu":      "names:\n  - name: X\n    ip: [1.2.3.4]\n    router: a",
		"anonyme sans route": "anonymous:\n  - prefixes: [/]",
		"aucune règle":       "",
	}
	for label, src := range bad {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s : accepté", label)
		}
	}
}

func TestNamedIP(t *testing.T) {
	r, err := Parse([]byte(`names:
  - name: Maison
    ip: [203.0.113.10, "2001:db8:1:f700::/64"]
  - name: Téléphone
    asn: [51207]
`))
	if err != nil {
		t.Fatal(err)
	}
	for ip, want := range map[string]string{
		"203.0.113.10":       "Maison",
		"2001:db8:1:f700::5": "Maison",
		"198.51.100.1":       "",
	} {
		if got := r.NamedIP(netip.MustParseAddr(ip)); got != want {
			t.Errorf("NamedIP(%s) = %q, attendu %q", ip, got, want)
		}
	}
}

// Le fichier d'exemple du dépôt doit rester valide.
func TestExampleFile(t *testing.T) {
	b, err := os.ReadFile("../../examples/names.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r := mustParse(t, string(b))
	if name, _ := r.Identify(entry("20.0.0.231", "homeassistant", "/api/webhook/x"), 8075); name != "Netatmo" {
		t.Errorf("Netatmo → %q", name)
	}
	if l := r.UsedLists(); len(l) != 2 || l[0] != "crowdsec" || l[1] != "partners" {
		t.Errorf("UsedLists = %v", l)
	}
}
