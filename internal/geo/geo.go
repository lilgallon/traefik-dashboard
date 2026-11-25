// Package geo donne le pays et l'opérateur (ASN) d'une IP à partir des bases
// GeoLite2 que CrowdSec télécharge déjà.
package geo

import (
	"fmt"
	"net/netip"
	"sync"

	"github.com/oschwald/maxminddb-golang/v2"
)

// Info décrit une IP. La valeur zéro signifie « inconnue » (IP privée ou absente
// des bases).
type Info struct {
	Country string // code ISO, ex. "FR"
	ASN     uint
	Org     string
}

// Lookuper est ce dont le reste du service a besoin.
type Lookuper interface {
	Lookup(ip netip.Addr) Info
}

// None est utilisé quand les bases manquent : toutes les IP sont inconnues.
type None struct{}

func (None) Lookup(netip.Addr) Info { return Info{} }

const cacheSize = 10_000

// DB interroge les deux bases, avec un cache borné.
type DB struct {
	city, asn *maxminddb.Reader

	mu    sync.Mutex
	cache map[netip.Addr]Info
}

// Open ouvre les bases (mmap : elles ne sont pas chargées en mémoire).
func Open(cityPath, asnPath string) (*DB, error) {
	city, err := maxminddb.Open(cityPath)
	if err != nil {
		return nil, fmt.Errorf("city database: %w", err)
	}
	asn, err := maxminddb.Open(asnPath)
	if err != nil {
		city.Close()
		return nil, fmt.Errorf("ASN database: %w", err)
	}
	return &DB{city: city, asn: asn, cache: make(map[netip.Addr]Info)}, nil
}

func (db *DB) Close() {
	db.city.Close()
	db.asn.Close()
}

type cityRecord struct {
	Country struct {
		ISO string `maxminddb:"iso_code"`
	} `maxminddb:"country"`
}

type asnRecord struct {
	Number uint   `maxminddb:"autonomous_system_number"`
	Org    string `maxminddb:"autonomous_system_organization"`
}

// Lookup ne renvoie jamais d'erreur : une IP introuvable donne Info{}.
func (db *DB) Lookup(ip netip.Addr) Info {
	if !ip.IsValid() {
		return Info{}
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if info, ok := db.cache[ip]; ok {
		return info
	}
	var info Info
	var c cityRecord
	if err := db.city.Lookup(ip).Decode(&c); err == nil {
		info.Country = c.Country.ISO
	}
	var a asnRecord
	if err := db.asn.Lookup(ip).Decode(&a); err == nil {
		info.ASN, info.Org = a.Number, a.Org
	}
	if len(db.cache) >= cacheSize {
		// Vider d'un coup : plus simple qu'un LRU, et le cache se re-remplit avec
		// les IP actives en quelques secondes.
		clear(db.cache)
	}
	db.cache[ip] = info
	return info
}

func (db *DB) cacheLen() int {
	db.mu.Lock()
	defer db.mu.Unlock()
	return len(db.cache)
}
