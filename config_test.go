package queue

import (
	"net/url"
	"strings"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/lemmego/api/config"
)

func TestResolveConfigUsesDefaults(t *testing.T) {
	cfg, err := resolveConfig(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Driver != "sql" || cfg.DriverName != "sqlite" || cfg.DSN != "./storage/database.sqlite" {
		t.Fatalf("unexpected SQL defaults: %#v", cfg)
	}
	if cfg.RoutePrefix != "/tasker" || cfg.RedisAddr != "localhost:6379" {
		t.Fatalf("unexpected defaults: %#v", cfg)
	}
}

func TestResolveConfigAppliesAppThenExplicitConfig(t *testing.T) {
	taskerConfig := config.M{
		"route_prefix":      "/from-app/",
		"max_open_conns":    40,
		"autoscale":         true,
		"redis_db":          0,
		"workers":           config.M{"mail": 2},
		"requeue_interval":  12,
		"prune_interval":    6,
		"conn_max_lifetime": time.Minute,
	}
	explicit := &Config{RoutePrefix: "/explicit/", MaxOpenConns: 50}
	cfg, err := resolveConfig(explicit, taskerConfig, nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RoutePrefix != "/explicit" || cfg.MaxOpenConns != 50 {
		t.Fatalf("explicit config did not win: %#v", cfg)
	}
	if cfg.EnableAutoscale || cfg.Workers["mail"] != 2 || cfg.RequeueInterval != 12 || cfg.PruneInterval != 6 {
		t.Fatalf("app config was not applied: %#v", cfg)
	}
	if cfg.ConnMaxLifetime != time.Minute {
		t.Fatalf("duration override was not applied: %s", cfg.ConnMaxLifetime)
	}
}

func TestResolveConfigBuildsPostgresDSN(t *testing.T) {
	sqlConfig := config.M{
		"default": "pgsql",
		"connections": config.M{
			"pgsql": config.M{
				"driver": "postgres", "host": "db.internal", "port": 5433,
				"database": "queue db", "user": "queue@user", "password": "p:a/ss", "sslmode": "require",
			},
		},
	}
	cfg, err := resolveConfig(nil, nil, sqlConfig)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(cfg.DSN)
	if err != nil {
		t.Fatal(err)
	}
	password, _ := parsed.User.Password()
	if cfg.DriverName != "postgres" || parsed.Host != "db.internal:5433" || parsed.User.Username() != "queue@user" || password != "p:a/ss" || parsed.Query().Get("sslmode") != "require" {
		t.Fatalf("unexpected PostgreSQL DSN %q", cfg.DSN)
	}
}

func TestResolveConfigBuildsMySQLDSN(t *testing.T) {
	sqlConfig := config.M{
		"default": "mysql",
		"connections": config.M{
			"mysql": config.M{
				"driver": "mysql", "host": "mysql.internal", "port": 3307,
				"database": "queue_db", "user": "queue", "password": "secret",
			},
		},
	}
	cfg, err := resolveConfig(nil, nil, sqlConfig)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mysql.ParseDSN(cfg.DSN)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DriverName != "mysql" || parsed.User != "queue" || parsed.Passwd != "secret" || parsed.Addr != "mysql.internal:3307" || parsed.DBName != "queue_db" || !parsed.ParseTime {
		t.Fatalf("unexpected MySQL config: %#v", cfg)
	}
}

func TestResolveConfigInheritsConnectionPoolWithQueueOverride(t *testing.T) {
	sqlConfig := config.M{
		"default": "sqlite",
		"connections": config.M{
			"sqlite": config.M{
				"driver": "sqlite", "database": "queue.sqlite",
				"max_open_conns": 80, "max_idle_conns": 20, "conn_max_lifetime": 2 * time.Hour,
			},
		},
	}
	cfg, err := resolveConfig(nil, config.M{"max_open_conns": 30}, sqlConfig)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxOpenConns != 30 || cfg.MaxIdleConns != 20 || cfg.ConnMaxLifetime != 2*time.Hour {
		t.Fatalf("unexpected inherited pool config: %#v", cfg)
	}
}

func TestResolveConfigNormalizesSQLAliasesAndSQLiteURL(t *testing.T) {
	sqlConfig := config.M{
		"default": "sqlite",
		"connections": config.M{
			"sqlite": config.M{"driver": "sqlite3", "url": "file:queue.sqlite?cache=shared"},
		},
	}
	cfg, err := resolveConfig(nil, nil, sqlConfig)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DriverName != "sqlite" || cfg.DSN != "file:queue.sqlite?cache=shared" {
		t.Fatalf("unexpected SQLite config: %#v", cfg)
	}
}

func TestResolveConfigValidation(t *testing.T) {
	tests := []struct {
		name   string
		config config.M
		want   string
	}{
		{name: "route", config: config.M{"route_prefix": "jobs"}, want: "absolute URL path"},
		{name: "table prefix", config: config.M{"table_prefix": "jobs;drop"}, want: "table_prefix"},
		{name: "attempts", config: config.M{"max_attempts": 0}, want: "max_attempts"},
		{name: "workers", config: config.M{"workers": config.M{"default": 0}}, want: "worker queues"},
		{name: "worker type", config: config.M{"workers": config.M{"default": "three"}}, want: "workers must map"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveConfig(nil, test.config, nil)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}
