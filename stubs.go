package queue

const ConfigStub = `package configs

import "github.com/lemmego/api/config"

func init() {
	config.Set("tasker", config.M{
		"route_prefix":      config.MustEnv("TASKER_ROUTE_PREFIX", "/tasker"),
		"table_prefix":      config.MustEnv("TASKER_TABLE_PREFIX", "tasker_"),
		"queue":             config.MustEnv("TASKER_QUEUE", "default"),
		"max_attempts":      config.MustEnv("TASKER_MAX_ATTEMPTS", 3),
		"heartbeat_interval": config.MustEnv("TASKER_HEARTBEAT_SEC", 5),
		"requeue_timeout":    config.MustEnv("TASKER_REQUEUE_SEC", 60),
		"prune_after_hours":  config.MustEnv("TASKER_PRUNE_HOURS", 168),
		"autoscale":          config.MustEnv("TASKER_AUTOSCALE", false),
	})
}
`
