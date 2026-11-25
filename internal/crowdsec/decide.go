package crowdsec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"time"
)

// Writer pose et lève des bans auprès de la LAPI, avec un compte « machine »
// (watcher) : une clé de bouncer ne permet que de lire les décisions.
// Le compte se crée une fois dans CrowdSec :
//
//	docker exec crowdsec cscli machines add traefik-dashboard --password '…' -f /dev/null
type Writer struct {
	baseURL, machineID, password string
	http                         *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

func NewWriter(baseURL, machineID, password string) *Writer {
	if machineID == "" || password == "" {
		return nil
	}
	c := NewClient(baseURL, "")
	// Délai large : la lecture complète des alertes (7 jours) fait plusieurs Mo.
	// Les écritures sont bornées par le contexte de la requête.
	return &Writer{baseURL: c.baseURL, machineID: machineID, password: password, http: &http.Client{Timeout: 30 * time.Second}}
}

// login renvoie un jeton valide, en le renouvelant une minute avant son échéance.
func (w *Writer) login(ctx context.Context, force bool) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !force && w.token != "" && time.Now().Before(w.expires.Add(-time.Minute)) {
		return w.token, nil
	}
	body, _ := json.Marshal(map[string]any{"machine_id": w.machineID, "password": w.password, "scenarios": []string{}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.baseURL+"/v1/watchers/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := w.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("connexion à la LAPI CrowdSec refusée (HTTP %d) : compte machine %q absent ou mot de passe faux", resp.StatusCode, w.machineID)
	}
	var out struct {
		Token  string    `json:"token"`
		Expire time.Time `json:"expire"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil || out.Token == "" {
		return "", errors.New("LAPI CrowdSec : réponse de connexion illisible")
	}
	if out.Expire.IsZero() {
		out.Expire = time.Now().Add(30 * time.Minute)
	}
	w.token, w.expires = out.Token, out.Expire
	return w.token, nil
}

// do envoie une requête authentifiée, avec une nouvelle connexion si le jeton a
// été refusé (LAPI redémarrée, par exemple).
func (w *Writer) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	return w.doLimit(ctx, method, path, body, 1<<20)
}

func (w *Writer) doLimit(ctx context.Context, method, path string, body []byte, limit int64) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		token, err := w.login(ctx, attempt > 0)
		if err != nil {
			return nil, err
		}
		var rd io.Reader
		if body != nil {
			rd = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, w.baseURL+path, rd)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := w.http.Do(req)
		if err != nil {
			return nil, err
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, limit))
		if err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			continue
		}
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("LAPI CrowdSec : HTTP %d", resp.StatusCode)
		}
		return data, nil
	}
}

// Ban bannit une IP pour la durée donnée. reason devient le « scénario » de la
// décision (affiché par cscli et par la page). L'origine est « cscli », comme un
// ban posé à la main : le bouncer Traefik l'applique et la page la compte.
func (w *Writer) Ban(ctx context.Context, ip netip.Addr, d time.Duration, reason string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	value, scope, origin := ip.String(), "Ip", "cscli"
	dur := d.String()
	alert := map[string]any{
		"capacity":         0,
		"events":           []any{},
		"events_count":     1,
		"leakspeed":        "0",
		"message":          reason,
		"scenario":         reason,
		"scenario_hash":    "",
		"scenario_version": "",
		"simulated":        false,
		"remediation":      true,
		"start_at":         now,
		"stop_at":          now,
		"created_at":       now,
		"source":           map[string]any{"scope": scope, "value": value, "ip": value},
		"decisions": []map[string]any{{
			"duration": dur, "origin": origin, "scenario": reason,
			"scope": scope, "type": "ban", "value": value,
		}},
	}
	body, err := json.Marshal([]any{alert})
	if err != nil {
		return err
	}
	_, err = w.do(ctx, http.MethodPost, "/v1/alerts", body)
	return err
}

// Unban lève toutes les décisions en cours sur l'IP (scénarios CrowdSec compris)
// et renvoie leur nombre.
func (w *Writer) Unban(ctx context.Context, ip netip.Addr) (int, error) {
	data, err := w.do(ctx, http.MethodDelete, "/v1/decisions?ip="+url.QueryEscape(ip.String()), nil)
	if err != nil {
		return 0, err
	}
	var out struct {
		N json.Number `json:"nbDeleted"`
	}
	_ = json.Unmarshal(data, &out)
	n, _ := out.N.Int64()
	return int(n), nil
}
