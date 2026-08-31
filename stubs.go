package queue

const ConfigStub = `package configs

import (
	"time"

	"github.com/lemmego/api/config"
)

func init() {
	config.Set("tasker", config.M{
		"driver":           config.MustEnv("TASKER_DRIVER", "sql"),
		"dsn":              config.MustEnv("TASKER_DSN", ""),
		"driver_name":      config.MustEnv("TASKER_SQL_DRIVER", ""),
		"route_prefix":      config.MustEnv("TASKER_ROUTE_PREFIX", "/tasker"),
		"table_prefix":      config.MustEnv("TASKER_TABLE_PREFIX", "tasker_"),
		"max_open_conns":    config.MustEnv("TASKER_MAX_OPEN_CONNS", 25),
		"max_idle_conns":    config.MustEnv("TASKER_MAX_IDLE_CONNS", 10),
		"conn_max_lifetime": time.Duration(config.MustEnv("TASKER_CONN_MAX_LIFETIME_SEC", 0)) * time.Second,
		"redis_addr":        config.MustEnv("TASKER_REDIS_ADDR", "localhost:6379"),
		"redis_password":    config.MustEnv("TASKER_REDIS_PASSWORD", ""),
		"redis_db":          config.MustEnv("TASKER_REDIS_DB", 0),
		"redis_pool_size":   config.MustEnv("TASKER_REDIS_POOL_SIZE", 0),
		"redis_prefix":      config.MustEnv("TASKER_REDIS_PREFIX", "tasker:"),
		"queue":             config.MustEnv("TASKER_QUEUE", "default"),
		"max_attempts":      config.MustEnv("TASKER_MAX_ATTEMPTS", 3),
		"workers": config.M{
			"default": config.MustEnv("TASKER_WORKERS", 3),
		},
		"heartbeat_interval": config.MustEnv("TASKER_HEARTBEAT_SEC", 5),
		"requeue_interval":   config.MustEnv("TASKER_REQUEUE_INTERVAL_SEC", 30),
		"requeue_timeout":    config.MustEnv("TASKER_REQUEUE_SEC", 60),
		"prune_interval":     config.MustEnv("TASKER_PRUNE_INTERVAL_HOURS", 24),
		"prune_after_hours":  config.MustEnv("TASKER_PRUNE_HOURS", 168),
		"autoscale":          config.MustEnv("TASKER_AUTOSCALE", false),
	})
}
`
