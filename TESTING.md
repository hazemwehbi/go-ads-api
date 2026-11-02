# Testing

## Run the checks

From the project directory in PowerShell:

```powershell
./scripts/verify.ps1
```

Docker Desktop must be running with Linux containers and Docker Compose. No local Go or C compiler is needed. The command checks formatting, runs go vet, executes unit and integration tests with race detection, and checks known vulnerabilities. It stops on failure and shows each test name.

Tests use a separate PostgreSQL test database. Each integration test creates its own schema and removes it afterwards. Application data is not changed. The test service remains available for another run; stop it with:

```powershell
docker compose --profile test stop test-database
```

For ordinary tests without a live database, use local Go:

```sh
go test -count=1 ./...
```

This command does not run integration tests or enable race detection. On Windows, use the Docker script for the full checks. CI also runs the integration tests with PostgreSQL and race detection.

## What is covered

| Area | Main checks |
| --- | --- |
| Reading ads | The requested ad and owner are returned, including when switching between two different ads. Missing ads return 404. |
| Deletion | Only the owner can delete. The response is an empty 204, later reads return 404, and other ads remain unchanged. Concurrent deletion is checked. |
| Prices | Exact cents storage, euro/cent compatibility, equivalent search filters, and rejection of invalid amounts without consuming quota or creating data. |
| Imports | All-or-nothing writes, concurrent retries, conflicting keys, owner isolation, and replay without duplicates after a committed result is lost in transit. |
| Individual retries | The same key returns the same ad without consuming another quota slot, including when quota is full. |
| Quota | Five publications succeed; further requests fail. Deletion does not refund a slot. Imports remain allowed. Expiry of one event permits one new publication. |
| Usernames | Correct names, null fallback, invalid responses, timeouts, cancellation and recovery. Extra private fields are not forwarded. Wrapped lookup errors preserve their cause; the usecase selects the fallback policy and HTTP maps the application error without importing the adapter. |
| Dependency protection | Cache expiry and renamed users, eviction, shared lookups, concurrency limits, circuit opening and recovery. |
| General behavior | Input validation, malformed search queries, repeated request keys, deadline/cancellation handling across all ad routes, safe diagnostic logs, selected OpenAPI contracts, configuration, transaction failures and graceful shutdown. |

Unit tests use controlled dependencies to exercise success and failure paths. Integration tests use real PostgreSQL and HTTP where needed.

Time-sensitive tests do not wait a day or change the system clock. Cache tests control the clock; quota tests adjust timestamps only in their isolated schema. The lost-response test closes the connection after the real import handler commits, then verifies a retry returns the saved IDs.

Some tests deliberately cause errors. A log mentioning an injected database failure is expected in those cases; the test's PASS or FAIL result determines whether the behavior was correct.

## Parser fuzz checks

The ordinary suite runs the seed cases for JSON and price parsing. For a short fuzz run with local Go:

```sh
go test ./ads/internal/cmd/ads-api/http -run '^$' -fuzz '^FuzzPriceRoundTrip$' -fuzztime=5s -parallel=2
go test ./ads/internal/cmd/ads-api/http -run '^$' -fuzz '^FuzzDecodeBody$' -fuzztime=5s -parallel=2
```

These checks exercise parser inputs and price round trips. A short run does not exhaust all possible inputs.

## Try a user-service outage locally

```text
docker compose up -d --build --wait
curl.exe -s http://localhost:8080/ads/1
docker compose stop users-api
```

An already cached name may remain for one minute. After it expires, repeat the GET: a connection failure should return the ad with username set to null. Repeated failures open the circuit. View reasons with `docker compose logs --tail 30 ads-api`.

Restore the service afterwards:

```text
docker compose start users-api
```

Once it is healthy and the circuit's five-second interval has passed, a successful probe restores the name. An upstream HTTP 503 is treated differently from a connection failure: the supplied client hides its status type, so the API returns 502.

## What passing tests do not prove

Passing tests cover the scenarios exercised, not every possible failure. Race detection checks Go memory races; database tests separately check concurrency rules. Coverage is not a correctness guarantee, and vulnerability checking is not a penetration test. The project has not been load-tested for 30 million customers.

## Suggested production checks

Before a large deployment, define peak request rates, concurrent users, catalog size and acceptable latency/error rates. Then:

- Test realistic reads and writes, traffic spikes, long runs, hot ads and expensive searches.
- Test several application instances, cold caches and deployment changes. Measure database connections, lock waits and dependency load.
- Exercise dependency outages, database failover, uncertain commits and recovery without duplicate writes.
- Review authentication, ownership, abuse controls and exposure of user data.
- Rehearse database migrations, rollback and backup restoration on representative data volumes.
- Verify supported web/mobile clients before removing old price fields or changing response contracts.

These checks are future proposals. The current work stays focused on the assessment requirements and their correctness, without adding production infrastructure solely to make the solution larger.

## Limited local load test

Run `./scripts/load-test.ps1` for the opt-in PostgreSQL/HTTP workload. It uses isolated test schemas and exercises mixed reads/writes, slow user lookups and slow database inserts. It is excluded from ordinary verification. See [the load report](validation/LOAD_TEST.md) for setup, recorded results and limitations. This short closed-loop check is not production capacity validation.
