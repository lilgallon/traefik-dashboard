package alert

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestTelegramSend(t *testing.T) {
	var path, chat, mode, text string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		path, chat, mode, text = r.URL.Path, r.Form.Get("chat_id"), r.Form.Get("parse_mode"), r.Form.Get("text")
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	tg := NewTelegram("123:SECRET", "42")
	tg.baseURL = srv.URL
	if err := tg.Send(context.Background(), "<b>a & b</b>"); err != nil {
		t.Fatal(err)
	}
	if path != "/bot123:SECRET/sendMessage" || chat != "42" || mode != "HTML" || text != "<b>a & b</b>" {
		t.Fatalf("path=%q chat=%q mode=%q text=%q", path, chat, mode, text)
	}
	// Message trop long pour Telegram : tronqué, pas refusé.
	if err := tg.Send(context.Background(), strings.Repeat("é", 5000)); err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(text)); n > 4096 {
		t.Fatalf("message de %d caractères envoyé", n)
	}
}

func TestTelegramErrorsHideToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"ok":false,"description":"Bad Request: chat not found"}`, http.StatusBadRequest)
	}))
	tg := NewTelegram("123:SECRET", "42")
	tg.baseURL = srv.URL
	err := tg.Send(context.Background(), "x")
	if err == nil || !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("erreur Telegram : %v", err)
	}
	srv.Close()
	err = tg.Send(context.Background(), "x") // connexion refusée : l'URL est dans l'erreur Go
	if err == nil || strings.Contains(err.Error(), "SECRET") {
		t.Fatalf("le jeton fuit dans l'erreur : %v", err)
	}
}

func TestAsyncRetriesAndNeverBlocks(t *testing.T) {
	var calls atomic.Int32
	flaky := notifierFunc(func(context.Context, string) error {
		if calls.Add(1) < 3 {
			return context.DeadlineExceeded
		}
		return nil
	})
	a := NewAsync(flaky, func(string, ...any) {})
	a.retryDelay = time.Millisecond
	if err := a.Send(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() != 3 {
		t.Fatalf("%d tentatives", calls.Load())
	}

	// File pleine : Send renvoie une erreur tout de suite, sans bloquer la lecture des logs.
	block := make(chan struct{})
	slow := NewAsync(notifierFunc(func(context.Context, string) error { <-block; return nil }), func(string, ...any) {})
	defer close(block)
	start := time.Now()
	var dropped int
	for i := 0; i < asyncQueue+10; i++ {
		if slow.Send(context.Background(), "x") != nil {
			dropped++
		}
	}
	if dropped == 0 || time.Since(start) > time.Second {
		t.Fatalf("dropped=%d en %v", dropped, time.Since(start))
	}
}

type notifierFunc func(context.Context, string) error

func (f notifierFunc) Send(ctx context.Context, html string) error { return f(ctx, html) }
