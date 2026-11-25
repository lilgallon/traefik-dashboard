package alert

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/logline"
	"github.com/lilgallon/traefik-dashboard/internal/store"
)

// thousands : 110742 → "110 742".
func thousands(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	head := len(s) % 3
	if head > 0 {
		b.WriteString(s[:head])
	}
	for i := head; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

func bytesFR(n int64) string {
	units := []string{"o", "Ko", "Mo", "Go", "To"}
	v := float64(n)
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d o", n)
	}
	return strings.Replace(strconv.FormatFloat(v, 'f', 1, 64), ".", ",", 1) + " " + units[i]
}

func total(c store.ClassCounts) (t int64) {
	for _, v := range c {
		t += v
	}
	return t
}

// DailyText compose le résumé quotidien à partir du résumé « 24 h ».
func DailyText(s store.Summary, publicURL string) string {
	var b strings.Builder
	b.WriteString("📊 <b>Traefik — dernières 24 h</b>\n\n")

	if s.Verdict.Red {
		fmt.Fprintf(&b, "🔴 <b>%d IP inconnue(s) ont obtenu un succès</b>\n", len(s.Verdict.Offenders))
		for i, o := range s.Verdict.Offenders {
			if i == 5 {
				fmt.Fprintf(&b, "  … et %d autre(s)\n", len(s.Verdict.Offenders)-5)
				break
			}
			url := ""
			if len(o.URLs) > 0 {
				url = " → " + esc(o.URLs[0])
			}
			fmt.Fprintf(&b, "  • <code>%s</code> %s : %d succès%s\n", esc(o.IP), esc(strings.TrimSpace(o.Country+" "+o.Org)), o.Count, url)
		}
	} else {
		b.WriteString("🟢 Aucune IP inconnue n'a obtenu de succès\n")
	}

	k := s.KPIs
	fmt.Fprintf(&b, "\nRequêtes : %s (moy. %s req/s, pic %d)\n", thousands(total(k.ByClass)),
		strings.Replace(strconv.FormatFloat(k.AvgRPS, 'f', 1, 64), ".", ",", 1), int64(k.PeakRPS))
	var st []string
	for _, c := range logline.Classes {
		if n := k.ByClass[c]; n > 0 {
			label := string(c)
			if c == logline.ClassAbandon {
				label = "abandons"
			}
			st = append(st, label+" "+thousands(n))
		}
	}
	fmt.Fprintf(&b, "Statuts : %s\n", strings.Join(st, " · "))
	fmt.Fprintf(&b, "Volume : %s entrants, %s sortants\n", bytesFR(k.BytesIn), bytesFR(k.BytesOut))
	fmt.Fprintf(&b, "IP : %d nommées, %d IP non nommées\n\n", s.Counts.NamedIPs, s.Counts.UnknownIPs)

	var with5xx []store.ServiceRow
	for _, svc := range s.Services {
		if svc.Err5xx > 0 {
			with5xx = append(with5xx, svc)
		}
	}
	if len(with5xx) > 0 {
		b.WriteString("⚠️ <b>5xx par router</b>\n")
		for _, svc := range with5xx {
			fmt.Fprintf(&b, "  • %s : %d\n", esc(svc.Router), svc.Err5xx)
		}
	} else {
		b.WriteString("✅ Aucune erreur 5xx\n")
	}

	var shown int
	for _, r := range s.IPs {
		if r.Verdict == store.VerdictKnown {
			continue
		}
		if shown == 0 {
			b.WriteString("\n🌍 <b>Top IP non nommées</b>\n")
		}
		if shown == 8 {
			break
		}
		shown++
		who := strings.TrimSpace(r.Country + " " + r.Org)
		if who != "" {
			who = " " + esc(who)
		}
		ban := ""
		if r.Ban != nil {
			ban = ", bannie (" + esc(r.Ban.Scenario) + ")"
		}
		fmt.Fprintf(&b, "  • <code>%s</code>%s : %s req, %s%s\n", esc(r.IP), who, thousands(total(r.ByClass)), esc(r.Verdict), ban)
	}

	if len(s.Sensitive) > 0 {
		b.WriteString("\n🚨 <b>Chemins sensibles servis (2xx)</b>\n")
		for i, h := range s.Sensitive {
			if i == 8 {
				break
			}
			fmt.Fprintf(&b, "  • <code>%s</code> %s → %d (×%d)\n", esc(h.IP), esc(h.Host+h.Path), h.Status, h.Count)
		}
	}

	if k.ActiveBans < 0 {
		b.WriteString("\n🛡️ CrowdSec injoignable : bans en cours inconnus\n")
	} else {
		fmt.Fprintf(&b, "\n🛡️ CrowdSec : %d ban(s) en cours\n", k.ActiveBans)
	}

	fmt.Fprintf(&b, "\n<a href=\"%s\">Ouvrir le tableau de bord</a>", esc(publicURL))
	return b.String()
}

// nextRun renvoie la prochaine occurrence de at ("HH:MM", UTC) strictement après now.
func nextRun(at string, now time.Time) (time.Time, error) {
	t, err := time.Parse("15:04", at)
	if err != nil {
		return time.Time{}, fmt.Errorf("heure du résumé « %s » invalide (attendu HH:MM)", at)
	}
	now = now.UTC()
	next := time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.UTC)
	if !next.After(now) {
		next = next.Add(24 * time.Hour)
	}
	return next, nil
}

// RunDaily envoie le résumé chaque jour à l'heure dite (UTC), jusqu'à l'arrêt de ctx.
func RunDaily(ctx context.Context, at string, build func() string, n Notifier) {
	for {
		next, err := nextRun(at, time.Now())
		if err != nil {
			log.Printf("résumé quotidien désactivé : %v", err)
			return
		}
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		if err := n.Send(ctx, build()); err != nil {
			log.Printf("résumé quotidien non envoyé : %v", err)
		}
	}
}
