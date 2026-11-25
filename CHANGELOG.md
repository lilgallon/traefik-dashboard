# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).
Each release is published as a multi-arch image (`linux/amd64`, `linux/arm64`)
on `ghcr.io/lilgallon/traefik-dashboard`.

## [Unreleased]

## [0.1.0] - 2026-11-25

First public release.

### Added

- Live monitoring of Traefik JSON access logs: requests per second, statuses,
  services, URLs and the latest requests over the last hour, day or week, with
  a drag-to-zoom chart and a server-sent events stream.
- Backfill of the last 7 days from `access.log` and its rotated archives
  (including `.gz`), then live following with `copytruncate` support.
- Known IPs in `names.yaml` (by IP, CIDR or ASN) and anonymous URLs; the
  dashboard turns red when an unknown IP gets a successful response.
- Telegram alerts with de-duplication across restarts, and a daily summary.
- Trust list checks: the CrowdSec whitelist and Traefik `ipAllowList`
  middlewares are compared with `names.yaml`.
- CrowdSec panel: bans, alerts, triggered scenarios, AppSec blocks and
  community blocklist size.
- Optional ban and unban from the IP detail panel, enabled only with a
  CrowdSec machine account; write endpoints are protected against cross-site
  requests.
- MaxMind GeoLite2 countries and operators (databases mounted at runtime).
- English and French UI and notifications.
- `/health` endpoint and container healthcheck.
- Local environment (`make dev`) with synthetic logs, a real CrowdSec and a
  Telegram mock, and `make screenshots`.

[Unreleased]: https://github.com/lilgallon/traefik-dashboard/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/lilgallon/traefik-dashboard/releases/tag/v0.1.0
