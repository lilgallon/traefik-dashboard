package names

import (
	"errors"
	"strings"
	"testing"
)

const whitelistOK = `
name: home/whitelist
whitelist:
  reason: test
  ip:
    - "203.0.113.10"
  cidr:
    - "2001:db8:1:f700::/64"
    - "192.168.0.0/16"
`

const dynamicOK = `
http:
  middlewares:
    app-allowlist:
      ipAllowList:
        sourceRange:
          - 2001:db8:1:f700::/64
          - 203.0.113.10/32
`

func src(path, data string) Source { return Source{Path: path, Data: []byte(data)} }

func TestCheckLists(t *testing.T) {
	r := mustParse(t, sample)
	cs := src("/crowdsec/whitelist.yaml", whitelistOK)
	dyn := src("/traefik/dynamic.yml", dynamicOK)
	if got := CheckLists(r, cs, dyn); len(got) != 0 {
		t.Fatalf("cohérent, mais : %v", got)
	}

	got := CheckLists(r, cs, src(dyn.Path, dynamicOK+"          - 1.2.3.4/32\n"))
	if len(got) != 1 || !strings.Contains(got[0], "1.2.3.4/32") || !strings.Contains(got[0], "dynamic.yml") {
		t.Fatalf("IP en plus dans dynamic.yml : %v", got)
	}

	missing := strings.Replace(whitelistOK, `    - "192.168.0.0/16"`+"\n", "", 1)
	got = CheckLists(r, src(cs.Path, missing), dyn)
	if len(got) != 1 || !strings.Contains(got[0], "192.168.0.0/16") || !strings.Contains(got[0], "whitelist.yaml") {
		t.Fatalf("CIDR absent de la whitelist : %v", got)
	}

	// Fichiers illisibles ou invalides : un message par liste, pas de panique.
	if got := CheckLists(r, src(cs.Path, "whitelist: ["), src(dyn.Path, "http: [")); len(got) != 2 {
		t.Fatalf("fichiers invalides : %v", got)
	}
	unreadable := Source{Path: cs.Path, Err: errors.New("absent")}
	if got := CheckLists(r, unreadable, Source{Path: dyn.Path, Err: errors.New("absent")}); len(got) != 2 {
		t.Fatalf("fichiers absents : %v", got)
	}
	// Listes utilisées mais fichiers non configurés : on le dit.
	if got := CheckLists(r, Source{}, Source{}); len(got) != 2 || !strings.Contains(got[0], "TRAEFIK_DYNAMIC") || !strings.Contains(got[1], "CROWDSEC_WHITELIST") {
		t.Fatalf("non configurés : %v", got)
	}
	// Middleware absent du fichier dynamique.
	got = CheckLists(r, cs, src(dyn.Path, "http:\n  middlewares: {}\n"))
	if len(got) != 1 || !strings.Contains(got[0], "app-allowlist") {
		t.Fatalf("middleware absent : %v", got)
	}
}

func TestCheckListsUnused(t *testing.T) {
	// Aucune règle ne porte de liste : rien à contrôler, même sans fichiers.
	r := mustParse(t, "names:\n  - name: X\n    ip: [1.2.3.4]\n")
	if got := CheckLists(r, Source{}, Source{}); len(got) != 0 {
		t.Fatalf("aucune liste utilisée, mais : %v", got)
	}
}
