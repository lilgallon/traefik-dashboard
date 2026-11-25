package names

import (
	"net/netip"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v3"

	"github.com/lilgallon/traefik-dashboard/internal/i18n"
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
func CheckLists(r *Rules, crowdsec, traefik Source) []i18n.Msg {
	var out []i18n.Msg

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
				out = append(out, i18n.M("lists.crowdsec_unset"))
				continue
			}
			var cs struct {
				Whitelist struct {
					IP   []string `yaml:"ip"`
					CIDR []string `yaml:"cidr"`
				} `yaml:"whitelist"`
			}
			if crowdsec.Err != nil || yaml.Unmarshal(crowdsec.Data, &cs) != nil || len(cs.Whitelist.IP)+len(cs.Whitelist.CIDR) == 0 {
				out = append(out, i18n.M("lists.crowdsec_unreadable", filepath.Base(crowdsec.Path)))
				continue
			}
			out = append(out, diff(want, append(cs.Whitelist.IP, cs.Whitelist.CIDR...), list, filepath.Base(crowdsec.Path))...)
			continue
		}
		if traefik.Path == "" {
			out = append(out, i18n.M("lists.traefik_unset", list))
			continue
		}
		if !dynOK {
			out = append(out, i18n.M("lists.traefik_unreadable", dynFile, list))
			continue
		}
		ranges := dyn.HTTP.Middlewares[list].IPAllowList.SourceRange
		if len(ranges) == 0 {
			out = append(out, i18n.M("lists.middleware_missing", list, dynFile))
			continue
		}
		out = append(out, diff(want, ranges, list, dynFile)...)
	}
	return out
}

func diff(want []netip.Prefix, have []string, list, file string) []i18n.Msg {
	wantSet := map[string]bool{}
	for _, p := range want {
		wantSet[p.String()] = true
	}
	haveSet := map[string]bool{}
	var out []i18n.Msg
	for _, s := range have {
		p, err := parsePrefix(s)
		if err != nil {
			out = append(out, i18n.M("lists.invalid_entry", file, s))
			continue
		}
		haveSet[p.String()] = true
	}
	for p := range wantSet {
		if !haveSet[p] {
			out = append(out, i18n.M("lists.missing_in_file", p, list, file))
		}
	}
	for p := range haveSet {
		if !wantSet[p] {
			out = append(out, i18n.M("lists.missing_in_names", p, file, list))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].In(i18n.EN) < out[j].In(i18n.EN) })
	return out
}
