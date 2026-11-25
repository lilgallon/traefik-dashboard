package names

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeAt(t *testing.T, path, content string, mtime time.Time) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
}

func TestReloaderKeepsPreviousOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "names.yaml")
	t0 := time.Now().Add(-time.Hour)
	writeAt(t, path, sample, t0)

	rl, err := NewReloader(path)
	if err != nil {
		t.Fatal(err)
	}
	before := rl.Rules()
	if len(before.Names) == 0 {
		t.Fatal("règles vides")
	}

	if changed, err := rl.Poll(); changed || err != nil {
		t.Fatalf("sans modification : changed=%v err=%v", changed, err)
	}

	writeAt(t, path, "names: [", t0.Add(time.Minute))
	changed, err := rl.Poll()
	if err == nil || changed {
		t.Fatalf("YAML invalide : changed=%v err=%v", changed, err)
	}
	if rl.Rules() != before {
		t.Fatal("les règles précédentes ont été remplacées")
	}
	// L'erreur reste signalée tant que le fichier n'est pas réparé.
	if _, err := rl.Poll(); err == nil || rl.Err() == nil {
		t.Fatal("l'erreur a disparu sans réparation")
	}

	writeAt(t, path, "names:\n  - name: Seule\n    ip: [1.2.3.4]\n", t0.Add(2*time.Minute))
	changed, err = rl.Poll()
	if !changed || err != nil || rl.Err() != nil {
		t.Fatalf("réparation : changed=%v err=%v", changed, err)
	}
	if got := rl.Rules().Names[0].Name; got != "Seule" {
		t.Fatalf("règles après réparation : %q", got)
	}
}

func TestReloaderFileRemoved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "names.yaml")
	writeAt(t, path, sample, time.Now().Add(-time.Hour))
	rl, err := NewReloader(path)
	if err != nil {
		t.Fatal(err)
	}
	before := rl.Rules()
	os.Remove(path)
	if changed, err := rl.Poll(); changed || err == nil {
		t.Fatalf("fichier supprimé : changed=%v err=%v", changed, err)
	}
	if rl.Rules() != before {
		t.Fatal("règles perdues")
	}
}

func TestNewReloaderBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "names.yaml")
	writeAt(t, path, "names: [", time.Now())
	rl, err := NewReloader(path)
	if err == nil {
		t.Fatal("erreur attendue")
	}
	// Utilisable quand même : règles vides, jamais chargées.
	if rl == nil || rl.Rules() == nil || rl.Loaded() {
		t.Fatal("Reloader inutilisable après un premier chargement raté")
	}
}
