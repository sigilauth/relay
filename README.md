# Sigil Auth Push Relay

Stateful Go service for APNs/FCM push notification delivery. Maps device fingerprints to push tokens.

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌──────────┐
│   Sigil     │────▶│    Relay    │────▶│ APNs/FCM │
│   Server    │     │  (stateful) │     │          │
└─────────────┘     └─────────────┘     └──────────┘
                          │
                          ▼
                    ┌──────────┐
                    │Postgres  │
                    │fp→token  │
                    └──────────┘
```

## Components

- `cmd/relay` — Main entrypoint
- `internal/push/apns` — APNs HTTP/2 client (.p8 key)
- `internal/push/fcm` — FCM HTTP v1 client (service account JSON)
- `internal/store` — Postgres access via pgx
- `internal/verify` — ECDSA signature verification for server→relay calls
- `internal/ratelimit` — Per-fingerprint rate limiting (10/min)
- `migrations` — golang-migrate compatible SQL files

## Acceptance Criteria (B2)

- `/devices/register` computes fingerprint = SHA256(pk)
- `/push` verifies server sig; 404 unknown fp; 502 APNs/FCM unreachable
- Invalid token (APNs BadDeviceToken / FCM NotRegistered) → evict
- Concurrent re-register: last-write-wins
- Rate limit 10/min per fingerprint
- Token eviction after 10 consecutive failures or 90d stale
- HTTPS only; structured logs; Prometheus `/metrics`
- Daily pg_dump to S3/GCS configurable
- Coverage 85/80 (line/branch)
- Load: 1000 pushes/sec sustained on CI runner

## TDD Required

Per DECISIONS.md D5, all code must be test-first. No merge without:
- Unit tests for all business logic
- Integration tests with testcontainers-go (Postgres)
- Load tests (k6 or vegeta)
- Coverage thresholds met

## Libraries

- `github.com/jackc/pgx/v5` — Postgres driver
- `github.com/sideshow/apns2` — APNs HTTP/2
- `firebase.google.com/go/v4/messaging` — FCM
- `github.com/go-chi/chi/v5` — HTTP router
- `golang.org/x/time/rate` — Rate limiting
- `github.com/prometheus/client_golang` — Metrics
- `github.com/testcontainers/testcontainers-go` — Integration tests

## Configuration

### Required Environment Variables

- `SERVER_PUBLIC_KEY` — Base64-encoded compressed P-256 public key (33 bytes) of the Sigil server that will send push requests. Used to verify ECDSA signatures on `/push` requests.

### Optional Environment Variables

- `PORT` — HTTP listen port (default: `8080`)
- `RELAY_MODE` — Operating mode: `production` (default) or `mock`
- `DATABASE_URL` — PostgreSQL connection string (required for production mode, ignored in mock mode)

### Security Note

The relay **requires** `SERVER_PUBLIC_KEY` to start. This enforces fail-closed security — unsigned push requests are rejected. The test environment (`docker-compose.test.yml`) includes a test key. **DO NOT use the test key in production.**

## Running

```bash
# Mock mode for local development (no Postgres, no APNs/FCM)
export RELAY_MODE=mock
export SERVER_PUBLIC_KEY="<base64-encoded-compressed-P256-public-key>"
export PORT=8090
go run ./cmd/relay

# Production (requires real server public key + Postgres)
export SERVER_PUBLIC_KEY="<base64-encoded-compressed-P256-public-key>"
export DATABASE_URL="postgresql://relay:password@postgres:5432/relay"
docker-compose up relay

# With Docker Compose (test environment)
docker-compose -f docker-compose.test.yml up
```

### Mock Mode for Local Development

Mock mode allows running the relay locally without Postgres or APNs/FCM credentials. Perfect for development and testing:

**What it does:**
- In-memory storage (no Postgres connection required)
- Push notifications printed to stdout as pretty JSON instead of sent to APNs/FCM
- ECDSA signature verification remains ACTIVE (security boundary enforced)
- Rate limiting remains ACTIVE (10/min per fingerprint)
- All HTTP endpoints unchanged

**Quick start:**
```bash
export RELAY_MODE=mock
export SERVER_PUBLIC_KEY="A+xLkrjaWue4L0KbKy8vDlIr0NLWLBYtNyJ4ChfbPuH+"
export PORT=8090
go run ./cmd/relay
```

**Inspecting dispatched pushes:**

All mock push dispatches are printed to stdout as JSON:
```json
{
  "event": "push_dispatched",
  "token": "device-push-token-here",
  "payload": {
    "challenge": "base64-challenge-here",
    "timestamp": "2026-04-26T16:00:00Z"
  },
  "would_send_via": "mock"
}
```

Grep for specific events:
```bash
go run ./cmd/relay 2>&1 | grep push_dispatched
```

**When NOT to use mock mode:**
- Testing actual device delivery (use production mode with real APNs/FCM)
- Load testing APNs/FCM behavior (use production mode)
- Integration testing with real backend services (use production mode)

Mock mode is for **local relay development only** — it validates request handling, rate limiting, and signature verification without external dependencies.

## Testing

```bash
# Unit tests
go test ./...

# Integration tests (requires Docker)
go test ./test/integration -tags=integration

# Load tests
k6 run test/load/push-load.js

# Coverage
go test -cover -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

## Status

**Implementation Complete (B2)** — Core relay functionality complete with signature verification, rate limiting, and push delivery. Tests passing. Ready for production deployment.

**Security:** All push requests require valid ECDSA signature from configured server (fail-closed).
