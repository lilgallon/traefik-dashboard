package store

import (
	"regexp"
	"strings"
)

const (
	idPlaceholder = "{id}"
	maxPathLen    = 160
	maxSegments   = 12
)

// NormalizePath borne la cardinalité des URL : la query string est retirée et les
// segments qui ressemblent à un identifiant deviennent « {id} ».
func NormalizePath(p string) string {
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	if p == "" {
		return "/"
	}
	if !strings.ContainsAny(p[1:], "0123456789") && len(p) <= maxPathLen {
		return p // cas courant : aucun chiffre, donc aucun identifiant
	}
	segs := strings.Split(p, "/")
	if len(segs) > maxSegments {
		segs = append(segs[:maxSegments], "…")
	}
	for i, s := range segs {
		if isID(s) {
			segs[i] = idPlaceholder
		}
	}
	out := strings.Join(segs, "/")
	if len(out) > maxPathLen {
		out = strings.ToValidUTF8(out[:maxPathLen], "") + "…"
	}
	return out
}

// isID : UUID ; ou au moins 12 caractères hexadécimaux ; ou au moins 5 chiffres ;
// ou au moins 8 caractères avec au moins 2 chiffres et une lettre.
func isID(s string) bool {
	if len(s) < 5 {
		return false
	}
	var digits, letters, hex int
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= '0' && c <= '9':
			digits++
			hex++
		case (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F'):
			letters++
			hex++
		case (c >= 'g' && c <= 'z') || (c >= 'G' && c <= 'Z'):
			letters++
		}
	}
	switch {
	case digits == len(s):
		return true
	case hex == len(s) && len(s) >= 12:
		return true
	case len(s) == 36 && hex == 32 && strings.Count(s, "-") == 4:
		return true
	case len(s) >= 8 && digits >= 2 && letters >= 1:
		return true
	}
	return false
}

// Chemins qu'un scanner cherche : une réponse 2xx dessus mérite un coup d'œil.
var sensitiveRE = regexp.MustCompile(`(?i)(/\.env|/\.git/|/\.aws|/\.ssh|wp-(login|admin|config)|xmlrpc\.php|phpmyadmin|/actuator|/server-status|/config\.(json|php|ya?ml)|\.(sql|bak|old|swp)$|/backup)`)

// IsSensitive dit si le chemin (brut, la query string est ignorée) est de ceux
// qu'un scanner cherche.
func IsSensitive(path string) bool {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	return sensitiveRE.MatchString(path)
}

var secretParamRE = regexp.MustCompile(`(?i)([?&][^=&]*(token|key|secret|password|passwd|pwd|auth|code|sig)[^=&]*=)[^&]*`)

// MaskSecrets masque la valeur des paramètres de query qui portent un secret
// (jeton Gotify dans /stream?token=…) : la page n'a pas d'authentification.
func MaskSecrets(path string) string {
	if !strings.Contains(path, "?") {
		return path
	}
	return secretParamRE.ReplaceAllString(path, "${1}…")
}
