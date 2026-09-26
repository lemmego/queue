package queue

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/lemmego/api/config"
)

// A queue on an in-memory database accepts jobs and loses every one of them on
// restart, silently. The scaffold's SQLite connection carried
// "...?cache=shared&mode=memory" as its url default, and the queue prefers url
// over database while the ORM ignores url entirely — so every generated
// project ran its queue against a database that evaporated, on a different
// database from the rest of the application.
func TestInMemoryDatabaseIsReported(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	cfg := DefaultConfig()
	appConfig := config.M{
		"default": "sqlite",
		"connections": config.M{
			"sqlite": config.M{
				"driver":   "sqlite",
				"url":      "file:./storage/database.sqlite?cache=shared&mode=memory",
				"database": "./storage/database.sqlite",
			},
		},
	}
	if err := cfg.autoFillDatabase(appConfig); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(logged.String(), "will not survive a restart") {
		t.Errorf("an in-memory queue database was not reported:\n%s", logged.String())
	}
}

func TestADurableDatabaseIsNotReported(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	cfg := DefaultConfig()
	appConfig := config.M{
		"default": "sqlite",
		"connections": config.M{
			"sqlite": config.M{
				"driver":   "sqlite",
				"url":      "file:./storage/database.sqlite",
				"database": "./storage/database.sqlite",
			},
		},
	}
	if err := cfg.autoFillDatabase(appConfig); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(logged.String(), "will not survive a restart") {
		t.Errorf("a durable database was reported as ephemeral:\n%s", logged.String())
	}
}

// The other spelling of the same thing.
func TestSharedCacheMemoryDSNIsReported(t *testing.T) {
	var logged bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	warnIfEphemeral("file::memory:?cache=shared")

	if !strings.Contains(logged.String(), "will not survive a restart") {
		t.Errorf(":memory: was not recognised:\n%s", logged.String())
	}
}

// An application usually has one Redis. Before this, the queue read
// tasker.redis_addr, the cache read cache.stores.redis.addr and the session
// read keyvalue.connections.redis — three settings for one server, with three
// different defaults.
func TestSharedRedisConnectionIsUsedWhenTaskerDoesNotSayOtherwise(t *testing.T) {
	shared := config.M{"host": "10.0.0.7", "port": 6380, "password": "hunter2"}

	cfg, err := resolveConfig(nil, config.M{"driver": "redis"}, nil, shared)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RedisAddr != "10.0.0.7:6380" {
		t.Errorf("redis addr = %q, want the shared connection", cfg.RedisAddr)
	}
	if cfg.RedisPass != "hunter2" {
		t.Errorf("redis password = %q, want the shared one", cfg.RedisPass)
	}
}

// A queue that names its own Redis still wins.
func TestTaskerOverridesTheSharedRedisConnection(t *testing.T) {
	shared := config.M{"host": "10.0.0.7", "port": 6380}
	own := config.M{"driver": "redis", "redis_addr": "queue-only:6391"}

	cfg, err := resolveConfig(nil, own, nil, shared)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RedisAddr != "queue-only:6391" {
		t.Errorf("redis addr = %q, want the queue's own", cfg.RedisAddr)
	}
}

func TestAbsentSharedRedisLeavesTheTaskerDefault(t *testing.T) {
	cfg, err := resolveConfig(nil, config.M{"driver": "redis"}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RedisAddr != DefaultConfig().RedisAddr {
		t.Errorf("redis addr = %q, want the default", cfg.RedisAddr)
	}
}
