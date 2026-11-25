package tail

import (
	"compress/gzip"
	"context"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"
)

func writeGz(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := gzip.NewWriter(f)
	zw.Write([]byte(content))
	zw.Close()
	f.Close()
}

func appendTo(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

func TestBackfillOrder(t *testing.T) {
	dir := t.TempDir()
	writeGz(t, filepath.Join(dir, "access.log.3.gz"), "a1\na2\n")
	writeGz(t, filepath.Join(dir, "access.log.2.gz"), "b1\n")
	// Tri numérique, pas alphabétique : 10 est plus ancien que 3.
	writeGz(t, filepath.Join(dir, "access.log.10.gz"), "z0\n")
	os.WriteFile(filepath.Join(dir, "access.log.1"), []byte("c1\nc2\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "access.log"), []byte("d1\nd2\n"), 0o644)
	// Fichiers voisins à ignorer.
	os.WriteFile(filepath.Join(dir, "error.log"), []byte("x\n"), 0o644)
	writeGz(t, filepath.Join(dir, "error.log.2.gz"), "x\n")

	var got []string
	offset, err := Backfill(dir, func(line []byte) { got = append(got, string(line)) })
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"z0", "a1", "a2", "b1", "c1", "c2", "d1", "d2"}
	if !slices.Equal(got, want) {
		t.Fatalf("ordre : %v", got)
	}
	if offset != 6 {
		t.Fatalf("offset = %d, attendu 6", offset)
	}
}

func TestBackfillPartialLastLine(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "access.log"), []byte("d1\nd2-incompl"), 0o644)
	var got []string
	offset, err := Backfill(dir, func(line []byte) { got = append(got, string(line)) })
	if err != nil || !slices.Equal(got, []string{"d1"}) || offset != 3 {
		t.Fatalf("got=%v offset=%d err=%v", got, offset, err)
	}
}

func TestBackfillMissingAndCorrupt(t *testing.T) {
	dir := t.TempDir()
	if offset, err := Backfill(dir, func([]byte) {}); err != nil || offset != 0 {
		t.Fatalf("dossier vide : offset=%d err=%v", offset, err)
	}
	// Une archive corrompue n'empêche pas de lire le reste.
	os.WriteFile(filepath.Join(dir, "access.log.2.gz"), []byte("pas du gzip"), 0o644)
	os.WriteFile(filepath.Join(dir, "access.log"), []byte("d1\n"), 0o644)
	var got []string
	_, err := Backfill(dir, func(line []byte) { got = append(got, string(line)) })
	if err == nil {
		t.Error("l'archive corrompue aurait dû être signalée")
	}
	if !slices.Equal(got, []string{"d1"}) {
		t.Fatalf("got=%v", got)
	}
}

// follower lance Follow et collecte les lignes.
type follower struct {
	mu    sync.Mutex
	lines []string
	stop  context.CancelFunc
	done  chan error
}

func startFollow(t *testing.T, path string, offset int64) *follower {
	t.Helper()
	old := PollInterval
	PollInterval = 10 * time.Millisecond
	ctx, cancel := context.WithCancel(context.Background())
	f := &follower{stop: cancel, done: make(chan error, 1)}
	go func() {
		f.done <- Follow(ctx, path, offset, func(line []byte) {
			f.mu.Lock()
			f.lines = append(f.lines, string(line))
			f.mu.Unlock()
		})
	}()
	t.Cleanup(func() {
		cancel()
		<-f.done
		PollInterval = old
	})
	return f
}

func (f *follower) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.lines)
}

func (f *follower) waitFor(t *testing.T, want []string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if slices.Equal(f.snapshot(), want) {
			// Laisser passer quelques sondages : rien ne doit s'ajouter.
			time.Sleep(50 * time.Millisecond)
			if got := f.snapshot(); !slices.Equal(got, want) {
				t.Fatalf("lignes en trop : %v", got)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("obtenu %v, attendu %v", f.snapshot(), want)
}

func TestFollowAppend(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	os.WriteFile(path, []byte("ancienne\n"), 0o644)
	f := startFollow(t, path, 9)
	appendTo(t, path, "l1\nl2\n")
	f.waitFor(t, []string{"l1", "l2"})
	appendTo(t, path, "l3\n")
	f.waitFor(t, []string{"l1", "l2", "l3"})
}

func TestFollowTruncation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	os.WriteFile(path, []byte("ancienne ligne 1\nancienne ligne 2\n"), 0o644)
	st, _ := os.Stat(path)
	f := startFollow(t, path, st.Size())
	appendTo(t, path, "avant\n")
	f.waitFor(t, []string{"avant"})

	// copytruncate : même inode, fichier vidé puis réécrit.
	if err := os.Truncate(path, 0); err != nil {
		t.Fatal(err)
	}
	appendTo(t, path, "après\n")
	f.waitFor(t, []string{"avant", "après"})
	appendTo(t, path, "suite\n")
	f.waitFor(t, []string{"avant", "après", "suite"})
}

func TestFollowTruncationDropsPartial(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	os.WriteFile(path, []byte("ancienne ligne assez longue\n"), 0o644)
	st, _ := os.Stat(path)
	f := startFollow(t, path, st.Size())
	appendTo(t, path, "moitié")
	time.Sleep(40 * time.Millisecond)
	os.Truncate(path, 0)
	appendTo(t, path, "neuve\n")
	// Le morceau d'avant la troncature ne doit pas être collé à la ligne suivante.
	f.waitFor(t, []string{"neuve"})
}

func TestFollowPartialLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	os.WriteFile(path, nil, 0o644)
	f := startFollow(t, path, 0)
	appendTo(t, path, `{"a":`)
	time.Sleep(40 * time.Millisecond)
	if got := f.snapshot(); len(got) != 0 {
		t.Fatalf("ligne incomplète livrée : %v", got)
	}
	appendTo(t, path, "1}\n")
	f.waitFor(t, []string{`{"a":1}`})
}

func TestFollowFileAppearsLater(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	f := startFollow(t, path, 0)
	time.Sleep(30 * time.Millisecond)
	appendTo(t, path, "l1\n")
	f.waitFor(t, []string{"l1"})
}

func TestFollowStopsOnCancel(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	os.WriteFile(path, nil, 0o644)
	f := startFollow(t, path, 0)
	f.stop()
	select {
	case <-f.done:
		f.done <- nil // pour le Cleanup
	case <-time.After(time.Second):
		t.Fatal("Follow ne s'arrête pas")
	}
}
