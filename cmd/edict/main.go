package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// AppConfig holds runtime configuration. For Day 1 we hardcode; later we can
// move this to YAML+env without forcing broad changes across the program.
type AppConfig struct {
	MongoURI       string
	Database       string
	Collection     string
	ConnectTimeout time.Duration
}

func defaultConfig() AppConfig {
	return AppConfig{
		MongoURI:       "mongodb://localhost:27017/?replicaSet=rs0&directConnection=true",
		Database:       "edict_demo",
		Collection:     "orders",
		ConnectTimeout: 5 * time.Second,
	}
}

func main() {
	// Text logs keep the prototype readable while we inspect behavior manually.
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// Root context is cancelled by SIGINT/SIGTERM so long-lived work can stop
	// cleanly instead of being torn down mid-stream.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cfg := defaultConfig()

	if err := run(ctx, cfg); err != nil {
		slog.Error("edict exited with error", "err", err)
		os.Exit(1)
	}

	slog.Info("edict shutdown complete")
}

func run(ctx context.Context, cfg AppConfig) error {
	slog.Info("starting edict",
		"mongo_uri", cfg.MongoURI,
		"db", cfg.Database,
		"collection", cfg.Collection,
	)

	client, err := connectMongo(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect mongo: %w", err)
	}
	defer func() {
		// Shutdown uses a fresh background context because the root context is
		// usually already cancelled once we get here.
		if err := client.Disconnect(context.Background()); err != nil {
			slog.Error("mongo disconnect failed", "err", err)
		}
	}()
	slog.Info("connected to mongo")

	collection := client.Database(cfg.Database).Collection(cfg.Collection)

	// updateLookup is necessary for update events when downstream logic needs the
	// complete post-update document instead of only changed fields.
	streamOpts := options.ChangeStream().
		SetFullDocument(options.UpdateLookup)

	// An empty pipeline means "watch every event"; we can add server-side filters
	// later once rule routing is part of the prototype.
	stream, err := collection.Watch(ctx, mongo.Pipeline{}, streamOpts)
	if err != nil {
		return fmt.Errorf("open change stream: %w", err)
	}
	defer func() {
		if err := stream.Close(context.Background()); err != nil {
			slog.Error("change stream close failed", "err", err)
		}
	}()

	slog.Info("change stream opened; waiting for events")

	// Next blocks for the next event or until shutdown/error, which makes this
	// the natural control loop for the prototype.
	for stream.Next(ctx) {
		var rawEvent bson.M
		if err := stream.Decode(&rawEvent); err != nil {
			slog.Error("decode event failed", "err", err)
			continue
		}

		handleEvent(rawEvent)
	}

	if ctx.Err() != nil {
		return nil
	}
	if err := stream.Err(); err != nil {
		return fmt.Errorf("change stream error: %w", err)
	}

	return nil
}

func connectMongo(ctx context.Context, cfg AppConfig) (*mongo.Client, error) {
	clientOpts := options.Client().
		ApplyURI(cfg.MongoURI).
		SetServerSelectionTimeout(cfg.ConnectTimeout)

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		return nil, err
	}

	// Connect is lazy, so Ping gives us an actual network round-trip and fails
	// early if Mongo is unavailable or misconfigured.
	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	if err := client.Ping(pingCtx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, err
	}

	return client, nil
}

func handleEvent(event bson.M) {
	// Pretty-printing raw events first keeps the prototype transparent before we
	// introduce typed event models and rule evaluation.
	opType, _ := event["operationType"].(string)

	var dbName any
	var collName any

	switch ns := event["ns"].(type) {
	case bson.M:
		dbName = ns["db"]
		collName = ns["coll"]
	case bson.D:
		dbName = bsonDValue(ns, "db")
		collName = bsonDValue(ns, "coll")
	}

	slog.Info("change event received",
		"operation", opType,
		"db", dbName,
		"collection", collName,
	)

	pretty, err := json.MarshalIndent(event, "", "  ")
	if err != nil {
		slog.Warn("failed to marshal event", "err", err)
		return
	}

	fmt.Println("--- event payload ---")
	fmt.Println(string(pretty))
	fmt.Println("---")
}

func bsonDValue(doc bson.D, key string) any {
	for _, elem := range doc {
		if elem.Key == key {
			return elem.Value
		}
	}

	return nil
}
