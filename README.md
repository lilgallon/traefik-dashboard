# traefik-dashboard

Live security dashboard for Traefik access logs. It answers one question: **is anyone I don't know getting through?**

- **Live monitoring**: requests per second, statuses, services, URLs and the latest requests over the last hour, day or week, plus a drag-to-zoom chart. Everything runs in a single small binary (about 30 MB of RAM for about 100,000 requests a day).
- **Known IPs**: name your home, phone, partners or webhooks by IP, CIDR or ASN in a `names.yaml` file. When an **unknown IP gets a successful response** outside the URLs you declared as public, the dashboard turns red and sends a Telegram alert. A daily summary is sent as well.
- **Trust lists**: the dashboard checks that your CrowdSec whitelist and your Traefik `ipAllowList` middlewares match `names.yaml`, and warns you about every difference.
- **Bans** *(optional)*: ban or unban an IP through CrowdSec from the IP detail panel.
- **WAF monitoring**: CrowdSec bans, alerts, triggered scenarios, AppSec blocks and community blocklist size.

Read-only by default: banning is enabled only when you provide a CrowdSec machine account.

> **Status**: early release, extracted from a personal home server. The UI is in French for now; English is planned. See the [roadmap](#roadmap).

## How it works

At startup the service reads the last 7 days of `access.log` and its rotated archives (`access.log.1`, `access.log.2.gz`…), then follows the file live. Rotate it with `copytruncate` (logrotate), which is fully supported. All data stays in memory. The only file it writes is `alerts.json` in `/state`, used to avoid sending the same alert twice.

A request is a **success** if it got a 2xx status, or if it was an accepted WebSocket (Traefik logs those with status 0 when they close; a connection of at least 30 s without status counts as accepted). Redirects (3xx) never turn the dashboard red.

The dashboard never shows a false green: while the backfill runs, when the last log line is older than 15 minutes, or when the page cannot reach the service, the banner says so instead of turning green.

## Quick start

1. Make Traefik write JSON access logs to a file, keeping the `User-Agent` header:

   ```yaml
   # traefik.yml
   accessLog:
     filePath: /logs/access.log
     format: json
     fields:
       headers:
         names:
           User-Agent: keep
   ```

2. Copy [`examples/names.yaml`](examples/names.yaml) and name at least your own IPs.
3. Start the container. See [`examples/docker-compose.yml`](examples/docker-compose.yml) for a complete example with CrowdSec.

   ```bash
   docker run -d --name traefik-dashboard \
     -p 192.168.1.2:8080:8080 \
     -e PUBLIC_URL=http://192.168.1.2:8080 \
     -v /path/to/traefik/logs:/logs:ro \
     -v /path/to/config:/config:ro \
     -v /path/to/geoip:/geo:ro \
     -v traefik-dashboard-state:/state \
     ghcr.io/lilgallon/traefik-dashboard:0.1.0
   ```

> [!WARNING]
> **Never expose the dashboard to the Internet.** It has no authentication yet, and when banning is enabled, anyone who can reach it can ban IPs. Publish it on a LAN address, reach it through a VPN, or put it behind an authenticating proxy. Write endpoints are protected against cross-site requests (custom header, `Origin`, `Sec-Fetch-Site` and `Host` checks), but that is no substitute for authentication.

## Configuration

All settings are environment variables. Only the log directory and `names.yaml` are required.

| Variable | Default | Description |
|---|---|---|
| `LOG_DIR` | `/logs` | Directory containing `access.log` and its rotated archives. |
| `NAMES_FILE` | `/config/names.yaml` | IP names and anonymous URLs. Mount the parent directory (not the file) so edits are picked up. |
| `PUBLIC_URL` | `http://localhost:8080` | URL used to open the dashboard. It is used in alert links, and its host is the only `Host` allowed for write requests. |
| `ALLOWED_HOSTS` | | Extra allowed `Host` values for write requests, comma-separated. |
| `LISTEN` | `:8080` | Listen address. |
| `STATE_DIR` | `/state` | Writable directory for alert de-duplication. |
| `GEO_CITY` / `GEO_ASN` | `/geo/GeoLite2-City.mmdb` / `/geo/GeoLite2-ASN.mmdb` | MaxMind GeoLite2 databases. Without them, countries and operators are not shown, ASN rules do not apply, and alerts are suspended (to avoid false alarms). |
| `CROWDSEC_LAPI_URL` | `http://crowdsec:8080` | CrowdSec local API. |
| `CROWDSEC_BOUNCER_KEY` | | Bouncer key used to read current bans and the blocklist size (`cscli bouncers add traefik-dashboard`). |
| `CROWDSEC_MACHINE_ID` | `traefik-dashboard` | Machine account used to read alerts and ban or unban. |
| `CROWDSEC_MACHINE_PASSWORD` | | Machine password (`cscli machines add traefik-dashboard --password '…' -f /dev/null`). **Empty means read-only**: no alerts history, no bans from the page. |
| `CROWDSEC_WHITELIST` | | CrowdSec whitelist file checked against the `crowdsec` list of `names.yaml`. |
| `TRAEFIK_DYNAMIC` | | Traefik dynamic configuration file whose `ipAllowList` middlewares are checked against the other lists of `names.yaml`. |
| `TELEGRAM_BOT_TOKEN` / `TELEGRAM_CHAT_ID` | | Telegram alerts. Without them, alerts are only logged. |
| `DAILY_AT` | `08:00` | Time of the daily summary (UTC). |

### GeoLite2 databases

The image does not include the MaxMind databases, because their licence forbids redistribution. You can:

- reuse CrowdSec's copies: with the `crowdsecurity/geoip-enrich` parser installed, CrowdSec stores `GeoLite2-City.mmdb` and `GeoLite2-ASN.mmdb` in its data directory (`/var/lib/crowdsec/data`), which you can mount read-only in `/geo`;
- or download them yourself with a free MaxMind account and [`geoipupdate`](https://github.com/maxmind/geoipupdate).

### `names.yaml`

The syntax is documented in [`examples/names.yaml`](examples/names.yaml). In short:

- `names:` lists rules that name requests, by `ip` (IPs or CIDRs) or `asn`, optionally narrowed by `routers` and `paths`. Rules apply per request: an ASN rule should always be narrowed, since an ASN is shared by thousands of customers.
- `anonymous:` lists URLs that answer anyone by design (login pages, status pages, webhooks). A success from an unknown IP on these URLs does not turn the dashboard red.
- `lists:` marks a rule's IPs as members of a trust list: `crowdsec` for the CrowdSec whitelist, or the name of a Traefik `ipAllowList` middleware.

The file is reloaded within 10 seconds. An invalid file is rejected and the previous rules stay active.

## Banning an IP

When `CROWDSEC_MACHINE_PASSWORD` is set, the IP detail panel can ban an IP for 4 h, 24 h, 7 days or 30 days, with an optional reason. The service creates a CrowdSec `ban` decision, which your CrowdSec bouncer then enforces. "Unban" removes **all** decisions for that IP, including those from CrowdSec scenarios.

Banning is refused for private or reserved addresses, and for any IP covered by an `ip` rule of `names.yaml`. Every ban and unban is logged with the browser's IP address.

## API

| Endpoint | Description |
|---|---|
| `GET /api/summary?window=1h\|24h\|7d` | Verdict, counters, IPs, URLs, services and the CrowdSec panel. `?from=…&to=…` (Unix seconds) selects a range within the last 7 days. |
| `GET /api/ip/<ip>?window=…` | IP detail, ban availability and CrowdSec history. |
| `GET /api/stream` | Server-sent events: the latest 200 requests, then one event per second. |
| `POST /api/ip/<ip>/ban` | `{"duration":"4h\|24h\|7d\|30d","reason":"…"}`, with header `X-Traefik-Dashboard: 1`. |
| `DELETE /api/ip/<ip>/ban` | Unban, with header `X-Traefik-Dashboard: 1`. |
| `GET /health` | 503 when the last log line is older than 15 minutes, or when the backfill takes more than 5 minutes. |

## Development

```bash
# Back end
go test ./...

# Front end (Svelte 5 + uPlot, embedded in the binary)
cd web && npm ci && npm run check && npm test && npm run build

# Image
docker build -t traefik-dashboard .
```

The GeoLite2 test is skipped unless the databases are in `testdata/geo/`.

## Roadmap

- English UI (and i18n)
- Optional authentication, required to enable write features
- Allowlist management from the UI: a dedicated Traefik dynamic file, and CrowdSec allowlists
- Generic webhook notifications (ntfy, Gotify, Discord…)
- A clean "no CrowdSec" mode

## Licence

[MIT](LICENSE)
