// Package i18n translates the messages the service shows to people: page
// warnings, API errors and notifications. Logs and configuration errors stay in
// English.
package i18n

import (
	"fmt"
	"strconv"
	"strings"
)

// Lang is a supported language.
type Lang string

const (
	EN Lang = "en"
	FR Lang = "fr"
)

// Langs lists the supported languages, default first.
var Langs = []Lang{EN, FR}

// Parse reads a language tag ("fr", "fr-FR", "en_US"…). ok is false for an
// unsupported or empty tag.
func Parse(s string) (Lang, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, "-_"); i >= 0 {
		s = s[:i]
	}
	for _, l := range Langs {
		if string(l) == s {
			return l, true
		}
	}
	return "", false
}

// FromAcceptLanguage picks the first supported language of an Accept-Language
// header, in the order given (quality values are ignored: browsers list
// languages by preference), or def.
func FromAcceptLanguage(header string, def Lang) Lang {
	for _, part := range strings.Split(header, ",") {
		tag, _, _ := strings.Cut(part, ";")
		if l, ok := Parse(tag); ok {
			return l
		}
	}
	return def
}

// T translates a message and formats it with args (fmt verbs). An unknown key is
// returned as is, so a missing translation is visible but never fatal.
func T(l Lang, key string, args ...any) string {
	e, ok := catalog[key]
	if !ok {
		return key
	}
	f := e.en
	if l == FR && e.fr != "" {
		f = e.fr
	}
	if len(args) == 0 {
		return f
	}
	return fmt.Sprintf(f, args...)
}

// N translates a message that depends on a count: key+".one" or key+".other".
// French uses the singular for 0 and 1, English for 1 only.
func N(l Lang, key string, n int64, args ...any) string {
	one := n == 1
	if l == FR {
		one = n == 0 || n == 1
	}
	if one {
		return T(l, key+".one", args...)
	}
	return T(l, key+".other", args...)
}

// Msg is a message translated when it is shown, for text computed once (a
// warning) but read in several languages.
type Msg struct {
	Key  string
	Args []any
}

// M builds a Msg. An argument that is itself a Msg is translated in the same
// language when the message is rendered.
func M(key string, args ...any) Msg { return Msg{Key: key, Args: args} }

// In renders the message in a language.
func (m Msg) In(l Lang) string {
	args := make([]any, len(m.Args))
	for i, a := range m.Args {
		if sub, ok := a.(Msg); ok {
			args[i] = sub.In(l)
		} else {
			args[i] = a
		}
	}
	return T(l, m.Key, args...)
}

// IsZero reports an empty message.
func (m Msg) IsZero() bool { return m.Key == "" }

// Render renders a list of messages.
func Render(l Lang, msgs []Msg) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.In(l))
	}
	return out
}

// Thousands formats an integer with the language's thousands separator:
// 110742 → "110,742" (en) or "110 742" (fr).
func Thousands(l Lang, n int64) string {
	sep := ","
	if l == FR {
		sep = " " // narrow no-break space
	}
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	head := len(s) % 3
	if head == 0 {
		head = 3
	}
	b.WriteString(s[:head])
	for i := head; i < len(s); i += 3 {
		b.WriteString(sep)
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// Decimal formats a number with one decimal: 1.5 (en) or 1,5 (fr).
func Decimal(l Lang, v float64) string {
	s := strconv.FormatFloat(v, 'f', 1, 64)
	if l == FR {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

// Bytes formats a size in base 1000, like the page: 1536 → "1.5 kB" (en) or
// "1,5 Ko" (fr).
func Bytes(l Lang, n int64) string {
	units := []string{"B", "kB", "MB", "GB", "TB"}
	if l == FR {
		units = []string{"o", "Ko", "Mo", "Go", "To"}
	}
	v := float64(n)
	i := 0
	for v >= 1000 && i < len(units)-1 {
		v /= 1000
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d %s", n, units[0])
	}
	return Decimal(l, v) + " " + units[i]
}
