# Edict

> A database-agnostic trigger framework. Declare reactive rules on any database — without depending on what the database engine natively supports.

## What it is

Edict lets you declaratively define **triggers on data and conditions** across any backing store (PostgreSQL, MongoDB, MySQL, and more). Each condition can chain **up to 50 triggers**, with pluggable actions: webhooks, message-queue dispatch, function invocation, or downstream DB writes.

Modern databases each have their own trigger story — PostgreSQL has `CREATE TRIGGER`, MongoDB has change streams, MySQL has stored procedures. None of them compose well across stacks, and none of them give you conditional chaining out of the box. Edict closes that gap.

## Why it matters

- **DB-agnostic** — same trigger DSL works regardless of the underlying database.
- **Conditional chaining** — up to 50 triggers per condition, evaluated as a directed graph.
- **High-performance evaluation** — backed by a custom in-memory rule store (Redis-style data structures) for sub-millisecond condition checks.
- **Pluggable actions** — webhook, queue, serverless function, or arbitrary side-effect, with at-least-once delivery and exponential-backoff retries.
- **Audit-first** — every trigger fire is recorded with full input/output context.

## Quick start

Edict watches a MongoDB collection's change stream and fires declarative rules.

```bash
# 1. Bring up a single-node replica set (Change Streams need the oplog).
docker compose up -d
docker exec edict-mongo mongosh --quiet --eval 'rs.initiate()'

# 2. Run the engine. It loads every *.json rule in ./rules and watches the
#    configured collection.
go run ./cmd/edict

# 3. In another shell, write to the watched collection and watch rules fire.
docker exec edict-mongo mongosh edict_demo --quiet --eval \
  'db.orders.insertOne({status:"paid", amount:1500})'
```

Configuration is environment-driven (all optional):

| Variable | Default | Meaning |
| --- | --- | --- |
| `EDICT_MONGO_URI` | `mongodb://localhost:27017/?replicaSet=rs0&directConnection=true` | Mongo connection string |
| `EDICT_DB` | `edict_demo` | Database to watch |
| `EDICT_COLLECTION` | `orders` | Collection to watch |
| `EDICT_RULES_DIR` | `rules` | Directory of `*.json` rule files |
| `EDICT_MAX_ATTEMPTS` | `3` | Action delivery attempts |
| `EDICT_RETRY_BASE_DELAY` | `200ms` | Backoff base |

## Rule format

Each rule is one JSON file. A rule binds conditions on a collection's change
events to ordered actions, and may chain to other rules via `then` (bounded at
50 to reject runaway graphs and cycles).

```json
{
  "id": "high-value-order",
  "collection": "orders",
  "on": ["insert", "update", "replace"],
  "when": {
    "all": [
      { "field": "status", "op": "eq", "value": "paid" },
      { "field": "amount", "op": "gte", "value": 1000 }
    ]
  },
  "actions": [
    { "type": "log", "level": "info", "message": "high-value paid order" }
  ],
  "then": ["notify-ops-webhook"]
}
```

- **Conditions** — `field` is a dot path into the document; `op` is one of
  `eq`/`ne`/`gt`/`gte`/`lt`/`lte`/`in`/`contains`/`exists`. `all` must all hold;
  `any` needs one. Omit `when` to match every event for the collection.
- **Actions** — `log` (structured line) and `webhook` (JSON POST) ship today;
  new types register one builder in `internal/action`.
- **Delivery** — actions run with at-least-once semantics and exponential
  backoff; every attempt is written to an audit trail.

See `rules/` for working examples.

## Architecture

```
MongoDB change stream → source adapter → normalized ChangeEvent
        → engine (match conditions, walk then-chain)
        → dispatcher (retry + audit) → actions (log / webhook / …)
```

The engine is decoupled from both the database and the action implementations,
so adding a new source (Postgres logical replication, MySQL binlog) or a new
action is a localized change.

## Status

MVP complete: rule loading + validation, condition matching with conditional
chaining, pluggable retrying actions with an audit trail, and a live MongoDB
source. See [NOTES.md](NOTES.md) for the build log.

## License

MIT
