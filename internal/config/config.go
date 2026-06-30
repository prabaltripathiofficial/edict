// Package config loads Edict's runtime configuration from environment
// variables, falling back to development-friendly defaults so the prototype
// runs with zero setup against a local replica set.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config is the resolved runtime configuration.
type Config struct {
	MongoURI       string
	Database       string
	Collection     string
	RulesDir       string
	ConnectTimeout time.Duration

	// Retry tuning for action dispatch.
	MaxAttempts int
	BaseDelay   time.Duration
	MaxDelay    time.Duration
}

// FromEnv builds a Config from EDICT_* environment variables with defaults.
func FromEnv() Config {
	return Config{
		MongoURI:       env("EDICT_MONGO_URI", "mongodb://localhost:27017/?replicaSet=rs0&directConnection=true"),
		Database:       env("EDICT_DB", "edict_demo"),
		Collection:     env("EDICT_COLLECTION", "orders"),
		RulesDir:       env("EDICT_RULES_DIR", "rules"),
		ConnectTimeout: envDuration("EDICT_CONNECT_TIMEOUT", 5*time.Second),
		MaxAttempts:    envInt("EDICT_MAX_ATTEMPTS", 3),
		BaseDelay:      envDuration("EDICT_RETRY_BASE_DELAY", 200*time.Millisecond),
		MaxDelay:       envDuration("EDICT_RETRY_MAX_DELAY", 5*time.Second),
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
