# Edict — Build Log

## Day 1 — 2026-05-22

### What I built
- MongoDB single-node replica set using Docker Compose
- A Go consumer using the MongoDB Go driver v2
- A Change Stream listener with `fullDocument=updateLookup`
- Graceful shutdown using `signal.NotifyContext` and context cancellation

### Concepts I now understand

**1. Why Change Streams require a replica set:**
MongoDB Change Streams depend on the oplog, and the oplog exists because replica sets need a durable ordered log to keep secondaries in sync. Since Change Streams are built on top of that same change history, standalone MongoDB cannot support them in the same way.

**2. Resume tokens:**
A resume token is the `_id` field attached to each change event. It acts like a bookmark so a consumer can reconnect and continue from the last processed event. It only works while the relevant history is still present in the oplog.

**3. The `fullDocument=updateLookup` option:**
By default, update events only include what changed in `updateDescription`. If I want the full latest version of the document after an update, I need `updateLookup`, which makes Mongo fetch the full updated document.

**4. Why we use `signal.NotifyContext` instead of `os/signal` directly:**
`signal.NotifyContext` converts OS shutdown signals like `SIGINT` and `SIGTERM` into context cancellation. That makes it easier to pass shutdown behavior through the whole program, including the Mongo watch loop.

### Surprises / "huh" moments
- The raw oplog is much lower-level than Change Stream events.
- Update events do not include `fullDocument` unless `updateLookup` is enabled.
- The namespace logging bug happened because nested BSON was not always decoded as `bson.M`.

### Open questions for later
- How exactly does Mongo decide when a resume token becomes unusable?
- What is the performance cost of `updateLookup`?
- What happens to Change Streams during a network partition?

### Tomorrow (Day 2)
- Rule schema design in JSON
- Loader from `rules/` directory
- Validation with `go-playground/validator`
- Hot reload via `fsnotify`

## Day 2 — MVP

### What I built
- A normalized `event.ChangeEvent` so the engine never touches BSON. The Mongo
  adapter now flattens `bson.M`/`bson.D`/`bson.A` into plain Go maps before the
  event reaches a rule.
- A declarative JSON rule schema: conditions (`eq/ne/gt/gte/lt/lte/in/contains/
  exists`, grouped by `all`/`any`), actions, and a `then` chain.
- A directory loader with strict decoding (`DisallowUnknownFields`) and a
  validator that enforces unique IDs, resolvable `then` references, and a
  bounded chain (≤ 50, which also catches cycles).
- The evaluation engine: match → fire actions → walk the chain, with a per-event
  `fired` set so diamonds in the rule graph fire each rule once.
- A pluggable action layer (`log`, `webhook`) behind a registry, plus a
  dispatcher with at-least-once exponential-backoff retry and an audit record
  for every attempt.
- Wired it all into `cmd/edict` with env-driven config and shipped example
  rules under `rules/`.

### Decisions I made
- **In-code validation instead of `go-playground/validator`.** The rules have
  cross-field and cross-rule invariants (chain bounds, `then` resolution, op-
  specific value types) that struct tags can't express cleanly, so a hand-
  written validator was simpler and gave better error messages. Fewer deps too.
- **Dispatch behind an interface.** The engine depends on a `Dispatcher`
  interface, not the action package. That kept the matching logic free of
  HTTP/retry concerns and made the engine trivially testable with a fake.
- **Numeric coercion in the matcher.** JSON numbers decode as `float64` but
  Mongo hands back `int32`/`int64`; comparisons coerce both to `float64` so a
  rule written with `1000` matches a stored `int64(1000)`.
- **Deferred hot reload (`fsnotify`).** Not needed for the MVP — rules load once
  at startup. Reloading safely mid-stream needs an atomic swap of the rule set,
  which is a clean follow-up rather than core.

### Surprises / "huh" moments
- Go's `internal/` rule really does block out-of-module imports — a throwaway
  verification program outside the module wouldn't compile, so the example-rule
  smoke test had to live inside the module as a real test (which is better
  anyway — the shipped examples can't rot now).

### Open questions for later
- Hot reload: watch `rules/` and swap the `*rule.Set` atomically.
- A `queue` action (Kafka/NATS) and a `db-write` action to close the loop on
  "downstream DB writes" from the README.
- Per-rule metrics + dead-letter handling once retries are exhausted.
