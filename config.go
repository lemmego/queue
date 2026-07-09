package queue

import (
	"fmt"

	"github.com/lemmego/api/config"
)

type Config struct {
	Driver             string
	DSN                string
	DriverName         string
	TablePrefix        string
	RoutePrefix        string
	MaxOpenConns       int
	MaxIdleConns       int

	RedisAddr          string
	RedisPass          string
	RedisDB            int
	RedisPrefix        string

	DefaultQueue       string
	DefaultMaxAttempts int

	Workers            map[string]int
	EnableAutoscale    bool

	HeartbeatInterval  int
	RequeueTimeout     int
	PruneAfterHours    int
}

func DefaultConfig() *Config {
	return &Config{
		DefaultQueue:       "default",
		DefaultMaxAttempts: 3,
		TablePrefix:        "tasker_",
		RoutePrefix:        "/tasker",
		MaxOpenConns:       25,
		MaxIdleConns:       10,
		HeartbeatInterval:  5,
		RequeueTimeout:     60,
		PruneAfterHours:    168,
		Workers:            map[string]int{"default": 3},
	}
}

// ApplyOverrides reads from a config.M (the "tasker" section) and
// overrides matching fields on the Config. Keys are flat (no "tasker." prefix).
func (cfg *Config) ApplyOverrides(c config.M) {
	if c == nil {
		return
	}
	if v := c.String("driver", ""); v != "" {
		cfg.Driver = v
	}
	if v := c.String("dsn", ""); v != "" {
		cfg.DSN = v
	}
	if v := c.String("driver_name", ""); v != "" {
		cfg.DriverName = v
	}
	if v := c.String("table_prefix", ""); v != "" {
		cfg.TablePrefix = v
	}
	if v := c.String("route_prefix", ""); v != "" {
		cfg.RoutePrefix = v
	}
	if v := c.String("redis_addr", ""); v != "" {
		cfg.RedisAddr = v
	}
	if v := c.String("redis_password", ""); v != "" {
		cfg.RedisPass = v
	}
	if v := c.Int("redis_db", 0); v > 0 {
		cfg.RedisDB = v
	}
	if v := c.String("queue", ""); v != "" {
		cfg.DefaultQueue = v
	}
	if v := c.Int("max_attempts", 0); v > 0 {
		cfg.DefaultMaxAttempts = v
	}
	if v := c.Int("heartbeat_interval", 0); v > 0 {
		cfg.HeartbeatInterval = v
	}
	if v := c.Int("requeue_timeout", 0); v > 0 {
		cfg.RequeueTimeout = v
	}
	if v := c.Int("prune_after_hours", 0); v > 0 {
		cfg.PruneAfterHours = v
	}
	if c.Bool("autoscale", false) {
		cfg.EnableAutoscale = true
	}
}

// AutoFillDatabase reads the app's default SQL connection config and
// fills DSN/DriverName. Only applies when Driver is "sql" and DSN is empty.
func (cfg *Config) AutoFillDatabase(c config.M) {
	if cfg.Driver == "redis" || cfg.DSN != "" {
		return
	}

	connName := c.String("default", "sqlite")
	connRaw := config.Get(fmt.Sprintf("sql.connections.%s", connName))
	conn, ok := connRaw.(config.M)
	if !ok {
		return
	}

	cfg.Driver = "sql"
	cfg.DriverName = conn.String("driver", "sqlite")
	if cfg.DriverName == "sqlite" || cfg.DriverName == "sqlite3" {
		cfg.DSN = conn.String("database", "./storage/database.sqlite")
	} else {
		host := conn.String("host", "localhost")
		port := conn.Int("port", 5432)
		user := conn.String("user", "")
		pass := conn.String("password", "")
		db := conn.String("database", "")
		cfg.DSN = fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable", user, pass, host, port, db)
	}
}
