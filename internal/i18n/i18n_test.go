package i18n

import (
	"regexp"
	"strings"
	"testing"
)

var verb = regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z%]`)

// Every message exists in both languages, with the same fmt verbs in the same order.
func TestCatalogComplete(t *testing.T) {
	for k, e := range catalog {
		if strings.TrimSpace(e.en) == "" || strings.TrimSpace(e.fr) == "" {
			t.Errorf("%s: missing translation", k)
			continue
		}
		en, fr := verb.FindAllString(e.en, -1), verb.FindAllString(e.fr, -1)
		if strings.Join(en, " ") != strings.Join(fr, " ") {
			t.Errorf("%s: verbs differ: en %v, fr %v", k, en, fr)
		}
	}
	for k := range catalog {
		if base, ok := strings.CutSuffix(k, ".one"); ok {
			if _, ok := catalog[base+".other"]; !ok {
				t.Errorf("%s: no .other form", k)
			}
		}
	}
}

func TestFromAcceptLanguage(t *testing.T) {
	cases := map[string]Lang{
		"":                        EN,
		"fr-FR,fr;q=0.9,en;q=0.8": FR,
		"de-DE,de;q=0.9,fr;q=0.8": FR,
		"en-GB,en;q=0.9,fr;q=0.8": EN,
		"de, ja":                  EN,
		"FR":                      FR,
		"fr_CA":                   FR,
	}
	for h, want := range cases {
		if got := FromAcceptLanguage(h, EN); got != want {
			t.Errorf("%q → %s, want %s", h, got, want)
		}
	}
	if got := FromAcceptLanguage("de", FR); got != FR {
		t.Errorf("default not used: %s", got)
	}
}

func TestMsgAndPlural(t *testing.T) {
	m := M("warn.lists", M("lists.invalid_entry", "dynamic.yml", "x"))
	if got := m.In(EN); got != "Trust lists: dynamic.yml: invalid entry “x”." {
		t.Errorf("en: %q", got)
	}
	if got := m.In(FR); got != "Listes de confiance : dynamic.yml : entrée « x » invalide." {
		t.Errorf("fr: %q", got)
	}
	if got := N(EN, "daily.cs_bans", 0, 0); !strings.Contains(got, "0 current bans") {
		t.Errorf("en 0: %q", got)
	}
	if got := N(FR, "daily.cs_bans", 0, 0); !strings.Contains(got, "0 ban en cours") {
		t.Errorf("fr 0: %q", got)
	}
	if got := T(EN, "no.such.key"); got != "no.such.key" {
		t.Errorf("unknown key: %q", got)
	}
}

func TestNumbers(t *testing.T) {
	if got := Thousands(EN, 110742); got != "110,742" {
		t.Errorf("en: %q", got)
	}
	if got := Thousands(FR, 1234567); got != "1 234 567" {
		t.Errorf("fr: %q", got)
	}
	if got := Thousands(EN, 999); got != "999" {
		t.Errorf("small: %q", got)
	}
	if got := Bytes(EN, 1536); got != "1.5 kB" {
		t.Errorf("en bytes: %q", got)
	}
	if got := Bytes(FR, 91_500_000); got != "91,5 Mo" {
		t.Errorf("fr bytes: %q", got)
	}
	if got := Bytes(EN, 12); got != "12 B" {
		t.Errorf("bytes: %q", got)
	}
}
