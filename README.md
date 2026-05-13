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

## Status

Early design phase. Implementation in progress.

## License

MIT
