// Package names applique le fichier de noms (names.yaml) : IP nommées, URL
// anonymes par conception et listes de confiance.
package names

import (
	"bytes"
	"errors"
	"fmt"
	"net/netip"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/lilgallon/traefik-dashboard/internal/logline"
)

// ListCrowdsec est la liste de confiance qui correspond à la whitelist CrowdSec.
// Tout autre nom de liste désigne un middleware ipAllowList de Traefik.
const ListCrowdsec = "crowdsec"

// Rules est le contenu de names.yaml.
type Rules struct {
	Names []NameRule `yaml:"names"`
	Anon  []AnonRule `yaml:"anonymous"` // URL anonymes par conception
}

// NameRule nomme les requêtes qui remplissent toutes ses conditions.
type NameRule struct {
	Name    string         `yaml:"name"`
	IPs     []netip.Prefix `yaml:"-"`
	RawIPs  []string       `yaml:"ip"` // IP seule (→ /32 ou /128) ou CIDR
	ASNs    []uint         `yaml:"asn"`
	Routers []string       `yaml:"routers"` // sans @file ; vide = tous
	Paths   []string       `yaml:"paths"`   // préfixes ; vide = tous
	Lists   []string       `yaml:"lists"`
}

// AnonRule déclare des URL qui répondent à n'importe qui par conception.
// Exact et Prefixes vides = tout le router.
type AnonRule struct {
	Router   string   `yaml:"router"`
	Exact    []string `yaml:"exact"`
	Prefixes []string `yaml:"prefixes"`
}

// Parse lit et valide le fichier. Un champ inconnu est une erreur : une faute de
// frappe (« router » pour « routers ») élargirait sinon la règle en silence.
func Parse(b []byte) (*Rules, error) {
	var r Rules
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("names.yaml unreadable: %w", err)
	}
	if len(r.Names) == 0 {
		return nil, errors.New("names.yaml: no rule under \"names\"")
	}
	for i := range r.Names {
		n := &r.Names[i]
		if strings.TrimSpace(n.Name) == "" {
			return nil, fmt.Errorf("names.yaml: rule #%d has no \"name\"", i+1)
		}
		if len(n.RawIPs) == 0 && len(n.ASNs) == 0 {
			return nil, fmt.Errorf("names.yaml: %q has neither \"ip\" nor \"asn\"", n.Name)
		}
		for _, s := range n.RawIPs {
			p, err := parsePrefix(s)
			if err != nil {
				return nil, fmt.Errorf("names.yaml: %q: %w", n.Name, err)
			}
			n.IPs = append(n.IPs, p)
		}
		for _, l := range n.Lists {
			if strings.TrimSpace(l) == "" || strings.Contains(l, "@") {
				return nil, fmt.Errorf("names.yaml: %q: invalid list name %q (\"crowdsec\" or a Traefik middleware name, without \"@file\")", n.Name, l)
			}
		}
		if len(n.Lists) > 0 && len(n.ASNs) > 0 {
			return nil, fmt.Errorf("names.yaml: %q: a rule with \"lists\" cannot have \"asn\"", n.Name)
		}
	}
	for i, a := range r.Anon {
		if a.Router == "" {
			return nil, fmt.Errorf("names.yaml: anonymous URLs #%d have no \"router\"", i+1)
		}
	}
	return &r, nil
}

// parsePrefix accepte une IP seule ou un CIDR, et normalise (10.1.2.3/8 → 10.0.0.0/8).
func parsePrefix(s string) (netip.Prefix, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, "/") {
		p, err := netip.ParsePrefix(s)
		if err != nil {
			return netip.Prefix{}, fmt.Errorf("invalid CIDR %q", s)
		}
		return netip.PrefixFrom(p.Addr().Unmap(), p.Bits()).Masked(), nil
	}
	a, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Prefix{}, fmt.Errorf("invalid IP %q", s)
	}
	a = a.Unmap()
	return netip.PrefixFrom(a, a.BitLen()), nil
}

func pathOnly(p string) string {
	if i := strings.IndexByte(p, '?'); i >= 0 {
		return p[:i]
	}
	return p
}

func (n *NameRule) matchesSource(ip netip.Addr, asn uint) bool {
	for _, p := range n.IPs {
		if p.Contains(ip) {
			return true
		}
	}
	return asn != 0 && slices.Contains(n.ASNs, asn)
}

// Identify nomme UNE requête : première règle dont toutes les conditions présentes
// sont vraies (IP ou ASN ; puis router et préfixe de chemin s'ils sont précisés).
func (r *Rules) Identify(e logline.Entry, asn uint) (name string, ok bool) {
	path := pathOnly(e.Path)
	for i := range r.Names {
		n := &r.Names[i]
		if !n.matchesSource(e.IP, asn) {
			continue
		}
		if len(n.Routers) > 0 && !slices.Contains(n.Routers, e.Router) {
			continue
		}
		if len(n.Paths) > 0 && !hasAnyPrefix(path, n.Paths) {
			continue
		}
		return n.Name, true
	}
	return "", false
}

// LooksLike renvoie le nom de la première règle dont la condition IP/ASN
// correspond, sans regarder router ni chemin : « ressemble à ».
func (r *Rules) LooksLike(ip netip.Addr, asn uint) string {
	for i := range r.Names {
		if r.Names[i].matchesSource(ip, asn) {
			return r.Names[i].Name
		}
	}
	return ""
}

// NamedIP renvoie le nom de la première règle dont une IP ou un CIDR contient
// l'adresse (les ASN, partagés par des milliers de clients, sont ignorés).
func (r *Rules) NamedIP(ip netip.Addr) string {
	for i := range r.Names {
		for _, p := range r.Names[i].IPs {
			if p.Contains(ip) {
				return r.Names[i].Name
			}
		}
	}
	return ""
}

// Anonymous dit si l'URL répond à n'importe qui par conception.
func (r *Rules) Anonymous(e logline.Entry) bool {
	if e.Router == "" {
		return false
	}
	path := pathOnly(e.Path)
	for _, a := range r.Anon {
		if a.Router != e.Router {
			continue
		}
		if len(a.Exact) == 0 && len(a.Prefixes) == 0 {
			return true
		}
		if slices.Contains(a.Exact, path) || hasAnyPrefix(path, a.Prefixes) {
			return true
		}
	}
	return false
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// UsedLists renvoie les listes de confiance utilisées par au moins une règle,
// triées.
func (r *Rules) UsedLists() []string {
	var out []string
	for _, n := range r.Names {
		for _, l := range n.Lists {
			if !slices.Contains(out, l) {
				out = append(out, l)
			}
		}
	}
	slices.Sort(out)
	return out
}

// ListPrefixes renvoie les IP et CIDR des règles marquées pour une liste de
// confiance, dans l'ordre du fichier.
func (r *Rules) ListPrefixes(list string) []netip.Prefix {
	var out []netip.Prefix
	for _, n := range r.Names {
		if slices.Contains(n.Lists, list) {
			out = append(out, n.IPs...)
		}
	}
	return out
}
