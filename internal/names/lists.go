package names

import (
	"fmt"
	"net/netip"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v3"
)

// Source est un fichier de configuration à comparer aux listes de names.yaml.
type Source struct {
	Path string // vide : non configuré
	Data []byte
	Err  error // lecture impossible
}

// CheckLists compare les listes de confiance de names.yaml à leurs copies. La
// liste « crowdsec » correspond à la whitelist CrowdSec ; tout autre nom désigne
// un middleware ipAllowList du fichier dynamique de Traefik. Seules les listes
// utilisées par une règle sont contrôlées. Renvoie un message par écart, rien si
// tout est cohérent.
func CheckLists(r *Rules, crowdsec, traefik Source) []string {
	var out []string

	var dyn struct {
		HTTP struct {
			Middlewares map[string]struct {
				IPAllowList struct {
					SourceRange []string `yaml:"sourceRange"`
				} `yaml:"ipAllowList"`
			} `yaml:"middlewares"`
		} `yaml:"http"`
	}
	dynOK := false
	switch {
	case traefik.Path == "":
	case traefik.Err != nil:
	default:
		dynOK = yaml.Unmarshal(traefik.Data, &dyn) == nil
	}
	dynFile := filepath.Base(traefik.Path)

	for _, list := range r.UsedLists() {
		want := r.ListPrefixes(list)
		if list == ListCrowdsec {
			if crowdsec.Path == "" {
				out = append(out, "la liste « crowdsec » est utilisée dans names.yaml mais CROWDSEC_WHITELIST n'est pas défini : contrôle impossible")
				continue
			}
			var cs struct {
				Whitelist struct {
					IP   []string `yaml:"ip"`
					CIDR []string `yaml:"cidr"`
				} `yaml:"whitelist"`
			}
			if crowdsec.Err != nil || yaml.Unmarshal(crowdsec.Data, &cs) != nil || len(cs.Whitelist.IP)+len(cs.Whitelist.CIDR) == 0 {
				out = append(out, fmt.Sprintf("whitelist CrowdSec (%s) illisible ou vide : contrôle de la liste « crowdsec » impossible", filepath.Base(crowdsec.Path)))
				continue
			}
			out = append(out, diff(want, append(cs.Whitelist.IP, cs.Whitelist.CIDR...), list, filepath.Base(crowdsec.Path))...)
			continue
		}
		if traefik.Path == "" {
			out = append(out, fmt.Sprintf("la liste « %s » est utilisée dans names.yaml mais TRAEFIK_DYNAMIC n'est pas défini : contrôle impossible", list))
			continue
		}
		if !dynOK {
			out = append(out, fmt.Sprintf("fichier dynamique de Traefik (%s) illisible : contrôle de la liste « %s » impossible", dynFile, list))
			continue
		}
		ranges := dyn.HTTP.Middlewares[list].IPAllowList.SourceRange
		if len(ranges) == 0 {
			out = append(out, fmt.Sprintf("middleware ipAllowList « %s » introuvable ou vide dans %s : contrôle impossible", list, dynFile))
			continue
		}
		out = append(out, diff(want, ranges, list, dynFile)...)
	}
	return out
}

func diff(want []netip.Prefix, have []string, list, file string) []string {
	wantSet := map[string]bool{}
	for _, p := range want {
		wantSet[p.String()] = true
	}
	haveSet := map[string]bool{}
	var out []string
	for _, s := range have {
		p, err := parsePrefix(s)
		if err != nil {
			out = append(out, fmt.Sprintf("%s : entrée « %s » invalide", file, s))
			continue
		}
		haveSet[p.String()] = true
	}
	for p := range wantSet {
		if !haveSet[p] {
			out = append(out, fmt.Sprintf("%s est dans names.yaml (%s) mais pas dans %s", p, list, file))
		}
	}
	for p := range haveSet {
		if !wantSet[p] {
			out = append(out, fmt.Sprintf("%s est dans %s mais pas dans names.yaml (%s)", p, file, list))
		}
	}
	sort.Strings(out)
	return out
}
