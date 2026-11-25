// loggen écrit un access.log Traefik synthétique (format JSON) pour
// l'environnement local : 7 jours d'historique, puis du trafic en direct.
//
// Les IP viennent des bases de test de MaxMind (dépôt maxmind/MaxMind-DB),
// que l'environnement local monte à la place des vraies bases GeoLite2 : pays et
// opérateurs s'affichent, sans aucune IP réelle de client.
//
//	go run ./dev/loggen -out /logs/access.log
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"
)

type actor struct {
	ip     string
	ua     string
	weight float64 // part du trafic
	gen    func(r *rand.Rand, a *actor) req
}

type req struct {
	host, router, method, path string
	status                     int
	dur                        time.Duration
	bytesOut                   int64
}

func pick[T any](r *rand.Rand, xs ...T) T { return xs[r.IntN(len(xs))] }

const (
	uaFirefox = "Mozilla/5.0 (X11; Linux x86_64; rv:145.0) Gecko/20100101 Firefox/145.0"
	uaSafari  = "Mozilla/5.0 (iPhone; CPU iPhone OS 26_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/26.1 Mobile/15E148 Safari/604.1"
	uaHA      = "Home Assistant/2026.11.2 (io.robbie.HomeAssistant; build:2026.1342; iOS 26.1.0)"
	uaPartner = "ocpi-client/2.2.1"
	uaKuma    = "Uptime-Kuma/2.1.0"
	uaScan    = "Mozilla/5.0 (compatible; Odin; https://docs.getodin.com/)"
	uaCurl    = "curl/8.17.0"
	uaGo      = "Go-http-client/1.1"
)

func ms(r *rand.Rand, lo, hi int) time.Duration {
	return time.Duration(lo+r.IntN(hi-lo+1)) * time.Millisecond
}

func homeUser(r *rand.Rand, _ *actor) req {
	switch r.IntN(10) {
	case 0:
		// WebSocket de l'application : Traefik le journalise à sa fermeture, statut 0.
		return req{"home.example.com", "homeassistant", "GET", "/api/websocket", 0, time.Duration(60+r.IntN(1800)) * time.Second, 0}
	case 1, 2:
		return req{"home.example.com", "homeassistant", "GET", pick(r, "/api/states", "/api/config", "/api/history/period"), 200, ms(r, 4, 40), int64(2000 + r.IntN(40000))}
	case 3:
		return req{"n8n.example.com", "n8n", "GET", pick(r, "/rest/workflows", "/rest/executions", "/"), 200, ms(r, 10, 80), int64(1000 + r.IntN(20000))}
	case 4:
		return req{"notify.example.com", "gotify", "GET", "/message?limit=20", 200, ms(r, 2, 10), 1800}
	default:
		return req{"home.example.com", "homeassistant", "GET", pick(r, "/", "/frontend_latest/app.js", "/static/icons/favicon.ico", "/lovelace/0"), pick(r, 200, 200, 304), ms(r, 2, 25), int64(500 + r.IntN(90000))}
	}
}

func partner(r *rand.Rand, _ *actor) req {
	path := pick(r, "/ocpi/2.2.1/locations", "/ocpi/2.2.1/sessions", "/ocpi/2.2.1/cdrs", "/ocpi/2.2.1/tariffs", "/ocpi/versions")
	method := pick(r, "GET", "GET", "PUT", "PATCH")
	status := 200
	if r.IntN(200) == 0 {
		status = 502
	}
	return req{"app.example.com", "app", method, path + fmt.Sprintf("/FR*EXA*E%05d", r.IntN(4000)), status, ms(r, 8, 120), int64(300 + r.IntN(6000))}
}

func phone(r *rand.Rand, _ *actor) req {
	return req{"home.example.com", "homeassistant", pick(r, "GET", "POST"), pick(r, "/api/states", "/api/webhook/5f2c9a1e8b7d4c3a2f1e0d9c8b7a6f5e", "/api/websocket"), 200, ms(r, 5, 60), int64(400 + r.IntN(8000))}
}

func kuma(r *rand.Rand, _ *actor) req {
	return req{"status.example.com", "uptime-kuma", "GET", pick(r, "/", "/api/status-page/heartbeat/default", "/api/badge/1/status"), 200, ms(r, 3, 20), 4200}
}

// Visiteurs anonymes : pages publiques par conception (connexion, statut, webhooks).
func anonymous(r *rand.Rand, _ *actor) req {
	switch r.IntN(3) {
	case 0:
		return req{"home.example.com", "homeassistant", "GET", pick(r, "/", "/auth/authorize?response_type=code", "/manifest.json"), 200, ms(r, 3, 20), 3000}
	case 1:
		return req{"status.example.com", "uptime-kuma", "GET", "/", 200, ms(r, 3, 20), 4200}
	default:
		return req{"n8n.example.com", "n8n-webhooks", "POST", "/webhook/github-push", 200, ms(r, 20, 300), 120}
	}
}

var scanPaths = []string{
	"/.env", "/.env.local", "/.git/config", "/wp-login.php", "/wp-admin/setup-config.php",
	"/xmlrpc.php", "/phpmyadmin/", "/actuator/health", "/server-status", "/config.json",
	"/backup.sql", "/.aws/credentials", "/admin/", "/cgi-bin/luci", "/vendor/phpunit/phpunit/src/Util/PHP/eval-stdin.php",
}

func scanner(r *rand.Rand, _ *actor) req {
	host := pick(r, "home.example.com", "app.example.com", "n8n.example.com", "status.example.com")
	router := map[string]string{"home.example.com": "homeassistant", "app.example.com": "app", "n8n.example.com": "n8n", "status.example.com": "uptime-kuma"}[host]
	status := pick(r, 404, 404, 404, 403, 401)
	if host == "app.example.com" {
		status = 403 // allowlist du partenaire
	}
	return req{host, router, pick(r, "GET", "GET", "POST"), pick(r, scanPaths...), status, ms(r, 1, 8), 19}
}

// Tâtonnement sur la connexion Home Assistant.
func bruteforce(r *rand.Rand, _ *actor) req {
	return req{"home.example.com", "homeassistant", "POST", "/auth/login_flow/" + fmt.Sprintf("%032x", r.Uint64()), pick(r, 400, 401, 401), ms(r, 30, 200), 64}
}

func actors() []*actor {
	return []*actor{
		{ip: "81.2.69.142", ua: uaFirefox, weight: 14, gen: homeUser},    // Home (GB)
		{ip: "1.128.0.5", ua: uaHA, weight: 6, gen: phone},               // téléphone, nommé par ASN (Telstra)
		{ip: "89.160.20.115", ua: uaPartner, weight: 40, gen: partner},   // partenaire (SE)
		{ip: "89.160.20.120", ua: uaPartner, weight: 25, gen: partner},   // partenaire (SE)
		{ip: "216.160.83.58", ua: uaKuma, weight: 6, gen: kuma},          // sonde externe (US)
		{ip: "2.125.160.218", ua: uaSafari, weight: 2, gen: anonymous},   // visiteur (GB)
		{ip: "2001:220:1::9", ua: uaSafari, weight: 1.5, gen: anonymous}, // visiteur (KR)
		{ip: "2001:218:1::7", ua: uaFirefox, weight: 1, gen: anonymous},  // visiteur (JP)
		{ip: "175.16.199.23", ua: uaScan, weight: 1.6, gen: scanner},     // scanner (CN)
		{ip: "202.196.224.9", ua: uaGo, weight: 1.2, gen: scanner},       // scanner (PH)
		{ip: "67.43.156.14", ua: uaCurl, weight: 0.8, gen: scanner},      // scanner (BT)
		{ip: "2001:238::77", ua: uaGo, weight: 0.6, gen: bruteforce},     // tâtonne (TW)
		{ip: "2001:250:2::42", ua: uaScan, weight: 0.6, gen: scanner},    // scanner (CN)
	}
}

func choose(r *rand.Rand, as []*actor) *actor {
	var total float64
	for _, a := range as {
		total += a.weight
	}
	x := r.Float64() * total
	for _, a := range as {
		if x -= a.weight; x < 0 {
			return a
		}
	}
	return as[len(as)-1]
}

// Rythme de la journée : creux la nuit, pic en soirée (heure UTC).
func rate(t time.Time) float64 {
	h := float64(t.Hour()) + float64(t.Minute())/60
	return 0.55 + 0.45*math.Sin((h-13)/24*2*math.Pi)
}

type line struct {
	ClientAddr            string `json:"ClientAddr"`
	ClientHost            string `json:"ClientHost"`
	DownstreamContentSize int64  `json:"DownstreamContentSize"`
	DownstreamStatus      int    `json:"DownstreamStatus"`
	Duration              int64  `json:"Duration"`
	OriginStatus          int    `json:"OriginStatus"`
	RequestAddr           string `json:"RequestAddr"`
	RequestContentSize    int64  `json:"RequestContentSize"`
	RequestHost           string `json:"RequestHost"`
	RequestMethod         string `json:"RequestMethod"`
	RequestPath           string `json:"RequestPath"`
	RequestProtocol       string `json:"RequestProtocol"`
	RequestScheme         string `json:"RequestScheme"`
	RouterName            string `json:"RouterName"`
	ServiceName           string `json:"ServiceName"`
	StartUTC              string `json:"StartUTC"`
	EntryPointName        string `json:"entryPointName"`
	Level                 string `json:"level"`
	Msg                   string `json:"msg"`
	UserAgent             string `json:"request_User-Agent"`
	Time                  string `json:"time"`
}

func render(t time.Time, a *actor, q req) []byte {
	l := line{
		ClientAddr:            "172.30.0.2:" + fmt.Sprint(30000+int(t.UnixNano()%30000)),
		ClientHost:            a.ip,
		DownstreamContentSize: q.bytesOut,
		DownstreamStatus:      q.status,
		Duration:              int64(q.dur),
		OriginStatus:          q.status,
		RequestAddr:           q.host,
		RequestHost:           q.host,
		RequestMethod:         q.method,
		RequestPath:           q.path,
		RequestProtocol:       "HTTP/1.1",
		RequestScheme:         "https",
		RouterName:            q.router + "@file",
		ServiceName:           q.router + "@file",
		StartUTC:              t.Add(-q.dur).UTC().Format(time.RFC3339Nano),
		EntryPointName:        "websecure",
		Level:                 "info",
		UserAgent:             a.ua,
		Time:                  t.UTC().Format(time.RFC3339),
	}
	if q.method != "GET" {
		l.RequestContentSize = int64(len(q.path)*8) + t.Unix()%977
	}
	b, _ := json.Marshal(l)
	return append(b, '\n')
}

// Succès d'une IP inconnue, hors des URL anonymes : passe le tableau de bord au
// rouge (alerte Telegram en direct).
var intruder = &actor{ip: "216.160.83.66", ua: uaGo, gen: func(r *rand.Rand, _ *actor) req {
	return req{"home.example.com", "homeassistant", "GET", "/api/states", 200, ms(r, 4, 30), 18000}
}}

func main() {
	out := flag.String("out", "/logs/access.log", "fichier à écrire")
	days := flag.Int("days", 7, "jours d'historique écrits au premier démarrage")
	perDay := flag.Int("per-day", 25000, "requêtes par jour (moyenne)")
	intruderAgo := flag.Duration("intruder-ago", 3*time.Hour, "âge du succès d'une IP inconnue dans l'historique (0 : aucun)")
	intruderEvery := flag.Duration("intruder-every", 0, "en direct, un succès d'une IP inconnue à cet intervalle (0 : jamais)")
	flag.Parse()

	r := rand.New(rand.NewPCG(42, 7))
	as := actors()
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}

	if _, err := os.Stat(*out); os.IsNotExist(err) {
		backfill(r, as, *out, *days, *perDay, *intruderAgo)
	} else {
		log.Printf("%s existe : pas d'historique, suite en direct", *out)
	}

	f, err := os.OpenFile(*out, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	perSec := float64(*perDay) / 86400
	lastIntruder := time.Now()
	// Rafale de scan toutes les 90 s, étalée sur une quinzaine de secondes, d'une
	// IP de scanner à l'autre : CrowdSec la reconnaît et bannit l'IP.
	scanners := []*actor{as[8], as[9], as[10], as[12]}
	nextBurst, burstLeft, burstIP := time.Now().Add(20*time.Second), 0, 0
	log.Printf("trafic en direct : ~%.1f requêtes/s", perSec)
	for {
		now := time.Now()
		n := poisson(r, perSec*rate(now))
		for range n {
			a := choose(r, as)
			f.Write(render(now, a, a.gen(r, a)))
		}
		if burstLeft == 0 && now.After(nextBurst) {
			burstLeft, burstIP = 25, (burstIP+1)%len(scanners)
			nextBurst = now.Add(90 * time.Second)
		}
		for i := 0; i < 2 && burstLeft > 0; i++ {
			a := scanners[burstIP]
			f.Write(render(now, a, scanner(r, a)))
			burstLeft--
		}
		if *intruderEvery > 0 && now.Sub(lastIntruder) >= *intruderEvery {
			f.Write(render(now, intruder, intruder.gen(r, intruder)))
			lastIntruder = now
		}
		time.Sleep(time.Second)
	}
}

func backfill(r *rand.Rand, as []*actor, out string, days, perDay int, intruderAgo time.Duration) {
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	now := time.Now().Truncate(time.Second)
	start := now.Add(-time.Duration(days) * 24 * time.Hour)
	perSec := float64(perDay) / 86400
	var n int
	intruderAt := now.Add(-intruderAgo)
	for t := start; t.Before(now); t = t.Add(time.Second) {
		for range poisson(r, perSec*rate(t)) {
			a := choose(r, as)
			w.Write(render(t, a, a.gen(r, a)))
			n++
		}
		if intruderAgo > 0 && t.Equal(intruderAt.Truncate(time.Second)) {
			for i := range 3 {
				w.Write(render(t.Add(time.Duration(i)*time.Second), intruder, intruder.gen(r, intruder)))
			}
		}
	}
	if err := w.Flush(); err != nil {
		log.Fatal(err)
	}
	f.Close()
	log.Printf("historique écrit : %d lignes sur %d jours", n, days)
}

// poisson tire un nombre d'événements de moyenne lambda (algorithme de Knuth,
// suffisant pour de petites moyennes).
func poisson(r *rand.Rand, lambda float64) int {
	l, k, p := math.Exp(-lambda), 0, 1.0
	for {
		p *= r.Float64()
		if p <= l {
			return k
		}
		k++
	}
}
