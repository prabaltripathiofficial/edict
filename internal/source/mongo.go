// Package source contains adapters that connect a concrete database's change
// feed to Edict's normalized event stream. The MongoDB adapter wraps a change
// stream and converts each raw event into an event.ChangeEvent.
package source

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/prabaltripathiofficial/edict/internal/event"
)

// Handler receives each normalized change event. Returning an error stops the
// stream; in practice the engine only errors on context cancellation.
type Handler func(ctx context.Context, ev event.ChangeEvent) error

// MongoConfig configures the MongoDB change-stream source.
type MongoConfig struct {
	URI            string
	Database       string
	Collection     string
	ConnectTimeout time.Duration
}

// Mongo is a change-stream source over a single collection.
type Mongo struct {
	cfg    MongoConfig
	client *mongo.Client
}

// ConnectMongo dials MongoDB and verifies the connection with a ping, so
// misconfiguration fails fast rather than on the first event.
func ConnectMongo(ctx context.Context, cfg MongoConfig) (*Mongo, error) {
	opts := options.Client().
		ApplyURI(cfg.URI).
		SetServerSelectionTimeout(cfg.ConnectTimeout)

	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("connect mongo: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()
	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("ping mongo: %w", err)
	}

	return &Mongo{cfg: cfg, client: client}, nil
}

// Close disconnects the underlying client.
func (m *Mongo) Close() error {
	return m.client.Disconnect(context.Background())
}

// Run opens the change stream and feeds every event to handler until the
// context is cancelled or the stream errors. updateLookup is enabled so update
// events carry the full post-change document for rule evaluation.
func (m *Mongo) Run(ctx context.Context, handler Handler) error {
	coll := m.client.Database(m.cfg.Database).Collection(m.cfg.Collection)

	streamOpts := options.ChangeStream().SetFullDocument(options.UpdateLookup)
	stream, err := coll.Watch(ctx, mongo.Pipeline{}, streamOpts)
	if err != nil {
		return fmt.Errorf("open change stream: %w", err)
	}
	defer func() { _ = stream.Close(context.Background()) }()

	for stream.Next(ctx) {
		var raw bson.M
		if err := stream.Decode(&raw); err != nil {
			// A single undecodable event should not tear down the whole stream.
			continue
		}
		ev := normalize(raw)
		if err := handler(ctx, ev); err != nil {
			return err
		}
	}

	if ctx.Err() != nil {
		return nil
	}
	return stream.Err()
}

// normalize converts a raw MongoDB change event into an event.ChangeEvent,
// flattening BSON document types into plain map[string]any so the rule engine
// never sees driver-specific types.
func normalize(raw bson.M) event.ChangeEvent {
	ev := event.ChangeEvent{
		Operation: event.Operation(asString(raw["operationType"])),
		Timestamp: time.Now(),
	}

	if ns, ok := toMap(raw["ns"]); ok {
		ev.Database = asString(ns["db"])
		ev.Collection = asString(ns["coll"])
	}
	if dk, ok := toMap(raw["documentKey"]); ok {
		ev.DocumentKey = dk
	}
	if fd, ok := toMap(raw["fullDocument"]); ok {
		ev.FullDocument = fd
	}
	if ud, ok := toMap(raw["updateDescription"]); ok {
		if uf, ok := toMap(ud["updatedFields"]); ok {
			ev.UpdatedFields = uf
		}
	}

	return ev
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

// toMap normalizes the BSON document representations the driver may hand back
// (bson.M, bson.D) into a map[string]any, recursing into nested values.
func toMap(v any) (map[string]any, bool) {
	switch d := v.(type) {
	case bson.M:
		out := make(map[string]any, len(d))
		for k, val := range d {
			out[k] = normalizeValue(val)
		}
		return out, true
	case bson.D:
		out := make(map[string]any, len(d))
		for _, e := range d {
			out[e.Key] = normalizeValue(e.Value)
		}
		return out, true
	case map[string]any:
		return d, true
	default:
		return nil, false
	}
}

// normalizeValue recursively flattens nested BSON documents and arrays.
func normalizeValue(v any) any {
	switch val := v.(type) {
	case bson.M, bson.D:
		m, _ := toMap(val)
		return m
	case bson.A:
		out := make([]any, len(val))
		for i, item := range val {
			out[i] = normalizeValue(item)
		}
		return out
	default:
		return val
	}
}
