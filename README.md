# Ads API

A solution to the six tasks in [INSTRUCTIONS.md](INSTRUCTIONS.md), built on the original Go HTTP/usecase/DAO structure. PostgreSQL remains the source of truth. The supplied `users/` service and client are preserved.

The work focuses on correct behavior, safe retries, concurrent requests and useful tests. Production extensions are described below as proposals, keeping the assessment understandable and avoiding unnecessary infrastructure.

## Assessment requirements

| Requirement | Solution |
| --- | --- |
| Fix `GetAdByID` | Pass the requested ID to the DAO; return 404 for a missing ad without calling users. |
| Delete an ad | HTTP endpoint, usecase and atomic owner-qualified SQL deletion. Success is an empty 204; missing, repeated and non-owned deletion returns 404. |
| Include the username | Use the supplied users client through an ads adapter. Return the verified owner's nickname or nullable fallback according to the documented failure policy. |
| Introduce `price` | Expose a JSON number in euros, parse it exactly and keep integer cents internally. Retain deprecated `price_cents` while clients migrate. |
| Fix bulk import | Validate the whole batch and commit ads plus the operation receipt together. Require an idempotency key to replay the original IDs after retries. |
| Five publications per rolling 24 hours | Serialize each owner's individual publications with a PostgreSQL transaction lock and retained publication history. Imports are excluded; deletion does not refund quota. |

Individual creation also supports optional idempotency keys. Request validation, bounded pagination, user-service protection and regression tests support these requirements.

## Architecture

```text
ads/internal/cmd/ads-api/
  main.go, config.go     Startup, configuration and dependency wiring
  http/                  Requests, responses and routing
  usecase/               Business operations and fallback policy
  domain/                Ads, identifiers and validation
  dao/                   Storage and transaction interfaces
    postgres/            SQL queries and schema files
  userclient/            Supplied-client adapter and bounded resilience
  userlookup/            Shared lookup error contract
common/                  HTTP server, middleware and response helpers
users/                   Original supplied service and client
```

HTTP calls usecases, and usecases depend on storage interfaces. The users adapter classifies failures; the usecase decides whether a missing name is acceptable; HTTP maps application errors. Names and caches do not control ownership.

## Run locally

Requirements: Docker Desktop/Engine with Compose and Linux containers. Local Go development requires Go 1.26.10 or a later supported patch. Docker supplies the toolchain for container verification.

```sh
docker compose -p backend-go-final up -d --build --wait
curl http://localhost:8080/health
curl http://localhost:8080/ads/1
```

The API uses `8080`, PostgreSQL `15440`; both published ports bind to loopback. `/health` returns 200 with an empty body and checks process liveness only. The users service is internal to the Compose network. Development database credentials are `ads:ads`.

The Compose file retains the name `backend-go-v2` from the source copy. Use `-p backend-go-final` consistently for application commands to keep this checkout's containers and volume separate. Other local stacks must release the same host ports before startup. This is especially important if several project copies exist on the same machine.

On a fresh volume, PostgreSQL applies schema SQL and sample data. It does not apply new migrations to an existing volume. Never delete a volume to upgrade an existing database; follow [database changes](DECISIONS.md#database-changes). `docker compose -p backend-go-final down` preserves application data.

For native Go execution, the defaults are `HTTP_ADDR=:8080`, `DATABASE_URL=postgres://ads:ads@localhost:5432/ads?sslmode=disable`, `IDENTITY_URL=http://localhost:8081`, and `IDENTITY_TIMEOUT=250ms`. Adjust the database URL to the actual exposed port. The default users endpoint must be separately reachable for enrichment.

## API

| Method | Path | Identity required | Behavior |
| --- | --- | --- | --- |
| GET | `/health` | no | Process liveness |
| GET | `/ads/{id}` | no | Ad details and nullable username |
| GET | `/ads` | no | Filtered page, descending IDs |
| POST | `/ads` | yes | Individual publication with rolling quota |
| DELETE | `/ads/{id}` | yes | Owner-qualified deletion |
| POST | `/ads/import` | yes | Atomic import of 1-100 ads, outside the publication quota |

The [OpenAPI contract](ads/internal/cmd/ads-api/specs/openapi.yaml) describes schemas, validation and responses.

Examples below use Bash quoting. In Windows PowerShell use `curl.exe` for simple reads; CMD and older PowerShell require different escaping for JSON. A PowerShell creation example is included separately.

```sh
curl 'http://localhost:8080/ads?title=bike&min_price=10&limit=20'

curl -X POST http://localhost:8080/ads \
  -H 'Authorization: 00000000-0000-0000-0000-000000000001' \
  -H 'Idempotency-Key: publish-bike-1' \
  -H 'Content-Type: application/json' \
  -d '{"title":"Mountain bike","price":120.50,"photo_url":"https://example.com/bike.jpg"}'

curl -X POST http://localhost:8080/ads/import \
  -H 'Authorization: 00000000-0000-0000-0000-000000000001' \
  -H 'Idempotency-Key: import-chairs-1' \
  -H 'Content-Type: application/json' \
  -d '{"ads":[{"title":"Chair","price":20,"photo_url":"https://example.com/chair.jpg"}]}'
```

PowerShell:

```powershell
$headers = @{
    Authorization = '00000000-0000-0000-0000-000000000001'
    'Idempotency-Key' = 'publish-bike-1'
}
$body = @{
    title = 'Mountain bike'
    price = 120.50
    photo_url = 'https://example.com/bike.jpg'
} | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri 'http://localhost:8080/ads' `
    -Headers $headers -ContentType 'application/json' -Body $body
```

### Identity and compatibility

Writes accept the template's plain UUID in `Authorization`. UUID normalization prevents alternate spellings from obtaining separate quotas. **A UUID header is not authentication.** This educational contract must be replaced or protected by a verified identity boundary before internet deployment.

Send exactly one of `price` in euros or deprecated `price_cents`. Responses retain both during migration. Storage and calculations use integer cents; fractional cents, negative amounts, null, strings and out-of-range values are rejected. The maximum is EUR 10,000,000,000.00. JSON numeric formatting does not replace UI currency formatting.

Price filters accept either the euro family (`min_price`, `max_price`) or the legacy cents family, never a mixture. Bounds are inclusive; `max_price=0` selects free ads. Default page size is 50, maximum 100. Follow `next_before_id` using `before_id` with unchanged filters. Title search treats `%` and `_` literally.

Coordinate supported web/mobile clients before retiring legacy fields. Request telemetry cannot prove that clients stopped reading a response field.

### Retry and failure behavior

Persist a fresh `Idempotency-Key` for each logical operation and reuse it after timeouts or lost responses. Import requires it; individual creation permits omission for compatibility. A key is scoped by owner and operation. The same normalized ordered content returns the original IDs; different content returns 409. Equivalent euro/cent representations identify the same payload.

Without a key, individual-create retries can duplicate an ad. Different keys deliberately request different operations. Receipts have no expiry, survive deletion and never recreate deleted ads. Failed transactions retain neither ads nor receipts. A timeout does not prove rollback: the commit may have succeeded before the response was lost.

JSON bodies are limited to 1 MiB. Unknown fields, duplicate keys, excessive nesting, trailing documents and malformed search encoding are rejected. Titles are trimmed and contain 1-200 Unicode characters without NUL; photo URLs must be absolute HTTP(S), without credentials, and at most 2048 bytes. URLs are stored, never fetched by this service.

Application errors return `{"error":"..."}`: 400 validation, 401 invalid identity, 404 missing/non-owned ad, 409 conflicting key, 413 oversized body, 429 quota, 502 invalid/opaque users response, 504 request deadline and 500 unexpected failure. Cancellation stops response handling. Router-generated 404/405 and transport errors retain standard HTTP behavior.

## User lookup resilience

A valid lookup returns the owner's nickname. Missing users, connection failures, lookup timeouts, an open circuit or exhausted lookup capacity return the ad with `username: null`. Invalid JSON, mismatched owner data and opaque non-404 HTTP failures return 502. The supplied client does not expose typed statuses, so an upstream 503 also follows this rule. Its error text is not parsed. Clients must support a nullable username.

Only valid successes are cached. Concurrent requests for the same canonical owner share a lookup. Cache hits do not extend expiry; expired names are never served. Lists do not call users. No Redis or automatic retry is used.

| Environment variable | Default | Meaning |
| --- | --- | --- |
| IDENTITY_TIMEOUT | 250ms | Upstream call timeout |
| IDENTITY_CACHE_SIZE | 1000 | Maximum cached users per process; 0 disables caching |
| IDENTITY_CACHE_TTL | 1m | Maximum age of a cached display name |
| IDENTITY_MAX_CONCURRENT | 20 | Maximum active upstream calls per process, without a waiting queue for distinct users |
| IDENTITY_FAILURE_THRESHOLD | 5 | Consecutive network failures/timeouts before opening |
| IDENTITY_OPEN_DURATION | 5s | Delay before allowing one recovery probe |

These defaults are initial settings, not production sizing. Invalid numeric resilience settings and nonpositive durations fall back as described in [TESTING.md](TESTING.md). Cached names may remain unchanged until expiry and must never determine account status or permissions.

## Verify the solution

From this checkout in PowerShell:

```powershell
./scripts/verify.ps1
```

The command runs the pinned linter, checks formatting, runs `go vet`, unit and real PostgreSQL/HTTP integration tests with race detection, scans known vulnerabilities and builds the ads API Docker image. Docker must be running; host Go and a C compiler are not required. The source is mounted read-only and each database test owns a temporary schema in a dedicated `_test` database.

The verification and optional load scripts retain the source copy's `backend-go-v2` test project/network names. They target the test service rather than the application database. Do not run these scripts concurrently from different project copies or use the disposable test service to store application data.

For unit tests with local Go:

```sh
go test -count=1 ./...
```

This excludes integration tests and race detection. See [TESTING.md](TESTING.md) for integration setup, parser fuzz checks and outage scenarios. The optional `./scripts/load-test.ps1` runs a short local mixed workload; [its report](validation/LOAD_TEST.md) explains the limits. Saved logs describe their recorded checkout and run; rerun checks after code changes. This checkout was also verified on 2026-11-02 against a separate disposable PostgreSQL container: formatting, vet, unit/integration tests with race detection and vulnerability scanning passed.

## Lint and CI checks

Run lint alone from PowerShell:

```powershell
./scripts/lint.ps1
```

This uses `golangci-lint` **v2.15.0** in Docker and the committed `.golangci.yml`, matching CI. It enables `errcheck`, `govet`, `ineffassign`, `staticcheck` and `unused`. Integration and load-test files are included in analysis; the linter does not execute tests or need PostgreSQL. The lint scope is `ads` and `common`, which are the assessment's editable packages. The original `users` packages still compile and run in the general test suite.

For an installed copy of the same linter version:

```sh
golangci-lint run --config=.golangci.yml ./ads/... ./common/...
```

The full `./scripts/verify.ps1` command includes lint and image building as well as the existing tests and security scan. `.github/workflows/test.yml` runs lint and builds the Docker image on pushes and pull requests. Findings or a failed image build fail verification. Lint complements behavioral tests; it does not prove correctness.

These changes add development checks. They do not change API behavior, enable automatic deployment or configure GitHub branch protection.

## Production readiness and scaling roadmap

The submission stays within the assessment's scope. The following extensions describe how to prepare it for a real operating environment; they are **not implemented by this README** and must not be confused with current features.

### First release safeguards

| Extension | Purpose |
| --- | --- |
| JWT through OIDC/JWKS with key rotation | Verify signatures, issuer, audience, expiry and subject using the approved identity provider. Replace the educational UUID contract and confirm authorization policy. |
| Managed migrations with checksums | Track applied versions, serialize migration jobs and reject altered migrations. Review existing installations before baseline adoption; never replay seed data as an upgrade. |
| Separate `/health` and `/ready` | Distinguish a running process from a database/schema ready to serve. Only `/health` currently exists. |
| OpenTelemetry and generated request IDs | Correlate requests and failures; measure latency, errors, user lookup outcomes, database connections and pool waits. Provision dashboards and alerts separately. |
| Configurable pool, lock and admission budgets | Bound waiting and protect reads during write contention. The current database pool has fixed settings; measure before changing them. Limits must account for all replicas. |
| `Retry-After`, `Location`, `Idempotency-Replayed` | Tell clients when to retry, where the created resource lives and whether a result was replayed. These headers are proposed, not part of the current contract. |

Also configure TLS, a secret manager, least-privilege runtime/migration accounts and gateway abuse limits. Rehearse backups, restoration, schema changes and graceful deployment. Define recovery time and acceptable data loss. Agree retention for publication history and receipts before deleting data; removing a receipt changes the retry guarantee.

### Measure real demand

Thirty million registered customers is not a request-rate specification. Establish peak/normal RPS, active users, read/write ratio, catalog size, hot ads, search patterns and p95/p99 latency/error objectives.

Run longer tests at a controlled arrival rate, including several replicas, cold caches, slow queries and dependency outages. Measure query plans, CPU/memory, connection waits and lock duration. The short local test observed increased read latency when writes were slowed; it does not establish sustained production capacity.

Scale stateless API replicas behind a load balancer while keeping quota checks and idempotency decisions on the authoritative PostgreSQL database. Each replica has its own cache and upstream concurrency allowance, so budget the aggregate users-service pressure. Review expensive queries on representative data with `EXPLAIN (ANALYZE, BUFFERS)` and maintained statistics.

If writes demonstrably occupy the shared pool and delay reads, evaluate separate connection budgets. Increasing every replica's pool may overload PostgreSQL. Use read replicas only for explicitly staleness-tolerant traffic; immediate deletion visibility, quota and retry decisions must retain authoritative consistency.

### Conditional infrastructure

- **Redis:** consider shared caching when cross-replica user lookups cause measured pressure. Define TTL, invalidation, memory limits and outage fallback. Names are display data, never authorization. Caching ads also requires a delete/update consistency contract.
- **Search service:** add a dedicated index when realistic text/filter workloads outgrow tuned PostgreSQL. Define indexing lag, deletion propagation and rebuilding procedures.
- **Kafka and transactional outbox:** introduce events when search, analytics or notification consumers need independent processing. Save ad changes and outbox events together, publish asynchronously and monitor backlog/retries. Delivery is at least once; consumers must deduplicate by event ID. Kafka does not implement the current quota or import idempotency.
- **Background imports:** introduce jobs when batches outgrow the synchronous API budget. Expose job status and preserve ownership, operation keys and documented atomicity or partial-progress semantics.

These services add operational cost. Introduce each one for a measured bottleneck or a concrete business requirement rather than adding infrastructure to imply scale. No production identity provider, monitoring platform, backup schedule or large-scale capacity validation is supplied by this assessment.

## Documentation

- [Instructions](INSTRUCTIONS.md): original scope and tasks.
- [Design decisions](DECISIONS.md): transactions, retries, quota, prices and lookup policy.
- [Testing](TESTING.md): reproducible checks and their boundaries.
- [Validation report](validation/REPORT.md): saved results and limitations.
- [OpenAPI](ads/internal/cmd/ads-api/specs/openapi.yaml): the implemented API contract.
