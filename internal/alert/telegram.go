// Package alert envoie les alertes Telegram du tableau de bord : succès d'une IP
// inconnue, volume anormal, pic de 5xx, chemin sensible servi, trou de données,
// et le résumé quotidien.
package alert

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Notifier envoie un message au format HTML de Telegram.
type Notifier interface {
	Send(ctx context.Context, html string) error
}

const telegramMaxRunes = 4096

type Telegram struct {
	token, chatID string
	baseURL       string
	http          *http.Client
}

func NewTelegram(token, chatID string) *Telegram {
	return &Telegram{token: token, chatID: chatID, baseURL: "https://api.telegram.org",
		http: &http.Client{Timeout: 10 * time.Second}}
}

// WithBaseURL remplace l'URL de l'API Telegram (proxy, ou faux serveur de
// l'environnement local).
func (t *Telegram) WithBaseURL(u string) *Telegram {
	if u != "" {
		t.baseURL = strings.TrimRight(u, "/")
	}
	return t
}

func (t *Telegram) Send(ctx context.Context, text string) error {
	if r := []rune(text); len(r) > telegramMaxRunes {
		// Coupe sur une ligne entière pour ne pas laisser une balise ouverte.
		cut := string(r[:telegramMaxRunes-2])
		if i := strings.LastIndexByte(cut, '\n'); i > 0 {
			cut = cut[:i]
		}
		text = cut + "\n…"
	}
	form := url.Values{
		"chat_id":                  {t.chatID},
		"parse_mode":               {"HTML"},
		"disable_web_page_preview": {"true"},
		"text":                     {text},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		t.baseURL+"/bot"+t.token+"/sendMessage", strings.NewReader(form.Encode()))
	if err != nil {
		return t.scrub(err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := t.http.Do(req)
	if err != nil {
		return t.scrub(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var r struct {
			Description string `json:"description"`
		}
		_ = json.Unmarshal(body, &r)
		return fmt.Errorf("Telegram : HTTP %d %s", resp.StatusCode, r.Description)
	}
	return nil
}

// scrub retire le jeton des erreurs : Go y met l'URL complète de la requête.
func (t *Telegram) scrub(err error) error {
	return errors.New(strings.ReplaceAll(err.Error(), t.token, "***"))
}

const asyncQueue = 100

// Async envoie en arrière-plan, avec quelques nouveaux essais : la lecture des
// logs ne doit jamais attendre Telegram.
type Async struct {
	next       Notifier
	queue      chan string
	logf       func(format string, args ...any)
	retryDelay time.Duration
}

func NewAsync(next Notifier, logf func(format string, args ...any)) *Async {
	a := &Async{next: next, queue: make(chan string, asyncQueue), logf: logf, retryDelay: 5 * time.Second}
	go a.run()
	return a
}

func (a *Async) Send(_ context.Context, text string) error {
	select {
	case a.queue <- text:
		return nil
	default:
		return errors.New("file d'envoi Telegram pleine, alerte abandonnée")
	}
}

func (a *Async) run() {
	for text := range a.queue {
		var err error
		for attempt := 0; attempt < 3; attempt++ {
			if attempt > 0 {
				time.Sleep(a.retryDelay * time.Duration(attempt))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			err = a.next.Send(ctx, text)
			cancel()
			if err == nil {
				break
			}
		}
		if err != nil {
			a.logf("alerte Telegram non envoyée : %v", err)
		}
	}
}

// Log est le Notifier utilisé quand Telegram n'est pas configuré : les alertes
// partent dans les journaux du conteneur.
type Log struct {
	Logf func(format string, args ...any)
}

func (l Log) Send(_ context.Context, text string) error {
	l.Logf("alerte (Telegram non configuré) : %s", strings.ReplaceAll(text, "\n", " | "))
	return nil
}

func esc(s string) string { return html.EscapeString(s) }
