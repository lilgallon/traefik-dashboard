// Package crowdsec lit les bans en cours auprès de la LAPI de CrowdSec, avec une
// clé de bouncer (lecture seule), et en pose ou en lève avec un compte machine
// (Writer).
package crowdsec

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/lilgallon/traefik-dashboard/internal/store"
)

type Client struct {
	baseURL, apiKey string
	http            *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

type decision struct {
	Duration string `json:"duration"`
	Origin   string `json:"origin"`
	Scenario string `json:"scenario"`
	Scope    string `json:"scope"`
	Type     string `json:"type"`
	Value    string `json:"value"`
}

// Bans renvoie les bans d'IP en cours décidés localement (scénarios CrowdSec et
// cscli). La blocklist communautaire (des dizaines de milliers d'entrées) est
// laissée de côté : elle ne dit rien de ce qui se passe ici.
func (c *Client) Bans(ctx context.Context) (map[netip.Addr]store.Ban, error) {
	if c.apiKey == "" {
		return nil, errors.New("CrowdSec bouncer key missing (CROWDSEC_BOUNCER_KEY)")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/v1/decisions?type=ban&scope=ip&origins=crowdsec,cscli", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Api-Key", c.apiKey)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("CrowdSec LAPI: HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	var decisions []decision // « null » quand il n'y a aucune décision
	if err := json.Unmarshal(body, &decisions); err != nil {
		return nil, fmt.Errorf("CrowdSec LAPI: unreadable response: %w", err)
	}
	now := time.Now()
	bans := make(map[netip.Addr]store.Ban, len(decisions))
	for _, d := range decisions {
		if !strings.EqualFold(d.Type, "ban") || !strings.EqualFold(d.Scope, "ip") {
			continue
		}
		ip, err := netip.ParseAddr(d.Value)
		if err != nil {
			continue
		}
		ban := store.Ban{Scenario: d.Scenario, Manual: d.Origin == "cscli"}
		if dur, err := time.ParseDuration(d.Duration); err == nil {
			ban.Until = now.Add(dur).UTC().Truncate(time.Second)
		}
		bans[ip.Unmap()] = ban
	}
	return bans, nil
}
