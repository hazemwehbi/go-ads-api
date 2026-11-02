# Design decisions

## Scope

This solution focuses on the six assessment tasks. It keeps the original HTTP, usecase and DAO structure, with PostgreSQL for storage. The users service is unchanged.

Additional work supports those tasks: safe retries, validation, dependency protection and tests. The aim is a small, understandable service, without unnecessary infrastructure or over-engineering.

## Ads and ownership

Reading an ad uses the requested ID. A missing ad returns 404 without calling the users service.

Deletion checks the ad ID and owner in one SQL statement. Success returns 204 with no body. Missing, already-deleted and non-owned ads return 404. Deleting an ad does not restore publication quota.

## Prices

The API exposes price in euros as a JSON number. Storage and calculations use integer cents to avoid floating-point rounding. Fractional cents are rejected.

Requests accept either price or price_cents, never both. Responses retain both fields while consumers migrate. Removing the old fields requires agreement with web/mobile teams and a versioned release. Request logs alone cannot tell us which response field a client reads. Currency formatting belongs to the interface.

## Retries and imports

Bulk import requires an Idempotency-Key. All ads and the saved result are committed together, or nothing is saved. Repeating the same owner, key and ordered content returns the original IDs. Different content with the same key returns 409.

Individual creation supports the same mechanism with an optional key, separate from import keys. A replay does not consume another quota slot. Without a key, retrying an individual creation can create another ad.

Clients must save the key before sending a request and reuse it after a timeout or lost response. A new key means a new operation. Saved results remain after ad deletion and do not recreate deleted ads. They currently have no expiry; adding retention must preserve an agreed retry contract.

## Publication quota

A user can publish five ads through POST /ads in a rolling 24-hour window. Imports do not count. Failed writes do not consume quota.

A PostgreSQL transaction locks publication checks for the same user, counts recent events, then saves the ad and its publication event. This protects the limit across application instances. Time is checked using the database clock after obtaining the lock. An event exactly 24 hours old no longer counts. All publication writers must follow this rule.

## Usernames

The `userlookup` package defines the shared lookup errors. The client classifies dependency failures, the usecase decides whether to omit the username, and HTTP maps the application error to 502. HTTP and usecases do not depend on the concrete `userclient` package. This separation does not change API responses.

The supplied client fetches the owner's display name. A name is not proof of identity or account status.

- A valid response returns the name.
- A missing user, connection failure, lookup timeout, open circuit or full lookup capacity returns the ad with username set to null.
- Invalid user data or an opaque upstream HTTP error returns 502. The supplied client does not expose typed non-404 statuses, so an upstream 503 also follows this rule.
- Cancelling the request stops its work. Expiry of the overall request deadline returns 504.

The local cache holds up to 1000 users for one minute. It never serves expired entries. Concurrent requests for the same user share a lookup, and each instance allows at most 20 active user-service calls. Five consecutive network failures or timeouts open the circuit for five seconds, followed by one recovery probe. Settings are configurable; these defaults are starting values, not production sizing.

Cached names can be out of date until expiry and must not control permissions. List endpoints do not fetch names. Clients must support nullable usernames. Configuration is described in [README.md](README.md#user-lookup-resilience).

## Request failures

Malformed query encoding, invalid UTF-8 and NUL are rejected before querying PostgreSQL. Creation and import accept only one Idempotency-Key header value; the key remains optional for creation.

All ad endpoints use the same deadline and cancellation handling. An expired deadline returns 504, while cancellation stops response handling. A timeout does not prove that a write rolled back; clients must retain their retry key. User lookup logs include the diagnostic cause, with HTTP request URLs omitted to avoid logging credentials or query tokens.

## Database changes

Compose applies SQL migrations and seed data only when creating an empty database. It does not upgrade existing databases automatically.

For an existing database, verify backups and applied versions, then apply missing migrations once in order. Do not replay seed data or delete volumes to upgrade. Migration 006 requires stopping incompatible writers before applying it and deploying the matching application. Backfill quota records only from confirmed individual-publication events.

## Possible production extensions

The project is not validated for 30 million customers. Customer count alone does not describe peak traffic or database load. Before expanding it, measure request rates, data size, latency and dependency pressure.

Depending on those results, consider:

- Verified gateway authentication, TLS, secrets management and abuse limits. The sample UUID header is not secure authentication(Ex. JWT etc ...).
- Readiness checks, metrics, tracing, alerts and recovery procedures.
- Query tuning, replica-wide connection budgets and read replicas where consistency permits. Quota and retry decisions must stay on the authoritative database.
- Shared caching or a search service only when measured demand justifies them, with clear freshness rules.
- Background import jobs only if batches outgrow the synchronous API, with explicit job status and retry handling.
- Managed migrations, backup restoration tests and agreed data-retention policies.

These are proposals, not features implemented in this assessment. See [TESTING.md](TESTING.md) for current coverage and suggested production checks.
