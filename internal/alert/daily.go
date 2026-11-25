package alert

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/i18n"
	"github.com/lilgallon/traefik-dashboard/internal/logline"
	"github.com/lilgallon/traefik-dashboard/internal/store"
)

func total(c store.ClassCounts) (t int64) {
	for _, v := range c {
		t += v
	}
	return t
}

// DailyText compose le résumé quotidien à partir du résumé « 24 h ».
func DailyText(s store.Summary, publicURL string, l i18n.Lang) string {
	var b strings.Builder
	b.WriteString(i18n.T(l, "daily.title") + "\n\n")

	incomplete := s.Loading || s.LastLine.IsZero() || s.GeneratedAt.Sub(s.LastLine) >= 15*time.Minute
	if incomplete {
		b.WriteString(i18n.T(l, "daily.incomplete") + "\n")
	}
	if s.Verdict.Red {
		n := s.Verdict.TotalIPs
		b.WriteString(i18n.N(l, "daily.red", int64(n), n) + "\n")
		for i, o := range s.Verdict.Offenders {
			if i == 5 {
				b.WriteString(i18n.T(l, "daily.more", n-5) + "\n")
				break
			}
			url := ""
			if len(o.URLs) > 0 {
				url = " → " + esc(o.URLs[0])
			}
			b.WriteString(i18n.N(l, "daily.offender", o.Count, esc(o.IP), esc(strings.TrimSpace(o.Country+" "+o.Org)), o.Count, url) + "\n")
		}
	} else if !incomplete {
		b.WriteString(i18n.T(l, "daily.green") + "\n")
	}

	k := s.KPIs
	b.WriteString("\n" + i18n.T(l, "daily.requests", i18n.Thousands(l, total(k.ByClass)), i18n.Decimal(l, k.AvgRPS), int64(k.PeakRPS)) + "\n")
	var st []string
	for _, c := range logline.Classes {
		if n := k.ByClass[c]; n > 0 {
			label := string(c)
			if c == logline.ClassAbandon {
				label = i18n.T(l, "daily.aborted")
			}
			st = append(st, label+" "+i18n.Thousands(l, n))
		}
	}
	b.WriteString(i18n.T(l, "daily.statuses", strings.Join(st, " · ")) + "\n")
	b.WriteString(i18n.T(l, "daily.volume", i18n.Bytes(l, k.BytesIn), i18n.Bytes(l, k.BytesOut)) + "\n")
	b.WriteString(i18n.T(l, "daily.ips", s.Counts.NamedIPs, s.Counts.UnknownIPs) + "\n\n")

	var with5xx []store.ServiceRow
	for _, svc := range s.Services {
		if svc.Err5xx > 0 {
			with5xx = append(with5xx, svc)
		}
	}
	if len(with5xx) > 0 {
		b.WriteString(i18n.T(l, "daily.5xx_title") + "\n")
		for _, svc := range with5xx {
			b.WriteString(i18n.T(l, "daily.5xx_line", esc(svc.Router), svc.Err5xx) + "\n")
		}
	} else {
		b.WriteString(i18n.T(l, "daily.no5xx") + "\n")
	}

	var shown int
	for _, r := range s.IPs {
		if r.Verdict == store.VerdictKnown {
			continue
		}
		if shown == 0 {
			b.WriteString("\n" + i18n.T(l, "daily.top") + "\n")
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
			ban = i18n.T(l, "daily.banned", esc(r.Ban.Scenario))
		}
		verdict := i18n.T(l, "verdict."+r.Verdict)
		b.WriteString(i18n.T(l, "daily.ip_line", esc(r.IP), who, i18n.Thousands(l, total(r.ByClass)), esc(verdict), ban) + "\n")
	}

	if len(s.Sensitive) > 0 {
		b.WriteString("\n" + i18n.T(l, "daily.sensitive") + "\n")
		for i, h := range s.Sensitive {
			if i == 8 {
				break
			}
			fmt.Fprintf(&b, "  • <code>%s</code> %s → %d (×%d)\n", esc(h.IP), esc(h.Host+h.Path), h.Status, h.Count)
		}
	}

	if k.ActiveBans < 0 {
		b.WriteString("\n" + i18n.T(l, "daily.cs_unreachable") + "\n")
	} else {
		b.WriteString("\n" + i18n.N(l, "daily.cs_bans", int64(k.ActiveBans), k.ActiveBans) + "\n")
	}

	fmt.Fprintf(&b, "\n<a href=\"%s\">%s</a>", esc(publicURL), i18n.T(l, "tg.open"))
	return b.String()
}

// nextRun renvoie la prochaine occurrence de at ("HH:MM", UTC) strictement après now.
func nextRun(at string, now time.Time) (time.Time, error) {
	t, err := time.Parse("15:04", at)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid daily summary time %q (expected HH:MM)", at)
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
			log.Printf("daily summary disabled: %v", err)
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
			log.Printf("daily summary not sent: %v", err)
		}
	}
}
