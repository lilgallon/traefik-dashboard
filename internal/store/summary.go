package store

import (
	"cmp"
	"errors"
	"net/netip"
	"slices"
	"sort"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/logline"
)

// Verdicts d'une IP sur la période.
const (
	VerdictKnown   = "known"   // toutes ses requêtes sont couvertes par une règle de nom
	VerdictSuccess = "success" // au moins un succès hors URL anonyme : rouge
	VerdictPublic  = "public"  // succès uniquement sur des URL anonymes
	VerdictBlocked = "blocked" // des 403, aucun autre 4xx, aucun succès
	VerdictProbing = "probing" // des 4xx hors 403, aucun succès
	VerdictOther   = "other"   // ni succès ni 4xx (redirections, 5xx, abandons)
)

const (
	maxRows         = 100
	maxNamedRows    = 40
	maxOffenders    = 50
	maxOffenderURLs = 5
	maxDetailURLs   = 50
)

// ClassCounts compte par famille de statut ; toutes les familles sont présentes.
type ClassCounts map[logline.Class]int64

func newCounts() ClassCounts {
	c := make(ClassCounts, nClasses)
	for _, k := range logline.Classes {
		c[k] = 0
	}
	return c
}

func (c ClassCounts) add(n *[nClasses]uint32) {
	for i, k := range logline.Classes {
		c[k] += int64(n[i])
	}
}

func (c ClassCounts) total() (t int64) {
	for _, v := range c {
		t += v
	}
	return t
}

type Summary struct {
	Window      string       `json:"window"` // "" pour un intervalle choisi
	Range       *Range       `json:"range,omitempty"`
	GeneratedAt time.Time    `json:"generatedAt"`
	LastLine    time.Time    `json:"lastLine"`
	Verdict     Verdict      `json:"verdict"`
	KPIs        KPIs         `json:"kpis"`
	Series      Series       `json:"series"`
	IPs         []IPRow      `json:"ips"`
	URLs        []URLRow     `json:"urls"`
	Services    []ServiceRow `json:"services"`
	Sensitive   []Sensitive  `json:"sensitive"`
	Counts      Counts       `json:"counts"`
	Warnings    []string     `json:"warnings"`
	// Capped : le plafond mémoire a été atteint sur la période (l'API en fait un
	// avertissement traduit).
	Capped bool `json:"-"`
	// Renseignés par l'API.
	Loading bool `json:"loading"` // relecture des archives en cours
}

type Verdict struct {
	TotalIPs       int        `json:"totalIPs"`
	TotalSuccesses int64      `json:"totalSuccesses"`
	Red            bool       `json:"red"`
	Offenders      []Offender `json:"offenders"`
}

// Offender est une IP inconnue qui a obtenu un succès hors URL anonymes.
type Offender struct {
	IP        string   `json:"ip"`
	Country   string   `json:"country"`
	Org       string   `json:"org"`
	ASN       uint     `json:"asn"`
	LooksLike string   `json:"looksLike"`
	Count     int64    `json:"count"`
	URLs      []string `json:"urls"` // hôte + chemin, les plus appelées d'abord
}

type KPIs struct {
	AvgRPS     float64     `json:"avgRps"`
	PeakRPS    float64     `json:"peakRps"`
	BytesIn    int64       `json:"bytesIn"`
	BytesOut   int64       `json:"bytesOut"`
	ByClass    ClassCounts `json:"byClass"`
	ActiveBans int         `json:"activeBans"` // renseigné par l'API ; -1 si CrowdSec est injoignable
}

// Series : requêtes par seconde, par famille de statut.
type Series struct {
	StepSeconds int64                       `json:"stepSeconds"`
	T           []int64                     `json:"t"` // début de chaque pas, secondes Unix
	ByClass     map[logline.Class][]float64 `json:"byClass"`
}

type IPRow struct {
	IP        string      `json:"ip"`
	Name      string      `json:"name"`
	LooksLike string      `json:"looksLike"`
	Country   string      `json:"country"`
	Org       string      `json:"org"`
	ASN       uint        `json:"asn"`
	ByClass   ClassCounts `json:"byClass"`
	Successes int64       `json:"successes"` // succès d'inconnue hors URL anonymes
	BytesIn   int64       `json:"bytesIn"`
	BytesOut  int64       `json:"bytesOut"`
	Verdict   string      `json:"verdict"`
	Ban       *Ban        `json:"ban"` // renseigné par l'API
}

// Ban est une décision CrowdSec en cours.
type Ban struct {
	Scenario string    `json:"scenario"`
	Until    time.Time `json:"until"`
	Manual   bool      `json:"manual"` // posé à la main (cscli ou cette page)
}

type URLRow struct {
	Host    string      `json:"host"`
	Path    string      `json:"path"`
	ByClass ClassCounts `json:"byClass"`
	// Sockets : WebSockets acceptés. Traefik les journalise avec le statut 0 : ils
	// sont aussi comptés dans ByClass["abandon"].
	Sockets int64 `json:"sockets"`
}

// urlAgg cumule les cellules d'une URL.
type urlAgg struct {
	n       [nClasses]uint32
	sockets uint32
}

func (u *urlAgg) add(c *cell) {
	for i, v := range c.n {
		u.n[i] += v
	}
	// success = 2xx + 101 + connexions longues sans statut.
	if c.success > c.n[1] {
		u.sockets += c.success - c.n[1]
	}
}

type ServiceRow struct {
	Router   string  `json:"router"`
	Requests int64   `json:"requests"`
	Err5xx   int64   `json:"err5xx"`
	P95Ms    float64 `json:"p95Ms"`
}

// Sensitive : chemin que cherchent les scanners, servi en 2xx à une IP non nommée.
type Sensitive struct {
	IP     string    `json:"ip"`
	Host   string    `json:"host"`
	Path   string    `json:"path"`
	Status int       `json:"status"`
	Count  int64     `json:"count"`
	Last   time.Time `json:"last"`
}

// Counts : tailles réelles, les tableaux étant plafonnés.
type Counts struct {
	IPs        int `json:"ips"`
	NamedIPs   int `json:"namedIps"`
	UnknownIPs int `json:"unknownIps"`
	URLs       int `json:"urls"`
}

type IPDetail struct {
	IPRow
	URLs       []URLRow         `json:"urls"`
	Routers    map[string]int64 `json:"routers"`
	Rules      map[string]int64 `json:"rules"`      // règles de nom qui ont couvert des requêtes
	UserAgents []string         `json:"userAgents"` // récents, pas limités à la période
	RawPaths   []RawHit         `json:"rawPaths"`   // requêtes non nommées récentes, la plus récente d'abord
}

type LiveTick struct {
	T        int64       `json:"t"`
	ByClass  ClassCounts `json:"byClass"`
	BytesIn  int64       `json:"bytesIn"`
	BytesOut int64       `json:"bytesOut"`
	LastLine time.Time   `json:"lastLine"`
}

// ipAgg cumule les cellules d'une IP sur la période.
type ipAgg struct {
	ip      netip.Addr
	n       [nClasses]uint32
	in, out uint64
	total   int64
	named   int64
	names   map[handle]int64
	uRed    int64 // succès d'inconnue hors URL anonymes
	uPublic int64 // succès d'inconnue sur URL anonymes
	u403    int64
	u4xx    int64
	redURLs map[urlKey]int64
	urls    map[urlKey]*urlAgg // détail d'IP seulement
	routers map[handle]int64   // détail d'IP seulement
}

func (a *ipAgg) add(c *cell, detail bool) {
	var tot int64
	for i, v := range c.n {
		a.n[i] += v
		tot += int64(v)
	}
	a.in += c.in
	a.out += c.out
	a.total += tot
	if c.name != hEmpty {
		a.named += tot
		if a.names == nil {
			a.names = make(map[handle]int64, 1)
		}
		a.names[c.name] += tot
	} else {
		if c.anon {
			a.uPublic += int64(c.success)
		} else if c.success > 0 {
			a.uRed += int64(c.success)
			if a.redURLs == nil {
				a.redURLs = make(map[urlKey]int64, 2)
			}
			a.redURLs[urlKey{c.host, c.path}] += int64(c.success)
		}
		a.u403 += int64(c.n403)
		a.u4xx += int64(c.n[3])
	}
	if detail {
		k := urlKey{c.host, c.path}
		u := a.urls[k]
		if u == nil {
			u = new(urlAgg)
			a.urls[k] = u
		}
		u.add(c)
		a.routers[c.router] += tot
	}
}

func (a *ipAgg) verdict() string {
	switch {
	case a.named == a.total:
		return VerdictKnown
	case a.uRed > 0:
		return VerdictSuccess
	case a.uPublic > 0:
		return VerdictPublic
	case a.u4xx > a.u403:
		return VerdictProbing
	case a.u403 > 0:
		return VerdictBlocked
	}
	return VerdictOther
}

// mainName : règle qui a couvert le plus de requêtes.
func (a *ipAgg) mainName() string {
	var best handle
	var bestN int64 = -1
	for h, n := range a.names {
		if n > bestN || (n == bestN && h.Value() < best.Value()) {
			best, bestN = h, n
		}
	}
	if bestN < 0 {
		return ""
	}
	return best.Value()
}

func (s *Store) row(a *ipAgg) IPRow {
	r := IPRow{
		IP:        OthersIP,
		ByClass:   newCounts(),
		Successes: a.uRed,
		BytesIn:   int64(a.in),
		BytesOut:  int64(a.out),
		Verdict:   a.verdict(),
	}
	r.ByClass.add(&a.n)
	if !a.ip.IsValid() {
		return r
	}
	r.IP = a.ip.String()
	var looks string
	if s.Enrich != nil {
		g, l := s.Enrich(a.ip)
		r.Country, r.Org, r.ASN = g.Country, g.Org, g.ASN
		looks = l
	}
	if r.Verdict == VerdictKnown {
		r.Name = a.mainName()
	} else if a.named > 0 {
		r.LooksLike = a.mainName()
	} else {
		r.LooksLike = looks
	}
	return r
}

func urlString(k urlKey) string {
	return k.host.Value() + k.path.Value()
}

func topRedURLs(m map[urlKey]int64) []string {
	type kv struct {
		u string
		n int64
	}
	all := make([]kv, 0, len(m))
	for k, n := range m {
		all = append(all, kv{urlString(k), n})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].n != all[j].n {
			return all[i].n > all[j].n
		}
		return all[i].u < all[j].u
	})
	if len(all) > maxOffenderURLs {
		all = all[:maxOffenderURLs]
	}
	out := make([]string, len(all))
	for i, e := range all {
		out[i] = e.u
	}
	return out
}

// eachMinute parcourt les minutes de la période (verrou tenu par l'appelant).
func (s *Store) eachMinute(from, to int64, fn func(mk int64, m *minute)) {
	if int64(len(s.minutes)) < to-from {
		for mk, m := range s.minutes {
			if mk >= from && mk <= to {
				fn(mk, m)
			}
		}
		return
	}
	for mk := from; mk <= to; mk++ {
		if m := s.minutes[mk]; m != nil {
			fn(mk, m)
		}
	}
}

// Range est un intervalle choisi sur le graphique : [From, To[, secondes Unix.
type Range struct {
	From int64 `json:"from"`
	To   int64 `json:"to"`
}

// plan décrit ce qu'un résumé couvre : minutes des cellules et forme de la série.
type plan struct {
	label    string // Summary.Window ("" pour un intervalle)
	rng      *Range // intervalle effectif (arrondi aux pas de la série)
	from, to int64  // minutes incluses
	start    int64  // début du premier pas de la série, secondes Unix
	points   int
	step     int64
	seconds  bool    // série tirée de l'anneau des secondes
	dur      float64 // durée de la moyenne, en secondes
}

func windowPlan(w Window, now time.Time) plan {
	from, to := minuteRange(w, now)
	p := plan{label: string(w), from: from, to: to, step: w.Step(), dur: w.Duration().Seconds()}
	switch w {
	case W1h:
		p.seconds = true
		p.start = from * 60
		p.points = int(now.Unix() - p.start + 1)
	case W7d:
		p.points = 168
		p.start = from * 60
	default:
		p.points = int(to - from + 1)
		p.start = from * 60
	}
	return p
}

// Pas de la série d'un intervalle choisi, selon sa longueur.
const (
	rangeSecondsMax = 3600          // à la seconde jusqu'à 1 h (si encore dans l'anneau)
	rangeMinutesMax = 2 * 24 * 3600 // à la minute jusqu'à 48 h, à l'heure au-delà
)

// ErrRange : intervalle vide, inversé ou entièrement hors des 7 jours gardés.
var ErrRange = errors.New("invalid range")

func rangePlan(r Range, now time.Time) (plan, error) {
	nowSec := now.Unix()
	r.To = min(r.To, nowSec+1)
	r.From = max(r.From, now.Add(-retention).Unix())
	if r.To <= r.From {
		return plan{}, ErrRange
	}
	// Les cellules sont à la minute : l'intervalle s'arrondit aux minutes entières
	// (à l'heure pour une série horaire), pour que série, indicateurs et tableaux
	// couvrent exactement la même chose.
	unit := int64(60)
	step := int64(60)
	span := r.To - r.From
	lastSec := now.Add(-LiveLag).Unix() // dernière seconde complète
	if span > rangeMinutesMax {
		unit, step = 3600, 3600
	}
	from := r.From / unit * unit
	to := (r.To + unit - 1) / unit * unit
	seconds := span <= rangeSecondsMax && from > lastSec-secRing
	if seconds {
		step = 1
	}
	end := min(to, nowSec+1) // fin des données
	if seconds {
		end = min(to, lastSec+1)
	}
	p := plan{
		rng:     &Range{From: from, To: to},
		from:    from / 60,
		to:      to/60 - 1,
		start:   from,
		step:    step,
		seconds: seconds,
		points:  int(max((end-from+step-1)/step, 1)),
		dur:     float64(max(min(to, nowSec)-from, 1)),
	}
	return p, nil
}

// Summary calcule tout ce que la page affiche pour une période.
func (s *Store) Summary(w Window, now time.Time) Summary {
	return s.summarize(windowPlan(w, now), now)
}

// SummaryRange : même chose sur un intervalle choisi sur le graphique.
func (s *Store) SummaryRange(r Range, now time.Time) (Summary, error) {
	p, err := rangePlan(r, now)
	if err != nil {
		return Summary{}, err
	}
	return s.summarize(p, now), nil
}

func (s *Store) summarize(p plan, now time.Time) Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	from, to := p.from, p.to
	sum := Summary{
		Window:      p.label,
		Range:       p.rng,
		GeneratedAt: now.UTC(),
		LastLine:    s.lastLine,
		Warnings:    []string{},
		Sensitive:   []Sensitive{},
	}
	sum.KPIs.ByClass = newCounts()

	// ── Série et indicateurs ────────────────────────────────────────────────────
	step, points, start := p.step, p.points, p.start
	series := Series{StepSeconds: step, ByClass: make(map[logline.Class][]float64, nClasses)}
	series.T = make([]int64, points)
	cols := make([][]float64, nClasses)
	for i := range cols {
		cols[i] = make([]float64, points)
		series.ByClass[logline.Classes[i]] = cols[i]
	}
	for i := range series.T {
		series.T[i] = start + int64(i)*step
	}

	var peak uint32
	var totals [nClasses]uint32
	var bytesIn, bytesOut uint64
	if p.seconds {
		for i := 0; i < points; i++ {
			ts := start + int64(i)
			sec := &s.seconds[ts%secRing]
			if sec.t != ts {
				continue
			}
			var tot uint32
			for c, v := range sec.n {
				cols[c][i] = float64(v)
				totals[c] += v
				tot += v
			}
			bytesIn += sec.in
			bytesOut += sec.out
			peak = max(peak, tot)
		}
	} else {
		nowSec := now.Unix()
		s.eachMinute(from, to, func(mk int64, m *minute) {
			i := int((mk*60 - start) / step)
			if i < 0 || i >= points {
				return
			}
			for c, v := range m.n {
				cols[c][i] += float64(v)
				totals[c] += v
			}
			bytesIn += m.in
			bytesOut += m.out
			peak = max(peak, m.peak)
		})
		// Comptes → requêtes par seconde ; le dernier pas est en cours.
		for i := 0; i < points; i++ {
			d := float64(step)
			if end := series.T[i] + step; end > nowSec {
				d = float64(max(nowSec-series.T[i], 1))
			}
			for c := range cols {
				cols[c][i] /= d
			}
		}
	}
	sum.Series = series
	sum.KPIs.ByClass.add(&totals)
	sum.KPIs.BytesIn, sum.KPIs.BytesOut = int64(bytesIn), int64(bytesOut)
	sum.KPIs.AvgRPS = float64(sum.KPIs.ByClass.total()) / p.dur
	sum.KPIs.PeakRPS = float64(peak)

	// ── IP, URL, services ───────────────────────────────────────────────────────
	ips := make(map[netip.Addr]*ipAgg, 256)
	urls := make(map[urlKey]*urlAgg, 256)
	routers := make(map[handle]*routerStat, 16)
	s.eachMinute(from, to, func(_ int64, m *minute) {
		for i := range m.cells {
			c := &m.cells[i]
			a := ips[c.ip]
			if a == nil {
				a = &ipAgg{ip: c.ip}
				ips[c.ip] = a
			}
			a.add(c, false)
			k := urlKey{c.host, c.path}
			u := urls[k]
			if u == nil {
				u = new(urlAgg)
				urls[k] = u
			}
			u.add(c)
		}
		for h, rs := range m.routers {
			agg := routers[h]
			if agg == nil {
				agg = &routerStat{}
				routers[h] = agg
			}
			agg.req += rs.req
			agg.err5 += rs.err5
			for j, v := range rs.hist {
				agg.hist[j] += v
			}
		}
	})

	var named, unknown, red []*ipAgg
	for _, a := range ips {
		switch a.verdict() {
		case VerdictKnown:
			named = append(named, a)
		case VerdictSuccess:
			red = append(red, a)
			unknown = append(unknown, a)
		default:
			unknown = append(unknown, a)
		}
	}
	byIP := func(a, b *ipAgg) int { return a.ip.Compare(b.ip) }
	slices.SortFunc(named, func(a, b *ipAgg) int {
		return cmp.Or(cmp.Compare(b.total, a.total), byIP(a, b))
	})
	slices.SortFunc(unknown, func(a, b *ipAgg) int {
		return cmp.Or(
			cmp.Compare(b.uRed, a.uRed),
			cmp.Compare(b.uPublic, a.uPublic),
			cmp.Compare(b.total, a.total),
			byIP(a, b))
	})
	sum.Counts = Counts{IPs: len(ips), NamedIPs: len(named), UnknownIPs: len(unknown), URLs: len(urls)}

	sum.IPs = make([]IPRow, 0, min(len(ips), maxRows))
	for _, a := range named[:min(len(named), maxNamedRows)] {
		sum.IPs = append(sum.IPs, s.row(a))
	}
	for _, a := range unknown[:min(len(unknown), maxRows-len(sum.IPs))] {
		sum.IPs = append(sum.IPs, s.row(a))
	}

	sum.Verdict = Verdict{Red: len(red) > 0, Offenders: []Offender{}}
	slices.SortFunc(red, func(a, b *ipAgg) int {
		return cmp.Or(cmp.Compare(b.uRed, a.uRed), byIP(a, b))
	})
	sum.Verdict.TotalIPs = len(red)
	for _, a := range red {
		sum.Verdict.TotalSuccesses += a.uRed
	}
	for _, a := range red[:min(len(red), maxOffenders)] {
		r := s.row(a)
		sum.Verdict.Offenders = append(sum.Verdict.Offenders, Offender{
			IP: r.IP, Country: r.Country, Org: r.Org, ASN: r.ASN, LooksLike: r.LooksLike,
			Count: a.uRed, URLs: topRedURLs(a.redURLs),
		})
	}

	sum.URLs = topURLs(urls, maxRows)

	sum.Services = make([]ServiceRow, 0, len(routers))
	for h, rs := range routers {
		sum.Services = append(sum.Services, ServiceRow{
			Router: h.Value(), Requests: int64(rs.req), Err5xx: int64(rs.err5), P95Ms: p95(&rs.hist),
		})
	}
	sort.Slice(sum.Services, func(i, j int) bool {
		a, b := sum.Services[i], sum.Services[j]
		if a.Requests != b.Requests {
			return a.Requests > b.Requests
		}
		return a.Router < b.Router
	})

	// ── Chemins sensibles servis ────────────────────────────────────────────────
	type sensitiveKey struct {
		ip, host, path string
		status         int
	}
	hits := map[sensitiveKey]Sensitive{}
	for _, h := range s.sensitive {
		if h.t/60 < from || h.t/60 > to {
			continue
		}
		k := sensitiveKey{h.ip, h.host, h.path, h.status}
		v := hits[k]
		at := time.Unix(h.t, 0).UTC()
		if at.After(v.Last) {
			v.Last = at
		}
		v.IP, v.Host, v.Path, v.Status = h.ip, h.host, h.path, h.status
		v.Count += h.count
		hits[k] = v
	}
	for _, v := range hits {
		sum.Sensitive = append(sum.Sensitive, v)
	}
	slices.SortFunc(sum.Sensitive, func(a, b Sensitive) int {
		return cmp.Or(b.Last.Compare(a.Last), cmp.Compare(a.IP, b.IP), cmp.Compare(a.Host, b.Host), cmp.Compare(a.Path, b.Path), cmp.Compare(a.Status, b.Status))
	})
	sum.Sensitive = sum.Sensitive[:min(20, len(sum.Sensitive))]
	if s.capped != 0 && s.capped >= from {
		sum.Capped = true
	}
	return sum
}

func topURLs(urls map[urlKey]*urlAgg, limit int) []URLRow {
	type entry struct {
		k     urlKey
		u     *urlAgg
		total uint64
	}
	all := make([]entry, 0, len(urls))
	for k, u := range urls {
		var t uint64
		for _, v := range u.n {
			t += uint64(v)
		}
		all = append(all, entry{k, u, t})
	}
	slices.SortFunc(all, func(a, b entry) int {
		return cmp.Or(cmp.Compare(b.total, a.total), cmp.Compare(urlString(a.k), urlString(b.k)))
	})
	if len(all) > limit {
		all = all[:limit]
	}
	out := make([]URLRow, len(all))
	for i, e := range all {
		c := newCounts()
		c.add(&e.u.n)
		out[i] = URLRow{Host: e.k.host.Value(), Path: e.k.path.Value(), ByClass: c, Sockets: int64(e.u.sockets)}
	}
	return out
}

// p95 approché : interpolation linéaire dans le bucket qui contient le 95e centile.
func p95(hist *[len(durBounds) + 1]uint32) float64 {
	var total uint64
	for _, v := range hist {
		total += uint64(v)
	}
	if total == 0 {
		return 0
	}
	target := 0.95 * float64(total)
	var cum float64
	for i, v := range hist {
		if v == 0 {
			continue
		}
		if cum+float64(v) >= target {
			lo := 0.0
			if i > 0 {
				lo = durBounds[i-1]
			}
			if i >= len(durBounds) {
				return durBounds[len(durBounds)-1] // au-delà du dernier bucket : borne basse
			}
			hi := durBounds[i]
			return lo + (hi-lo)*(target-cum)/float64(v)
		}
		cum += float64(v)
	}
	return durBounds[len(durBounds)-1]
}

// IPDetail détaille une IP sur la période ; ok est faux si elle n'y apparaît pas.
func (s *Store) IPDetail(ip netip.Addr, w Window, now time.Time) (IPDetail, bool) {
	return s.ipDetail(ip, windowPlan(w, now), false)
}

// IPDetailRange : même chose sur un intervalle choisi sur le graphique ; les
// dernières requêtes brutes sont limitées à l'intervalle.
func (s *Store) IPDetailRange(ip netip.Addr, r Range, now time.Time) (IPDetail, bool, error) {
	p, err := rangePlan(r, now)
	if err != nil {
		return IPDetail{}, false, err
	}
	d, ok := s.ipDetail(ip, p, true)
	return d, ok, nil
}

func (s *Store) ipDetail(ip netip.Addr, p plan, clipRaw bool) (IPDetail, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	from, to := p.from, p.to
	a := &ipAgg{ip: ip, urls: make(map[urlKey]*urlAgg), routers: make(map[handle]int64)}
	s.eachMinute(from, to, func(_ int64, m *minute) {
		for i := range m.cells {
			if m.cells[i].ip == ip {
				a.add(&m.cells[i], true)
			}
		}
	})
	if a.total == 0 {
		return IPDetail{}, false
	}
	d := IPDetail{
		IPRow:      s.row(a),
		URLs:       topURLs(a.urls, maxDetailURLs),
		Routers:    make(map[string]int64, len(a.routers)),
		Rules:      make(map[string]int64, len(a.names)),
		UserAgents: []string{},
		RawPaths:   []RawHit{},
	}
	for h, n := range a.routers {
		d.Routers[h.Value()] = n
	}
	for h, n := range a.names {
		d.Rules[h.Value()] = n
	}
	if x := s.extras[ip]; x != nil {
		d.UserAgents = append(d.UserAgents, x.uas...)
		for i := len(x.raw) - 1; i >= 0; i-- {
			if t := x.raw[i].Time.Unix() / 60; clipRaw && (t < from || t > to) {
				continue
			}
			d.RawPaths = append(d.RawPaths, x.raw[i])
		}
	}
	return d, true
}

// Second renvoie les compteurs de la dernière seconde complète (now - LiveLag).
func (s *Store) Second(now time.Time) LiveTick {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ts := now.Add(-LiveLag).Unix()
	tick := LiveTick{T: ts, ByClass: newCounts(), LastLine: s.lastLine}
	if sec := &s.seconds[ts%secRing]; sec.t == ts {
		tick.ByClass.add(&sec.n)
		tick.BytesIn, tick.BytesOut = int64(sec.in), int64(sec.out)
	}
	return tick
}
