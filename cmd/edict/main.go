// Command edict runs the trigger engine: it loads declarative rules, connects to
// a MongoDB change stream, and fires the matching rules' actions for every
// change event.
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/prabaltripathiofficial/edict/internal/action"
	"github.com/prabaltripathiofficial/edict/internal/config"
	"github.com/prabaltripathiofficial/edict/internal/engine"
	"github.com/prabaltripathiofficial/edict/internal/rule"
	"github.com/prabaltripathiofficial/edict/internal/source"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)

	// SIGINT/SIGTERM cancel the root context so the change stream and in-flight
	// retries unwind cleanly instead of being killed mid-delivery.
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, config.FromEnv()); err != nil {
		slog.Error("edict exited with error", "err", err)
		os.Exit(1)
	}
	slog.Info("edict shutdown complete")
}

func run(ctx context.Context, cfg config.Config) error {
	// 1. Load and validate rules before touching the database; a bad rule set
	//    should fail before we start consuming changes.
	rules, err := rule.LoadDir(cfg.RulesDir)
	if err != nil {
		return err
	}
	slog.Info("loaded rules", "count", len(rules.All()), "dir", cfg.RulesDir)

	// 2. Build the dispatcher, which precompiles every rule's actions and
	//    surfaces malformed specs (bad webhook URLs, unknown types) up front.
	dispatcher, err := action.NewDispatcher(rules, action.RetryPolicy{
		MaxAttempts: cfg.MaxAttempts,
		BaseDelay:   cfg.BaseDelay,
		MaxDelay:    cfg.MaxDelay,
	}, nil)
	if err != nil {
		return err
	}

	eng := engine.New(rules, dispatcher, slog.Default())

	// 3. Connect the MongoDB source and stream changes into the engine.
	src, err := source.ConnectMongo(ctx, source.MongoConfig{
		URI:            cfg.MongoURI,
		Database:       cfg.Database,
		Collection:     cfg.Collection,
		ConnectTimeout: cfg.ConnectTimeout,
	})
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	slog.Info("edict watching for changes",
		"db", cfg.Database,
		"collection", cfg.Collection,
	)

	return src.Run(ctx, eng.HandleEvent)
}
