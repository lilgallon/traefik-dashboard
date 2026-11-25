package geo

import (
	"net/netip"
	"os"
	"testing"
)

// Bases copiées depuis l'image crowdsec (dossier ignoré par Git) :
//
//	docker cp -L crowdsec:/staging/var/lib/crowdsec/data/GeoLite2-City.mmdb testdata/geo/
//	docker cp -L crowdsec:/staging/var/lib/crowdsec/data/GeoLite2-ASN.mmdb testdata/geo/
const (
	cityPath = "../../testdata/geo/GeoLite2-City.mmdb"
	asnPath  = "../../testdata/geo/GeoLite2-ASN.mmdb"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	if _, err := os.Stat(cityPath); err != nil {
		t.Skip("bases GeoLite2 absentes de testdata/geo")
	}
	db, err := Open(cityPath, asnPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestLookupKnown(t *testing.T) {
	db := openTestDB(t)
	got := db.Lookup(netip.MustParseAddr("8.8.8.8"))
	if got.ASN != 15169 || got.Country != "US" || got.Org == "" {
		t.Fatalf("8.8.8.8 → %+v", got)
	}
	// Deuxième appel : servi par le cache, même résultat.
	if again := db.Lookup(netip.MustParseAddr("8.8.8.8")); again != got {
		t.Fatalf("cache : %+v != %+v", again, got)
	}
}

func TestLookupUnknown(t *testing.T) {
	db := openTestDB(t)
	for _, ip := range []string{"192.168.1.10", "10.0.0.1", "fe80::1"} {
		if got := db.Lookup(netip.MustParseAddr(ip)); got != (Info{}) {
			t.Errorf("%s → %+v, attendu vide", ip, got)
		}
	}
	if got := db.Lookup(netip.Addr{}); got != (Info{}) {
		t.Errorf("adresse invalide → %+v", got)
	}
}

func TestCacheBounded(t *testing.T) {
	db := openTestDB(t)
	base := netip.MustParseAddr("8.0.0.0")
	ip := base
	for i := 0; i < cacheSize+500; i++ {
		db.Lookup(ip)
		ip = ip.Next()
	}
	if n := db.cacheLen(); n > cacheSize {
		t.Fatalf("cache à %d entrées, plafond %d", n, cacheSize)
	}
}

func TestNone(t *testing.T) {
	var l Lookuper = None{}
	if got := l.Lookup(netip.MustParseAddr("8.8.8.8")); got != (Info{}) {
		t.Fatalf("None → %+v", got)
	}
}

func TestOpenMissing(t *testing.T) {
	if _, err := Open("/nexiste/pas.mmdb", "/nexiste/pas.mmdb"); err == nil {
		t.Fatal("Open aurait dû échouer")
	}
}
