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
