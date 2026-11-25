// Package logline lit une ligne du journal d'accès JSON de Traefik.
package logline

import (
	"encoding/json"
	"net/netip"
	"strings"
	"time"
)

// Class est la famille de statut d'une réponse.
type Class string

const (
	Class1xx     Class = "1xx"
	Class2xx     Class = "2xx"
	Class3xx     Class = "3xx"
	Class4xx     Class = "4xx"
	Class5xx     Class = "5xx"
	ClassAbandon Class = "abandon" // statut 0 : le client est parti avant la réponse
)

// Classes liste les familles dans l'ordre d'affichage.
var Classes = []Class{Class1xx, Class2xx, Class3xx, Class4xx, Class5xx, ClassAbandon}

// Entry est une requête servie (ou refusée) par Traefik.
type Entry struct {
	Time     time.Time     // champ "time" (UTC)
	IP       netip.Addr    // "ClientHost" : vraie IP du client
	Status   int           // "DownstreamStatus"
	Router   string        // "RouterName" sans le suffixe "@file" ; "" si aucun router
	Host     string        // "RequestHost"
	Path     string        // "RequestPath" (avec query string)
	Method   string        // "RequestMethod"
	UA       string        // "request_User-Agent"
	BytesIn  int64         // "RequestContentSize"
	BytesOut int64         // "DownstreamContentSize"
	Duration time.Duration // "Duration" (ns)
}

type raw struct {
	Time     time.Time `json:"time"`
	IP       string    `json:"ClientHost"`
	Status   int       `json:"DownstreamStatus"`
	Router   string    `json:"RouterName"`
	Host     string    `json:"RequestHost"`
	Path     string    `json:"RequestPath"`
	Method   string    `json:"RequestMethod"`
	UA       string    `json:"request_User-Agent"`
	BytesIn  int64     `json:"RequestContentSize"`
	BytesOut int64     `json:"DownstreamContentSize"`
	Duration int64     `json:"Duration"`
}

// ParseLine décode une ligne. Renvoie false si elle n'est pas du JSON valide, n'a
// pas d'horodatage ou pas d'IP cliente exploitable.
func ParseLine(b []byte) (Entry, bool) {
	if len(b) == 0 || b[0] != '{' {
		return Entry{}, false
	}
	var r raw
	if err := json.Unmarshal(b, &r); err != nil {
		return Entry{}, false
	}
	if r.Time.IsZero() {
		return Entry{}, false
	}
	ip, ok := clientIP(r.IP)
	if !ok {
		return Entry{}, false
	}
	router, _, _ := strings.Cut(r.Router, "@")
	return Entry{
		Time:     r.Time.UTC(),
		IP:       ip.Unmap().WithZone(""),
		Status:   r.Status,
		Router:   router,
		Host:     r.Host,
		Path:     r.Path,
		Method:   r.Method,
		UA:       r.UA,
		BytesIn:  r.BytesIn,
		BytesOut: r.BytesOut,
		Duration: time.Duration(r.Duration),
	}, true
}

// clientIP lit "ClientHost". Quand un client envoie son propre X-Forwarded-For,
// Traefik a pu journaliser la liste entière (« localhost,203.0.113.104 ») : la
// dernière adresse est celle qu'a ajoutée le proxy de confiance.
func clientIP(s string) (netip.Addr, bool) {
	if i := strings.LastIndexByte(s, ','); i >= 0 {
		s = strings.TrimSpace(s[i+1:])
	}
	ip, err := netip.ParseAddr(s)
	if err != nil {
		return netip.Addr{}, false
	}
	return ip, true
}

// ClassOf range un statut HTTP dans sa famille. Tout statut hors 100–599 est un abandon.
func ClassOf(status int) Class {
	switch {
	case status >= 100 && status < 200:
		return Class1xx
	case status >= 200 && status < 300:
		return Class2xx
	case status >= 300 && status < 400:
		return Class3xx
	case status >= 400 && status < 500:
		return Class4xx
	case status >= 500 && status < 600:
		return Class5xx
	}
	return ClassAbandon
}

// LongLived : durée à partir de laquelle une connexion close sans statut est tenue
// pour un WebSocket accepté.
const LongLived = 30 * time.Second

// Success dit si la requête a obtenu un succès : 2xx, ou WebSocket accepté.
//
// Traefik ne journalise pas 101 pour un WebSocket : la connexion est détournée et
// la ligne, écrite à la fermeture, porte le statut 0 comme un abandon. Ce qui les
// distingue est la durée : un client qui part avant la réponse ne reste pas 30 s,
// alors qu'un WebSocket accepté dure (Home Assistant ferme au bout de 10 s celui
// qui ne s'authentifie pas). Une connexion sans statut d'au moins LongLived compte
// donc comme un succès ; 101 aussi, au cas où Traefik le journaliserait un jour.
func (e Entry) Success() bool {
	return e.Status == 101 || (e.Status >= 200 && e.Status < 300) || e.LongLivedSocket()
}

// LongLivedSocket : statut 0 et durée d'au moins LongLived.
func (e Entry) LongLivedSocket() bool {
	return e.Status == 0 && e.Duration >= LongLived
}
