package alert

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// cooldowns mémorise la dernière alerte envoyée par sujet, sur disque pour
// survivre à un redémarrage.
type cooldowns struct {
	path string
	last map[string]time.Time
}

func loadCooldowns(path string) *cooldowns {
	c := &cooldowns{path: path, last: map[string]time.Time{}}
	if b, err := os.ReadFile(path); err == nil {
		if json.Unmarshal(b, &c.last) != nil || c.last == nil {
			c.last = map[string]time.Time{} // fichier corrompu : on repart de zéro
		}
	}
	return c
}

// ready dit si l'alerte key peut partir (aucun envoi depuis every).
func (c *cooldowns) ready(key string, at time.Time, every time.Duration) bool {
	last, ok := c.last[key]
	return !ok || at.Sub(last) >= every
}

// mark enregistre l'envoi et écrit le fichier (renommage atomique).
func (c *cooldowns) mark(key string, at time.Time) error {
	c.last[key] = at
	for k, t := range c.last { // rien ne se répète au-delà de 24 h
		if at.Sub(t) > 48*time.Hour {
			delete(c.last, k)
		}
	}
	b, err := json.Marshal(c.last)
	if err != nil {
		return err
	}
	tmp := filepath.Join(filepath.Dir(c.path), "."+filepath.Base(c.path)+".tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}
