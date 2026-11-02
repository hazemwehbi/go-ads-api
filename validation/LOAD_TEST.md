# Limited local load check

The four scenarios passed with 12,000 requests in total. No transport errors, unexpected HTTP statuses or invalid response bodies were observed. Each scenario created 100 ads and returned 200 expected quota rejections. No owner exceeded five publications.

## Setup

Run from PowerShell with Docker Desktop running:

```powershell
./scripts/load-test.ps1
```

The opt-in test uses real PostgreSQL, the real ads router/usecases/DAO, and a controlled HTTP users service. Each scenario creates a separate test schema with 10,000 ads and removes it afterwards. Application data is not touched. The test is excluded from ordinary verification unless the `loadtest` build tag is provided.

Each scenario sends 3,000 requests: 80% ad detail, 10% filtered list and 10% creation. Twenty owners submit 15 distinct creation requests each. Five succeed per owner; the rest must return 429. Requests use unique idempotency keys. This run tests quota contention, not repeated-key replay, which is covered by the integration suite.

The healthy scenarios use 20 workers; the degraded scenarios use 40. The user lookup timeout is 250 ms, with the default local cache, circuit breaker and 20-call concurrency limit. Slow users respond after 500 ms unless cancelled. Slow database writes use a test-only 50 ms insert trigger. The application database pool retains its default ten connections.

## Observations

| Scenario | Elapsed seconds | Detail p95 ms | List p95 ms | Create 201 p95 ms | DB pool waits | Null usernames |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| healthy | 0.648 | 8.684 | 11.814 | 31.394 | 2506 | 0 |
| slow_users | 0.685 | 17.895 | 15.896 | 27.656 | 2594 | 2400 |
| slow_database_writes | 0.916 | 44.599 | 43.013 | 98.607 | 2990 | 0 |
| healthy_again | 0.538 | 7.024 | 8.781 | 28.159 | 2643 | 0 |

All scenarios returned 2,700 HTTP 200 responses, 100 HTTP 201 responses and 200 HTTP 429 responses. The final database counts were 10,100 ads, 100 publication events and 100 request receipts in every schema.

Slow user lookups kept detail responses available with nullable usernames. Slow writes increased contention in the shared database pool. Pool wait duration in the JSON is cumulative across requests, not elapsed wall time.

## Interpretation and limits

This is a short closed-loop burst on Docker Desktop: workers send their next request only after the previous one finishes. It does not model a fixed external arrival rate, sustained traffic, large catalogs, multiple application replicas or a production network. The last healthy scenario uses a fresh application and schema; it is a repeat baseline, not proof of recovery of the same circuit breaker. The supplied users service is simulated, and diagnostic logs are discarded during measurement. The router is served by a Go test HTTP server rather than the production listener. Timing runs do not enable race detection.

Reported throughput and percentiles describe these brief local runs only. Differences between the first and last healthy runs include warm tooling/database/OS effects. There is no production latency threshold or customer-capacity claim.

The observed shared-pool contention warrants a longer, rate-controlled run on representative data before changing pool sizes or separating read/write budgets. These results alone do not justify additional infrastructure.

See [raw output](limited-load.txt) and [structured results](limited-load.json) for per-operation p50/p95/p99/max, status counts, upstream calls and database wait totals.
