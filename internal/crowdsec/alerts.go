package crowdsec

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/geo"
	"github.com/lilgallon/traefik-dashboard/internal/store"
)

// Alert est une alerte CrowdSec locale, réduite à ce que la page affiche.
type Alert struct {
	ID       int64     `json:"id"`
	At       time.Time `json:"at"`
	IP       string    `json:"ip"` // IP, ou plage pour une alerte sur une plage
	Country  string    `json:"country"`
	Org      string    `json:"org"`
	Scenario string    `json:"scenario"`
	// Kind : « ban » (décision posée), « waf » (requête bloquée par AppSec, sans
	// ban) ou « alerte » (scénario déclenché sans décision : IP déjà bannie…).
	Kind   string   `json:"kind"`
	Manual bool     `json:"manual"`           // posée à la main (cscli ou la page)
	Detail string   `json:"detail,omitempty"` // règle WAF : « Detect access to .env files »
	Host   string   `json:"host,omitempty"`
	Paths  []string `json:"paths,omitempty"` // 3 premiers chemins visés
	More   int      `json:"more,omitempty"`  // chemins visés au-delà des 3 premiers
	Events int      `json:"events"`
}

type rawMeta struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type rawAlert struct {
	ID          int64     `json:"id"`
	CreatedAt   time.Time `json:"created_at"`
	Scenario    string    `json:"scenario"`
	Message     string    `json:"message"`
	MachineID   string    `json:"machine_id"`
	EventsCount int       `json:"events_count"`
	Source      struct {
		IP     string `json:"ip"`
		Value  string `json:"value"`
		Cn     string `json:"cn"`
		AsName string `json:"as_name"`
	} `json:"source"`
	Decisions []struct {
		Type   string `json:"type"`
		Origin string `json:"origin"`
	} `json:"decisions"`
	Meta   []rawMeta `json:"meta"`
	Events []struct {
		Meta []rawMeta `json:"meta"`
	} `json:"events"`
}

// metaValues lit une valeur de méta : tableau JSON encodé en chaîne (méta
// d'alerte, voir crowdsec/context.yaml) ou chaîne simple (méta d'événement).
func metaValues(meta []rawMeta, key string) []string {
	for _, m := range meta {
		if m.Key != key || m.Value == "" {
			continue
		}
		var list []string
		if strings.HasPrefix(m.Value, "[") && json.Unmarshal([]byte(m.Value), &list) == nil {
			return list
		}
		return []string{m.Value}
	}
	return nil
}

func (r *rawAlert) alert(machineID string) Alert {
	a := Alert{
		ID: r.ID, At: r.CreatedAt.UTC(), Scenario: r.Scenario, Events: r.EventsCount,
		IP: cmp.Or(r.Source.IP, r.Source.Value), Country: r.Source.Cn, Org: r.Source.AsName,
		Kind: "alerte", Manual: r.MachineID == machineID && machineID != "",
	}
	for _, d := range r.Decisions {
		if strings.EqualFold(d.Type, "ban") {
			a.Kind = "ban"
		}
		if d.Origin == "cscli" {
			a.Manual = true
		}
	}
	if a.Kind != "ban" && strings.HasPrefix(r.Message, "WAF block") {
		a.Kind = "waf"
	}
	if v := metaValues(r.Meta, "msg"); len(v) > 0 {
		a.Detail = v[0]
	}
	paths := metaValues(r.Meta, "target_uri")
	hosts := metaValues(r.Meta, "target_fqdn")
	for _, e := range r.Events {
		if len(hosts) == 0 {
			hosts = metaValues(e.Meta, "target_fqdn")
		}
		if len(paths) == 0 {
			paths = metaValues(e.Meta, "uri")
		}
		if a.Detail == "" && a.Kind == "waf" {
			if v := metaValues(e.Meta, "message"); len(v) > 0 {
				a.Detail = v[0]
			}
		}
	}
	if len(hosts) > 0 {
		a.Host = hosts[0]
	}
	if len(paths) > 3 {
		a.More = len(paths) - 3
		paths = paths[:3]
	}
	a.Paths = paths
	return a
}

// Alerts lit les alertes locales créées depuis since (la blocklist communautaire,
// des dizaines de milliers d'IP par alerte, est exclue).
func (w *Writer) Alerts(ctx context.Context, since time.Duration) ([]Alert, error) {
	path := fmt.Sprintf("/v1/alerts?since=%s&include_capi=false&limit=0", since.Round(time.Second))
	data, err := w.doLimit(ctx, http.MethodGet, path, nil, 64<<20)
	if err != nil {
		return nil, err
	}
	var raw []rawAlert // « null » quand il n'y en a aucune
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("LAPI CrowdSec : alertes illisibles : %w", err)
	}
	out := make([]Alert, 0, len(raw))
	for i := range raw {
		out = append(out, raw[i].alert(w.machineID))
	}
	return out, nil
}

// Blocklist compte les IP bannies par la blocklist communautaire (CAPI) et les
// listes auxquelles le moteur est abonné.
type Blocklist struct {
	Community int       `json:"community"`
	Lists     int       `json:"lists"`
	At        time.Time `json:"at"`
}

func (c *Client) Blocklist(ctx context.Context) (Blocklist, error) {
	if c.apiKey == "" {
		return Blocklist{}, errors.New("clé de bouncer CrowdSec absente (CROWDSEC_BOUNCER_KEY)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/decisions?type=ban&origins=CAPI,lists", nil)
	if err != nil {
		return Blocklist{}, err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	hc := *c.http
	hc.Timeout = time.Minute // ≈ 10 Mo de JSON
	resp, err := hc.Do(req)
	if err != nil {
		return Blocklist{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Blocklist{}, fmt.Errorf("LAPI CrowdSec : HTTP %d", resp.StatusCode)
	}
	var ds []struct {
		Origin string `json:"origin"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 128<<20)).Decode(&ds); err != nil {
		return Blocklist{}, fmt.Errorf("LAPI CrowdSec : blocklist illisible : %w", err)
	}
	b := Blocklist{At: time.Now().UTC().Truncate(time.Second)}
	for _, d := range ds {
		if strings.EqualFold(d.Origin, "lists") {
			b.Lists++
		} else {
			b.Community++
		}
	}
	return b, nil
}

// Keep : durée d'historique gardée (la plus longue période de la page, plus une
// marge).
const Keep = 7*24*time.Hour + time.Hour

// Feed garde les alertes des 7 derniers jours et la taille de la blocklist.
type Feed struct {
	mu        sync.RWMutex
	enabled   bool
	alerts    map[int64]Alert
	alertsErr error
	blocklist *Blocklist
}

func NewFeed(enabled bool) *Feed { return &Feed{enabled: enabled, alerts: map[int64]Alert{}} }

// SetAlerts range une lecture : full remplace tout, sinon fusionne (lecture
// incrémentale des dernières minutes).
func (f *Feed) SetAlerts(alerts []Alert, full bool, err error, now time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.alertsErr = err
	if err != nil {
		return
	}
	if full {
		f.alerts = make(map[int64]Alert, len(alerts))
	}
	for _, a := range alerts {
		f.alerts[a.ID] = a
	}
	cutoff := now.Add(-Keep)
	for id, a := range f.alerts {
		if a.At.Before(cutoff) {
			delete(f.alerts, id)
		}
	}
}

func (f *Feed) SetBlocklist(b Blocklist) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.blocklist = &b
}

// BanRow est un ban en cours, tel que la page le liste.
type BanRow struct {
	IP       string    `json:"ip"`
	Country  string    `json:"country"`
	Org      string    `json:"org"`
	Scenario string    `json:"scenario"`
	Manual   bool      `json:"manual"`
	Until    time.Time `json:"until"`
}

// ScenarioRow regroupe les alertes d'un scénario (et d'un type) sur la période.
type ScenarioRow struct {
	Scenario string    `json:"scenario"`
	Kind     string    `json:"kind"`
	Detail   string    `json:"detail,omitempty"`
	Alerts   int       `json:"alerts"`
	IPs      int       `json:"ips"`
	Last     time.Time `json:"last"`
	Hosts    []string  `json:"hosts"` // les plus visés d'abord, 3 au plus
}

type View struct {
	// Enabled : compte machine configuré, donc alertes lisibles.
	Enabled bool `json:"enabled"`
	// Error : dernière lecture des alertes en échec (l'historique affiché peut
	// être en retard).
	Error     string     `json:"error,omitempty"`
	Bans      []BanRow   `json:"bans"`
	Blocklist *Blocklist `json:"blocklist"`
	Counts    struct {
		Bans   int `json:"bans"`   // alertes ayant posé un ban
		WAF    int `json:"waf"`    // requêtes bloquées par le WAF, sans ban
		Other  int `json:"other"`  // alertes sans décision
		IPs    int `json:"ips"`    // IP distinctes
		Manual int `json:"manual"` // bans posés à la main
	} `json:"counts"`
	Scenarios []ScenarioRow `json:"scenarios"`
	Recent    []Alert       `json:"recent"`
}

const (
	maxScenarios = 15
	maxRecent    = 12
	maxIPAlerts  = 30
)

// View assemble le bloc CrowdSec d'une période glissante.
func (f *Feed) View(win time.Duration, now time.Time, bans map[netip.Addr]store.Ban, lookup func(netip.Addr) geo.Info) View {
	return f.ViewRange(now.Add(-win), now.Add(time.Minute), bans, lookup)
}

// ViewRange assemble le bloc CrowdSec des alertes de [from, to] ; les bans sont
// ceux en cours, quel que soit l'intervalle.
func (f *Feed) ViewRange(from, to time.Time, bans map[netip.Addr]store.Ban, lookup func(netip.Addr) geo.Info) View {
	f.mu.RLock()
	defer f.mu.RUnlock()
	v := View{Enabled: f.enabled, Blocklist: f.blocklist, Bans: []BanRow{}, Scenarios: []ScenarioRow{}, Recent: []Alert{}}
	if f.alertsErr != nil {
		v.Error = f.alertsErr.Error()
	}

	for ip, b := range bans {
		row := BanRow{IP: ip.String(), Scenario: b.Scenario, Manual: b.Manual, Until: b.Until}
		if lookup != nil {
			g := lookup(ip)
			row.Country, row.Org = g.Country, g.Org
		}
		v.Bans = append(v.Bans, row)
	}
	slices.SortFunc(v.Bans, func(a, b BanRow) int {
		return cmp.Or(b.Until.Compare(a.Until), strings.Compare(a.IP, b.IP))
	})

	type agg struct {
		row   ScenarioRow
		ips   map[string]struct{}
		hosts map[string]int
	}
	groups := map[[2]string]*agg{}
	ips := map[string]struct{}{}
	var recent []Alert
	for _, a := range f.alerts {
		if a.At.Before(from) || a.At.After(to) {
			continue
		}
		recent = append(recent, a)
		ips[a.IP] = struct{}{}
		switch a.Kind {
		case "ban":
			v.Counts.Bans++
			if a.Manual {
				v.Counts.Manual++
			}
		case "waf":
			v.Counts.WAF++
		default:
			v.Counts.Other++
		}
		key := [2]string{a.Scenario, a.Kind}
		g := groups[key]
		if g == nil {
			g = &agg{row: ScenarioRow{Scenario: a.Scenario, Kind: a.Kind}, ips: map[string]struct{}{}, hosts: map[string]int{}}
			groups[key] = g
		}
		g.row.Alerts++
		g.ips[a.IP] = struct{}{}
		if a.Host != "" {
			g.hosts[a.Host]++
		}
		if a.At.After(g.row.Last) {
			g.row.Last = a.At
		}
		if g.row.Detail == "" {
			g.row.Detail = a.Detail
		}
	}
	v.Counts.IPs = len(ips)

	for _, g := range groups {
		g.row.IPs = len(g.ips)
		hosts := make([]string, 0, len(g.hosts))
		for h := range g.hosts {
			hosts = append(hosts, h)
		}
		slices.SortFunc(hosts, func(a, b string) int { return cmp.Or(g.hosts[b]-g.hosts[a], strings.Compare(a, b)) })
		if len(hosts) > 3 {
			hosts = hosts[:3]
		}
		g.row.Hosts = hosts
		v.Scenarios = append(v.Scenarios, g.row)
	}
	slices.SortFunc(v.Scenarios, func(a, b ScenarioRow) int {
		return cmp.Or(b.Alerts-a.Alerts, b.Last.Compare(a.Last), strings.Compare(a.Scenario, b.Scenario))
	})
	if len(v.Scenarios) > maxScenarios {
		v.Scenarios = v.Scenarios[:maxScenarios]
	}

	sortRecent(recent)
	if len(recent) > maxRecent {
		recent = recent[:maxRecent]
	}
	if recent != nil {
		v.Recent = recent
	}
	return v
}

func sortRecent(a []Alert) {
	slices.SortFunc(a, func(x, y Alert) int { return cmp.Or(y.At.Compare(x.At), cmp.Compare(y.ID, x.ID)) })
}

// ForIP renvoie l'historique CrowdSec d'une IP (7 jours), le plus récent d'abord.
func (f *Feed) ForIP(ip netip.Addr) []Alert {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := []Alert{}
	s := ip.String()
	for _, a := range f.alerts {
		if a.IP == s {
			out = append(out, a)
		}
	}
	sortRecent(out)
	if len(out) > maxIPAlerts {
		out = out[:maxIPAlerts]
	}
	return out
}
