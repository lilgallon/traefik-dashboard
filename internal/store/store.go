// Package store agrège les requêtes en mémoire : compteurs par seconde pour la
// dernière heure, et par minute sur 7 jours.
//
// Chaque minute garde des « cellules » : un compteur par (IP, hôte, chemin
// normalisé, router). Tout le reste (lignes d'IP, URL, verdicts) est recalculé à
// la demande à partir des cellules, ce qui permet de renommer une IP après coup
// (Reclassify) sans relire les logs.
package store

import (
	"net/netip"
	"sync"
	"time"
	"unique"

	"github.com/lilgallon/traefik-dashboard/internal/geo"
	"github.com/lilgallon/traefik-dashboard/internal/logline"
)

// Identity est ce que les règles de noms disent d'UNE requête.
type Identity struct {
	Name      string   // nom de la règle qui couvre la requête, "" si inconnue
	LooksLike string   // règle dont seule la condition IP/ASN correspond
	Anonymous bool     // URL anonyme par conception
	Geo       geo.Info // pays et opérateur de l'IP
}

// Window est une période d'affichage.
type Window string

const (
	W1h  Window = "1h"
	W24h Window = "24h"
	W7d  Window = "7d"
)

// Windows liste les périodes proposées.
var Windows = []Window{W1h, W24h, W7d}

func ParseWindow(s string) (Window, bool) {
	switch Window(s) {
	case W1h, W24h, W7d:
		return Window(s), true
	}
	return "", false
}

func (w Window) Duration() time.Duration {
	switch w {
	case W1h:
		return time.Hour
	case W7d:
		return 7 * 24 * time.Hour
	}
	return 24 * time.Hour
}

// Step est le pas de la série, en secondes.
func (w Window) Step() int64 {
	switch w {
	case W1h:
		return 1
	case W7d:
		return 3600
	}
	return 60
}

const (
	// Plafonds de cardinalité par minute. Au-delà, les requêtes sont regroupées
	// (IP « autres », chemin « (autres) ») ; les compteurs globaux restent exacts.
	maxIPsPerMinute  = 2000
	maxURLsPerMinute = 2000
	// Budget global de cellules (≈ 130 octets chacune). Une fois atteint, chaque
	// nouvelle minute n'a plus droit qu'à minCellsPerMinute cellules détaillées.
	defaultMaxCells   = 400_000
	minCellsPerMinute = 50

	retention = 7 * 24 * time.Hour

	secRing = 3600

	// IP dont on garde user-agents et derniers chemins bruts.
	maxExtras  = 3000
	maxUAs     = 5
	maxRawHits = 30
	maxStrLen  = 200

	maxSensitive = 200

	// Dernières requêtes gardées pour le bloc « en direct ».
	maxRecent = 200

	// OthersIP et OthersPath désignent les requêtes regroupées hors plafond.
	OthersIP   = "autres"
	OthersPath = "(autres)"
	// NoRouter : requêtes qu'aucun router n'a prises (hôte inconnu).
	NoRouter = "(aucun)"
)

// LiveLag : le suivi du fichier sonde toutes les 500 ms, donc une seconde n'est
// complète qu'un peu après sa fin. Ticks et série « 1 h » s'arrêtent à now-LiveLag.
const LiveLag = 2 * time.Second

type handle = unique.Handle[string]

var (
	hEmpty  = unique.Make("")
	hOthers = unique.Make(OthersPath)
)

const nClasses = 6

func classIndex(status int) int {
	switch {
	case status >= 100 && status < 200:
		return 0
	case status >= 200 && status < 300:
		return 1
	case status >= 300 && status < 400:
		return 2
	case status >= 400 && status < 500:
		return 3
	case status >= 500 && status < 600:
		return 4
	}
	return 5
}

type cellKey struct {
	ip                 netip.Addr // invalide = IP regroupées
	host, path, router handle
	// Pour une cellule regroupée, l'identité ne peut plus être recalculée à partir
	// de la clé : elle en fait partie, figée à l'ingestion.
	fixed     bool
	fixedName handle
	fixedAnon bool
}

type cell struct {
	cellKey
	name    handle // règle qui couvre ces requêtes, hEmpty si inconnues
	anon    bool
	n       [nClasses]uint32
	success uint32 // 2xx et 101
	n403    uint32
	in, out uint64
}

type urlKey struct{ host, path handle }

// Buckets de durée (bornes hautes, en ms) pour le p95 par router.
var durBounds = [...]float64{1, 2, 5, 10, 20, 50, 100, 200, 500, 1000, 2000, 5000, 10000, 30000, 60000}

type routerStat struct {
	req, err5 uint32
	hist      [len(durBounds) + 1]uint32
}

type minute struct {
	n       [nClasses]uint32
	in, out uint64
	peak    uint32 // plus forte seconde de la minute
	red     uint32 // succès d'inconnues hors URL anonymes
	cells   []cell
	routers map[handle]*routerStat

	// Présents seulement tant que la minute reçoit des lignes (voir seal).
	index map[cellKey]int32
	ips   map[netip.Addr]struct{}
	urls  map[urlKey]struct{}
}

type second struct {
	t       int64
	n       [nClasses]uint32
	in, out uint64
}

// RawHit est une requête brute (avec query string) d'une IP, pour l'identifier.
type RawHit struct {
	Time   time.Time `json:"time"`
	Method string    `json:"method"`
	Host   string    `json:"host"`
	Path   string    `json:"path"`
	Status int       `json:"status"`
}

type extra struct {
	lastSeen int64 // minute
	uas      []string
	raw      []RawHit // du plus ancien au plus récent
}

// Hit est une des dernières requêtes lues, pour le bloc « en direct ».
type Hit struct {
	Seq     uint64    `json:"seq"` // croissant, repart de 1 au redémarrage du service
	Time    time.Time `json:"time"`
	IP      string    `json:"ip"`
	Name    string    `json:"name"` // "" si inconnue
	Country string    `json:"country"`
	Method  string    `json:"method"`
	Host    string    `json:"host"`
	// Chemin normalisé pour une requête nommée, brut (secrets masqués) sinon :
	// comme dans le détail d'une IP, la page n'a pas d'authentification.
	Path   string  `json:"path"`
	Status int     `json:"status"`
	Ms     float64 `json:"ms"`
	Socket bool    `json:"socket"` // WebSocket accepté (journalisé sans statut)
	Red    bool    `json:"red"`    // succès d'une inconnue hors URL anonymes
}

type recentHit struct {
	Hit
	ip                netip.Addr
	router, norm, raw string
	success           bool
}

func (h *recentHit) identify(id Identity) {
	h.Name = id.Name
	h.Red = h.success && id.Name == "" && !id.Anonymous
	if id.Name != "" {
		h.Path = h.norm
	} else {
		h.Path = h.raw
	}
}

type sensitiveHit struct {
	t              int64
	ip, host, path string
	status         int
	count          int64
}

// Store est sûr pour un usage concurrent.
type Store struct {
	// Enrich donne pays, opérateur et « ressemble à » d'une IP pour les lignes
	// renvoyées. Facultatif ; à poser avant usage concurrent.
	Enrich func(ip netip.Addr) (geo.Info, string)

	mu        sync.RWMutex
	minutes   map[int64]*minute
	open      []int64 // minutes qui ont encore leur index
	maxMinute int64
	seconds   [secRing]second
	lastLine  time.Time
	cells     int
	maxCells  int
	capped    int64 // dernière minute où le budget global a forcé un regroupement
	extras    map[netip.Addr]*extra
	sensitive []sensitiveHit
	recent    [maxRecent]recentHit // anneau : la requête n est en (n-1) % maxRecent
	recentSeq uint64
}

func New() *Store {
	return &Store{
		minutes:  make(map[int64]*minute),
		extras:   make(map[netip.Addr]*extra),
		maxCells: defaultMaxCells,
	}
}

// Add compte une requête.
func (s *Store) Add(e logline.Entry, id Identity) {
	ts := e.Time.Unix()
	mk := ts / 60
	ci := classIndex(e.Status)
	in, out := uint64(max(e.BytesIn, 0)), uint64(max(e.BytesOut, 0))
	success := e.Success()
	unknownRed := success && id.Name == "" && !id.Anonymous

	router := e.Router
	if router == "" {
		router = NoRouter
	}
	hRouter := unique.Make(router)
	rawPath := e.Path
	normPath := NormalizePath(rawPath)

	s.mu.Lock()
	defer s.mu.Unlock()

	if e.Time.After(s.lastLine) {
		s.lastLine = e.Time
	}

	m := s.minutes[mk]
	if m == nil {
		m = &minute{routers: make(map[handle]*routerStat, 4)}
		s.minutes[mk] = m
	}
	if mk > s.maxMinute {
		s.maxMinute = mk
		s.sealOld()
	}

	// Compteurs globaux : toujours exacts.
	m.n[ci]++
	m.in += in
	m.out += out
	if unknownRed {
		m.red++
	}

	// Seconde (dernière heure) et pic de la minute.
	sec := &s.seconds[ts%secRing]
	if sec.t < ts {
		*sec = second{t: ts}
	}
	if sec.t == ts {
		sec.n[ci]++
		sec.in += in
		sec.out += out
		var tot uint32
		for _, v := range sec.n {
			tot += v
		}
		m.peak = max(m.peak, tot)
	} else if m.peak == 0 {
		m.peak = 1
	}

	// Router : volume, 5xx, durées.
	rs := m.routers[hRouter]
	if rs == nil {
		rs = &routerStat{}
		m.routers[hRouter] = rs
	}
	rs.req++
	if ci == 4 {
		rs.err5++
	}
	// Ni WebSocket (des heures) ni abandon : ce ne sont pas des temps de réponse.
	if e.Status != 101 && e.Status != 0 {
		rs.hist[durBucket(e.Duration)]++
	}

	// Cellule.
	c := s.cellFor(m, mk, cellKey{ip: e.IP, host: unique.Make(e.Host), path: unique.Make(normPath), router: hRouter}, id)
	c.n[ci]++
	c.in += in
	c.out += out
	if success {
		c.success++
	}
	if e.Status == 403 {
		c.n403++
	}

	s.recentSeq++
	h := &s.recent[(s.recentSeq-1)%maxRecent]
	*h = recentHit{
		Hit: Hit{
			Seq: s.recentSeq, Time: e.Time, IP: e.IP.String(), Country: id.Geo.Country,
			Method: clip(e.Method), Host: clip(e.Host), Status: e.Status,
			Ms:     float64(e.Duration) / float64(time.Millisecond),
			Socket: e.Status == 0 && success,
		},
		ip: e.IP, router: e.Router, norm: clip(normPath), raw: clip(maskSecrets(rawPath)), success: success,
	}
	h.identify(id)

	s.addExtra(e, id, mk)
	if id.Name == "" && e.Status >= 200 && e.Status < 300 && IsSensitive(rawPath) {
		s.addSensitive(e, ts)
	}
}

// cellFor trouve ou crée la cellule de la requête, en regroupant si un plafond
// est atteint.
func (s *Store) cellFor(m *minute, mk int64, key cellKey, id Identity) *cell {
	m.ensureIndex()
	if i, ok := m.index[key]; ok {
		c := &m.cells[i]
		// Les règles ont pu changer depuis la création de la cellule.
		c.name, c.anon = unique.Make(id.Name), id.Anonymous
		return c
	}

	cellCap := maxIPsPerMinute + maxURLsPerMinute
	if s.cells >= s.maxCells {
		cellCap = minCellsPerMinute
	}
	_, ipKnown := m.ips[key.ip]
	uk := urlKey{key.host, key.path}
	_, urlKnown := m.urls[uk]
	overCells := len(m.cells) >= cellCap
	if overCells && s.cells >= s.maxCells {
		s.capped = mk
	}

	switch {
	case overCells || (!ipKnown && len(m.ips) >= maxIPsPerMinute):
		// Trop d'IP : tout part dans la ligne « autres ».
		key = cellKey{host: hEmpty, path: hOthers, router: key.router,
			fixed: true, fixedName: unique.Make(id.Name), fixedAnon: id.Anonymous}
	case !urlKnown && len(m.urls) >= maxURLsPerMinute:
		// Trop d'URL : l'IP reste suivie, le chemin est regroupé.
		key = cellKey{ip: key.ip, host: hEmpty, path: hOthers, router: key.router,
			fixed: true, fixedName: unique.Make(id.Name), fixedAnon: id.Anonymous}
		m.ips[key.ip] = struct{}{}
	default:
		m.ips[key.ip] = struct{}{}
		m.urls[uk] = struct{}{}
	}
	if key.fixed {
		if i, ok := m.index[key]; ok {
			return &m.cells[i]
		}
	}
	m.cells = append(m.cells, cell{cellKey: key, name: unique.Make(id.Name), anon: id.Anonymous})
	m.index[key] = int32(len(m.cells) - 1)
	s.cells++
	return &m.cells[len(m.cells)-1]
}

// ensureIndex (re)construit les index d'une minute ; nécessaire quand une ligne
// en retard arrive dans une minute déjà scellée.
func (m *minute) ensureIndex() {
	if m.index != nil {
		return
	}
	m.index = make(map[cellKey]int32, len(m.cells)+8)
	m.ips = make(map[netip.Addr]struct{})
	m.urls = make(map[urlKey]struct{})
	for i := range m.cells {
		c := &m.cells[i]
		m.index[c.cellKey] = int32(i)
		if c.ip.IsValid() {
			m.ips[c.ip] = struct{}{}
		}
		if !c.fixed {
			m.urls[urlKey{c.host, c.path}] = struct{}{}
		}
	}
}

// sealOld libère les index des minutes qui ne reçoivent plus de lignes : seules
// les cellules (tableau compact) restent en mémoire.
func (s *Store) sealOld() {
	keep := s.open[:0]
	for _, mk := range s.open {
		if mk >= s.maxMinute-1 {
			keep = append(keep, mk)
			continue
		}
		if m := s.minutes[mk]; m != nil {
			m.index, m.ips, m.urls = nil, nil, nil
			m.cells = append([]cell(nil), m.cells...) // rend la capacité inutilisée
		}
	}
	s.open = append(keep, s.maxMinute)
	// Les minutes rouvertes par une ligne en retard ne sont pas dans s.open : elles
	// sont rescellées par Prune.
}

func durBucket(d time.Duration) int {
	ms := float64(d) / float64(time.Millisecond)
	for i, b := range durBounds {
		if ms <= b {
			return i
		}
	}
	return len(durBounds)
}

func clip(s string) string {
	if len(s) > maxStrLen {
		return s[:maxStrLen] + "…"
	}
	return s
}

func (s *Store) addExtra(e logline.Entry, id Identity, mk int64) {
	x := s.extras[e.IP]
	if x == nil {
		if len(s.extras) >= maxExtras {
			return
		}
		x = &extra{}
		s.extras[e.IP] = x
	}
	x.lastSeen = max(x.lastSeen, mk)
	if e.UA != "" {
		ua := clip(e.UA)
		found := false
		for _, u := range x.uas {
			if u == ua {
				found = true
				break
			}
		}
		if !found && len(x.uas) < maxUAs {
			x.uas = append(x.uas, ua)
		}
	}
	if id.Name != "" {
		return
	}
	path := clip(maskSecrets(e.Path))
	if n := len(x.raw); n > 0 && x.raw[n-1].Path == path && x.raw[n-1].Status == e.Status && x.raw[n-1].Host == e.Host {
		x.raw[n-1].Time = e.Time
		return
	}
	if len(x.raw) >= maxRawHits {
		copy(x.raw, x.raw[1:])
		x.raw = x.raw[:maxRawHits-1]
	}
	x.raw = append(x.raw, RawHit{Time: e.Time, Method: e.Method, Host: e.Host, Path: path, Status: e.Status})
}

func (s *Store) addSensitive(e logline.Entry, ts int64) {
	ip, path := e.IP.String(), clip(e.Path)
	for i := range s.sensitive {
		h := &s.sensitive[i]
		if h.ip == ip && h.host == e.Host && h.path == path {
			h.count++
			h.t = max(h.t, ts)
			return
		}
	}
	if len(s.sensitive) >= maxSensitive {
		s.sensitive = s.sensitive[1:]
	}
	s.sensitive = append(s.sensitive, sensitiveHit{t: ts, ip: ip, host: e.Host, path: path, status: e.Status, count: 1})
}

// LastLine est l'heure de la requête la plus récente lue.
func (s *Store) LastLine() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastLine
}

// Prune supprime ce qui a plus de 7 jours et rescelle les minutes rouvertes.
func (s *Store) Prune(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Une heure de marge : la série « 7 j » est alignée sur les heures.
	cutoff := now.Add(-retention-time.Hour).Unix() / 60
	for mk, m := range s.minutes {
		if mk < cutoff {
			s.cells -= len(m.cells)
			delete(s.minutes, mk)
			continue
		}
		if m.index != nil && mk < s.maxMinute-1 {
			m.index, m.ips, m.urls = nil, nil, nil
		}
	}
	for ip, x := range s.extras {
		if x.lastSeen < cutoff {
			delete(s.extras, ip)
		}
	}
	// Plein : libérer les IP vues il y a le plus longtemps (ici, plus d'une heure).
	if len(s.extras) >= maxExtras {
		old := now.Add(-time.Hour).Unix() / 60
		for ip, x := range s.extras {
			if x.lastSeen < old {
				delete(s.extras, ip)
			}
		}
	}
	keep := s.sensitive[:0]
	for _, h := range s.sensitive {
		if h.t/60 >= cutoff {
			keep = append(keep, h)
		}
	}
	s.sensitive = keep
}

// Reclassify recalcule nom et anonymat de chaque cellule après un changement de
// règles : nommer une IP fait disparaître son rouge passé. Les cellules regroupées
// gardent l'identité de leur ingestion.
func (s *Store) Reclassify(f func(e logline.Entry) Identity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	type memoKey struct {
		ip           netip.Addr
		path, router handle
	}
	type memoVal struct {
		name handle
		anon bool
	}
	memo := make(map[memoKey]memoVal)
	for _, m := range s.minutes {
		m.red = 0
		for i := range m.cells {
			c := &m.cells[i]
			if !c.fixed {
				k := memoKey{c.ip, c.path, c.router}
				v, ok := memo[k]
				if !ok {
					router := c.router.Value()
					if router == NoRouter {
						router = ""
					}
					id := f(logline.Entry{IP: c.ip, Router: router, Host: c.host.Value(), Path: c.path.Value()})
					v = memoVal{unique.Make(id.Name), id.Anonymous}
					memo[k] = v
				}
				c.name, c.anon = v.name, v.anon
			}
			if c.name == hEmpty && !c.anon {
				m.red += c.success
			}
		}
	}
	for i := range s.recent {
		if h := &s.recent[i]; h.Seq != 0 {
			h.identify(f(logline.Entry{IP: h.ip, Router: h.router, Host: h.Host, Path: h.norm}))
		}
	}
}

// Recent renvoie les requêtes de numéro supérieur à after, de la plus ancienne à la
// plus récente, limitées aux limit dernières.
func (s *Store) Recent(after uint64, limit int) []Hit {
	s.mu.RLock()
	defer s.mu.RUnlock()
	first := after + 1
	if n := uint64(min(max(limit, 0), maxRecent)); s.recentSeq > n && first < s.recentSeq-n+1 {
		first = s.recentSeq - n + 1
	}
	out := make([]Hit, 0, max(int64(s.recentSeq-first)+1, 0))
	for seq := first; seq <= s.recentSeq; seq++ {
		out = append(out, s.recent[(seq-1)%maxRecent].Hit)
	}
	return out
}

// Red dit si une IP inconnue a obtenu un succès hors URL anonymes sur la période.
func (s *Store) Red(w Window, now time.Time) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	from, to := minuteRange(w, now)
	if int64(len(s.minutes)) < to-from {
		for mk, m := range s.minutes {
			if mk >= from && mk <= to && m.red > 0 {
				return true
			}
		}
		return false
	}
	for mk := from; mk <= to; mk++ {
		if m := s.minutes[mk]; m != nil && m.red > 0 {
			return true
		}
	}
	return false
}

// minuteRange renvoie les minutes (incluses) couvertes par la période.
func minuteRange(w Window, now time.Time) (from, to int64) {
	to = now.Unix() / 60
	switch w {
	case W7d:
		// Aligné sur les heures, comme la série.
		from = (now.Unix()/3600 - 167) * 60
	default:
		from = to - int64(w.Duration()/time.Minute) + 1
	}
	return from, to
}

// Stats décrit l'occupation mémoire, pour les journaux et les tests.
type Stats struct {
	Minutes, Cells, Extras int
}

func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Stats{Minutes: len(s.minutes), Cells: s.cells, Extras: len(s.extras)}
}
