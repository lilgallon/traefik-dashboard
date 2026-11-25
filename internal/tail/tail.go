// Package tail relit les archives de access.log puis suit le fichier en direct,
// y compris à travers une rotation en copytruncate.
package tail

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	baseName = "access.log"
	// Une ligne plus longue que ça n'est pas une ligne de log : on la jette plutôt
	// que de la garder en mémoire en attendant un saut de ligne.
	maxLine = 1 << 20
)

// PollInterval est la période de sondage de Follow.
var PollInterval = 500 * time.Millisecond

// Backfill lit access.log.N.gz (du plus ancien au plus récent), access.log.1 puis
// access.log, et appelle fn pour chaque ligne complète. Renvoie la position
// atteinte dans access.log (fin de la dernière ligne complète), d'où Follow
// reprend. Une archive illisible est signalée dans l'erreur mais n'arrête pas la
// relecture.
func Backfill(dir string, fn func(line []byte)) (offset int64, err error) {
	var errs []error

	matches, _ := filepath.Glob(filepath.Join(dir, baseName+".*.gz"))
	type archive struct {
		n    int
		path string
	}
	var archives []archive
	for _, m := range matches {
		num := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(m), baseName+"."), ".gz")
		if n, err := strconv.Atoi(num); err == nil {
			archives = append(archives, archive{n, m})
		}
	}
	sort.Slice(archives, func(i, j int) bool { return archives[i].n > archives[j].n })

	for _, a := range archives {
		if err := readGz(a.path, fn); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(a.path), err))
		}
	}
	if _, err := readPlain(filepath.Join(dir, baseName+".1"), fn); err != nil {
		errs = append(errs, fmt.Errorf("%s.1: %w", baseName, err))
	}
	offset, err = readPlain(filepath.Join(dir, baseName), fn)
	if err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", baseName, err))
	}
	return offset, errors.Join(errs...)
}

func readGz(path string, fn func([]byte)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer zr.Close()
	_, err = readLines(zr, fn)
	return err
}

// readPlain renvoie le nombre d'octets consommés (lignes complètes seulement).
// Un fichier absent n'est pas une erreur.
func readPlain(path string, fn func([]byte)) (int64, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return readLines(f, fn)
}

// readLines appelle fn pour chaque ligne terminée par '\n' et renvoie le nombre
// d'octets de ces lignes. Le tampon passé à fn n'est valable que pendant l'appel.
func readLines(r io.Reader, fn func([]byte)) (int64, error) {
	br := bufio.NewReaderSize(r, 256<<10)
	var consumed int64
	var long []byte // ligne plus longue que le tampon du lecteur
	for {
		chunk, err := br.ReadSlice('\n')
		switch {
		case err == nil:
			line := chunk
			if long != nil {
				long = append(long, chunk...)
				line = long
			}
			consumed += int64(len(line))
			if len(line) <= maxLine {
				fn(bytes.TrimRight(line, "\r\n"))
			}
			long = nil
		case errors.Is(err, bufio.ErrBufferFull):
			if len(long) <= maxLine {
				long = append(long, chunk...)
			} else {
				consumed += int64(len(chunk)) // ligne démesurée : sautée au fil de l'eau
			}
		case errors.Is(err, io.EOF):
			return consumed, nil // une fin sans '\n' reste à lire par Follow
		default:
			return consumed, err
		}
	}
}

// Follow suit le fichier à partir de offset et appelle fn pour chaque nouvelle
// ligne complète. Si le fichier devient plus court que la position atteinte, il a
// été tronqué (rotation copytruncate) : la lecture reprend au début, et le morceau
// de ligne en attente est abandonné. Un fichier absent est attendu. Ne s'arrête
// qu'avec ctx.
//
// Limite connue : si, entre deux sondages, le fichier est tronqué PUIS dépasse à
// nouveau l'ancienne position, la troncature passe inaperçue. Il faudrait plus de
// 100 Mo écrits en une demi-seconde.
func Follow(ctx context.Context, path string, offset int64, fn func(line []byte)) error {
	var partial []byte
	ticker := time.NewTicker(PollInterval)
	defer ticker.Stop()
	for {
		offset, partial = poll(path, offset, partial, fn)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func poll(path string, offset int64, partial []byte, fn func([]byte)) (int64, []byte) {
	f, err := os.Open(path)
	if err != nil {
		return offset, partial
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return offset, partial
	}
	// offset compte aussi les octets du morceau en attente.
	if st.Size() < offset {
		offset, partial = 0, nil
	}
	if st.Size() == offset {
		return offset, partial
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return offset, partial
	}
	buf := make([]byte, 64<<10)
	for {
		n, err := f.Read(buf)
		if n > 0 {
			offset += int64(n)
			data := buf[:n]
			for {
				i := bytes.IndexByte(data, '\n')
				if i < 0 {
					break
				}
				line := data[:i]
				if partial != nil {
					partial = append(partial, line...)
					line = partial
				}
				if len(line) <= maxLine {
					fn(bytes.TrimRight(line, "\r"))
				}
				partial = nil
				data = data[i+1:]
			}
			if len(data) > 0 && len(partial) <= maxLine {
				partial = append(partial, data...)
			}
		}
		if err != nil {
			return offset, partial
		}
	}
}
