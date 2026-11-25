# Traefik dashboard: Svelte front end embedded in a single Go binary.
#
# The front end and the binary are built on the build platform and the binary is
# cross-compiled, so multi-arch images do not need emulation.
#
# GeoLite2 databases are NOT included (MaxMind's licence forbids redistributing
# them): mount them in /geo at runtime (see README).

# ── Front end ─────────────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM node:26.13.0-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json web/.npmrc ./
RUN npm ci
COPY web/ ./
RUN npm run build

# ── Binary ────────────────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY web/embed.go ./web/embed.go
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/traefik-dashboard ./cmd/traefik-dashboard

# ── Final image ───────────────────────────────────────────────────────────────
FROM alpine:3.24.4
RUN apk add --no-cache ca-certificates && mkdir -m 0755 /geo /state && chown 1000:1000 /state
COPY --from=build /out/traefik-dashboard /usr/local/bin/traefik-dashboard
# Soft Go heap limit: the garbage collector works earlier, well before the
# container limit (target: < 100 MB in normal operation).
ENV GOMEMLIMIT=80MiB
USER 1000:1000
EXPOSE 8080
VOLUME /state
# /health returns 503 when the last log line read is older than 15 minutes.
HEALTHCHECK --interval=30s --timeout=5s --start-period=90s --retries=3 \
    CMD wget -qO /dev/null http://127.0.0.1:8080/health || exit 1
ENTRYPOINT ["/usr/local/bin/traefik-dashboard"]
