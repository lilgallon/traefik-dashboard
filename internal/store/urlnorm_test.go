package store

import (
	"strings"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"/ocpi/example/cpo/2.2.1/tokens/0A1B2C3D4E5F60/authorize":                       "/ocpi/example/cpo/2.2.1/tokens/{id}/authorize",
		"/api/webhook/25a3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c3": "/api/webhook/{id}",
		"/a?b=1":             "/a",
		"/":                  "/",
		"":                   "/",
		"/?x=0A1B2C3D4E5F60": "/",
		"/api/states/sensor.thermometre_salon_temperature":    "/api/states/sensor.thermometre_salon_temperature",
		"/sessions/3f2b1c4d-5e6f-7a8b-9c0d-1e2f3a4b5c6d/stop": "/sessions/{id}/stop",
		"/frontend_latest/core-BhGk2s9d.js":                   "/frontend_latest/{id}",
		"/users/1234567/profile":                              "/users/{id}/profile",
		"/v2/items/42":                                        "/v2/items/42",
		"/ocpi/2.2.1/locations/FR/ABC/LOC000123":              "/ocpi/2.2.1/locations/FR/ABC/{id}",
		"/static/icons/favicon-192x192.png":                   "/static/icons/{id}",
	}
	for in, want := range cases {
		if got := NormalizePath(in); got != want {
			t.Errorf("NormalizePath(%q) = %q, attendu %q", in, got, want)
		}
	}
}

func TestNormalizePathBounded(t *testing.T) {
	long := "/" + strings.Repeat("abcdefgh/", 100)
	if got := NormalizePath(long); len(got) > maxPathLen+8 {
		t.Errorf("chemin long : %d caractères", len(got))
	}
	deep := strings.Repeat("/a1", 50)
	if got := NormalizePath(deep); strings.Count(got, "/") > maxSegments {
		t.Errorf("chemin profond : %q", got)
	}
}

func TestIsSensitive(t *testing.T) {
	for path, want := range map[string]bool{
		"/.env":                  true,
		"/app/.git/config":       true,
		"/wp-login.php":          true,
		"/dump.sql":              true,
		"/backup/site.zip":       true,
		"/":                      false,
		"/api/states":            false,
		"/x?file=/.env":          false,
		"/static/environment.js": false,
	} {
		if got := IsSensitive(path); got != want {
			t.Errorf("IsSensitive(%q) = %v", path, got)
		}
	}
}

func TestMaskSecrets(t *testing.T) {
	for in, want := range map[string]string{
		"/stream?token=C4o.A0D6":             "/stream?token=…",
		"/x?a=1&api_key=abc&b=2":             "/x?a=1&api_key=…&b=2",
		"/message?limit=10":                  "/message?limit=10",
		"/auth/authorize?client_id=x&code=y": "/auth/authorize?client_id=x&code=…",
		"/token/abc":                         "/token/abc",
	} {
		if got := maskSecrets(in); got != want {
			t.Errorf("maskSecrets(%q) = %q, attendu %q", in, got, want)
		}
	}
}
