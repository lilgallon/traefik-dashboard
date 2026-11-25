package names

import (
	"os"
	"sync"
	"time"
)

// Reloader garde les règles courantes et relit le fichier quand il change.
// Un fichier cassé ne remplace jamais des règles valides.
type Reloader struct {
	path string

	mu     sync.RWMutex
	rules  *Rules
	loaded bool // au moins un chargement réussi
	err    error
	mtime  time.Time
	size   int64
}

// NewReloader charge le fichier. En cas d'erreur, le Reloader renvoyé reste
// utilisable (règles vides, Loaded() faux) et se rattrapera au prochain Poll.
func NewReloader(path string) (*Reloader, error) {
	rl := &Reloader{path: path, rules: &Rules{}}
	_, err := rl.Poll()
	return rl, err
}

func (rl *Reloader) Rules() *Rules {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.rules
}

// Loaded dit si des règles valides ont été chargées au moins une fois.
func (rl *Reloader) Loaded() bool {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.loaded
}

// Err renvoie l'erreur du dernier chargement, nil si le fichier courant est valide.
func (rl *Reloader) Err() error {
	rl.mu.RLock()
	defer rl.mu.RUnlock()
	return rl.err
}

// Poll relit le fichier si sa date ou sa taille a changé. En erreur, les règles
// précédentes sont conservées et l'erreur est renvoyée à chaque appel jusqu'à
// réparation.
func (rl *Reloader) Poll() (changed bool, err error) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	st, err := os.Stat(rl.path)
	if err != nil {
		rl.err = err
		rl.mtime, rl.size = time.Time{}, -1
		return false, err
	}
	if st.ModTime().Equal(rl.mtime) && st.Size() == rl.size {
		return false, rl.err
	}
	rl.mtime, rl.size = st.ModTime(), st.Size()

	b, err := os.ReadFile(rl.path)
	if err != nil {
		rl.err = err
		return false, err
	}
	rules, err := Parse(b)
	if err != nil {
		rl.err = err
		return false, err
	}
	rl.rules, rl.err, rl.loaded = rules, nil, true
	return true, nil
}
