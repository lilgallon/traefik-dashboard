// telegram-mock imite l'API Telegram (sendMessage) pour l'environnement local :
// chaque alerte est écrite dans ses logs et listée sur sa page d'accueil.
//
//	TELEGRAM_API_URL=http://telegram-mock:8081 dans le tableau de bord
package main

import (
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type message struct {
	at   time.Time
	text string
}

func main() {
	var (
		mu   sync.Mutex
		msgs []message
	)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sendMessage") && r.Method == http.MethodPost {
			text := r.FormValue("text")
			log.Printf("alerte reçue :\n%s\n", text)
			mu.Lock()
			msgs = append(msgs, message{time.Now(), text})
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"ok":true,"result":{"message_id":1}}`))
			return
		}
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var b strings.Builder
		b.WriteString(`<!doctype html><meta charset="utf-8"><title>Telegram mock</title><body style="font-family:sans-serif;max-width:50rem;margin:2rem auto">`)
		b.WriteString("<h1>Alertes reçues</h1>")
		if len(msgs) == 0 {
			b.WriteString("<p>Aucune pour l'instant.</p>")
		}
		for i := len(msgs) - 1; i >= 0; i-- {
			// Texte échappé : on voit le HTML de Telegram (b, code, a) tel qu'envoyé.
			b.WriteString(`<pre style="white-space:pre-wrap;background:#f4f4f4;padding:1rem">`)
			b.WriteString(html.EscapeString(msgs[i].at.Format(time.RFC3339)) + "\n\n" + html.EscapeString(msgs[i].text))
			b.WriteString("</pre>")
		}
		w.Write([]byte(b.String()))
	})
	log.Printf("faux Telegram sur :8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
