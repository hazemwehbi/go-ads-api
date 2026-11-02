# Verification report

The recorded checks passed. They cover the six assessment requirements, including retries, concurrent requests and failures of the user service. Supporting changes are limited to protecting these flows and making them testable, without turning the assessment into a larger platform.

## What was checked

- **Read an ad:** the requested ID selects the correct record. Missing ads return 404. User details do not expose private fields.
- **Delete an ad:** only its owner can delete it. Other ads stay unchanged. Repeated deletion returns 404, and deletion does not restore the publication quota.
- **Username:** a successful lookup returns the name. Temporary connection failures return the ad with `username: null`. Tests cover timeouts, cancellation, cache expiry, concurrent lookups, the circuit breaker and recovery. Lookup error classification, the usecase fallback policy and HTTP error mapping are tested separately; the API contract is unchanged.
- **Prices:** `price` accepts amounts such as `120.50`, while storage remains in integer cents. Tests cover compatibility with `price_cents`, invalid amounts and filters.
- **Bulk import:** the batch succeeds or rolls back as a whole. Retrying with the same key returns the original IDs, including after a lost response. Reusing the key with different content returns 409.
- **Publication quota:** a user can publish five ads per rolling 24 hours through `POST /ads`. Concurrent requests cannot bypass the limit. Imports are excluded, and expired publications free a slot.

Single-ad creation also supports an optional idempotency key. Without a key, a retry can create another ad.

- **Request handling:** malformed search queries and repeated idempotency headers return 400. All ad routes handle deadlines and cancellation consistently. Rejected imports leave no ads or receipts. User lookup logs retain diagnostic causes without request URLs.

## Saved results

The submission checkout was checked on 2026-11-02 with Go 1.26.10 in Linux containers and a dedicated PostgreSQL 16 database.

- [Complete verification log](pre-submission-tests.txt): formatting, default and integration/load-tagged `go vet`, unit and PostgreSQL/HTTP integration tests, optional load scenarios and race detection passed. The verbose run recorded 378 passed functions/subtests, with no failures or skips.
- Both parser fuzz targets passed ten-second runs. Price conversion exercised 72,295 inputs and JSON parsing 11,295 inputs. These bounded runs are supplementary checks, not exhaustive proofs.
- Four load scenarios completed 12,000 mixed requests with zero unexpected responses. Quota rejections were expected and checked separately. The workload ran with race detection during this verification; its timings are not capacity benchmarks. See [the workload description](LOAD_TEST.md) for the setup and limitations of the earlier timing run.
- Known-vulnerability scanning reported no vulnerabilities at scan time.
- [Docker build log](pre-submission-build.txt): the final ads API image built successfully. Compose configuration validation also passed.

All seven files under `/users` match the original template. Verification mounted the checkout read-only, used an isolated disposable database/network and removed those resources afterwards. Running application containers and application data were not changed. Only verification evidence and this report were updated.

## Limits and next steps

These checks do not establish capacity for 30 million customers. Before a production rollout, measure realistic traffic, database size and latency, then decide whether shared caching or other infrastructure is needed. Real authentication and operational monitoring are also required.

Cached names may remain unchanged until expiry. Some upstream HTTP errors cannot be classified by the supplied client and return 502. See [DECISIONS.md](../DECISIONS.md) for these choices and possible extensions.

To check your current checkout, run this from the project root:

```powershell
./scripts/verify.ps1
```

See [TESTING.md](../TESTING.md) for setup and test details. The logs above record these runs; rerun verification after further code changes.
