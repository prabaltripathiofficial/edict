package action

import (
	"context"
	"log/slog"

	"github.com/prabaltripathiofficial/edict/internal/event"
	"github.com/prabaltripathiofficial/edict/internal/rule"
)

func init() {
	Register("log", buildLog)
}

// logAction writes a structured log line when a rule fires. It is the simplest
// action and is handy as a no-network default in examples and tests.
type logAction struct {
	level   slog.Level
	message string
}

func buildLog(spec rule.ActionSpec) (Action, error) {
	level := slog.LevelInfo
	switch spec.Level {
	case "", "info":
		level = slog.LevelInfo
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	msg := spec.Message
	if msg == "" {
		msg = "edict rule fired"
	}
	return &logAction{level: level, message: msg}, nil
}

func (a *logAction) Type() string { return "log" }

func (a *logAction) Execute(ctx context.Context, ev event.ChangeEvent, r rule.Rule) error {
	slog.Log(ctx, a.level, a.message,
		"rule", r.ID,
		"operation", ev.Operation,
		"collection", ev.Collection,
		"documentKey", ev.DocumentKey,
	)
	return nil
}
