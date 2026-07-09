package queue

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	"github.com/lemmego/tasker"
	"github.com/lemmego/tasker/cmd"
	"github.com/lemmego/tasker/driver/redisdriver"
	"github.com/lemmego/tasker/driver/sqldriver"
	"github.com/lemmego/tasker/supervisor"
	"github.com/lemmego/tasker/web"
)

type Provider struct {
	Config *Config
}

func (p *Provider) Provide(a app.App) error {
	cfg := DefaultConfig()
	if p.Config != nil {
		cfg = p.Config
	}

	taskerCfg, _ := a.Config().Get("tasker").(config.M)
	cfg.ApplyOverrides(taskerCfg)

	appCfg, _ := a.Config().Get("sql").(config.M)
	cfg.AutoFillDatabase(appCfg)

	if cfg.RoutePrefix == "" {
		cfg.RoutePrefix = "/tasker"
	}

	d, err := createDriver(cfg)
	if err != nil {
		return fmt.Errorf("tasker: failed to create driver: %w", err)
	}

	if sd, ok := d.(interface{ Migrate(context.Context) error }); ok {
		if err := sd.Migrate(context.Background()); err != nil {
			return fmt.Errorf("tasker: failed to migrate: %w", err)
		}
	}

	mgr := tasker.NewConfiguredManager(tasker.Config{
		Driver:             d,
		DefaultQueue:       tasker.QueueName(cfg.DefaultQueue),
		DefaultMaxAttempts: cfg.DefaultMaxAttempts,
	})
	tasker.SetGlobal(mgr)

	if !a.RunningInConsole() {
		supCfg := supervisor.DefaultConfig()
		supCfg.Queues = make(map[tasker.QueueName]supervisor.QueueConfig)
		for name, count := range cfg.Workers {
			supCfg.Queues[tasker.QueueName(name)] = supervisor.QueueConfig{
				MaxWorkers: count,
				MinWorkers: 1,
			}
		}
		supCfg.EnableAutoscale = cfg.EnableAutoscale
		if cfg.HeartbeatInterval > 0 {
			supCfg.HeartbeatInterval = time.Duration(cfg.HeartbeatInterval) * time.Second
		}
		if cfg.RequeueTimeout > 0 {
			supCfg.RequeueTimeout = time.Duration(cfg.RequeueTimeout) * time.Second
		}
		if cfg.PruneAfterHours > 0 {
			supCfg.PruneAfter = time.Duration(cfg.PruneAfterHours) * time.Hour
		}

		sup := supervisor.New(mgr, supCfg)
		a.AddService(sup)

		srv := web.NewWithPrefix(mgr, sup, cfg.RoutePrefix)
		a.AddService(srv)
	}

	return nil
}

func (p *Provider) AddCommands() []app.Command {
	return []app.Command{
		func(a app.App) *cobra.Command {
			return cmd.WorkCommand(a)
		},
	}
}

func (p *Provider) AddRoutes() app.RouteCallback {
	return func(a app.App) {
		srv := app.Get[*web.Server](a)
		if srv == nil {
			return
		}
		prefix := "/tasker"
		if p.Config != nil && p.Config.RoutePrefix != "" {
			prefix = p.Config.RoutePrefix
		}
		a.Router().Handle(prefix+"/", srv)
	}
}

func (p *Provider) AddPublishables() []*app.Publishable {
	return []*app.Publishable{
		{
			FilePath: "internal/configs/tasker.go",
			Content:  []byte(ConfigStub),
			Tag:      "config",
		},
	}
}

func (p *Provider) Shutdown(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	_ = ctx
	return nil
}

func createDriver(cfg *Config) (tasker.Driver, error) {
	switch cfg.Driver {
	case "sql", "postgres", "mysql", "sqlite", "sqlite3":
		return sqldriver.NewDriver(sqldriver.Config{
			DSN:          cfg.DSN,
			DriverName:   cfg.DriverName,
			MaxOpenConns: cfg.MaxOpenConns,
			MaxIdleConns: cfg.MaxIdleConns,
			TablePrefix:  cfg.TablePrefix,
		})
	case "redis":
		return redisdriver.NewDriver(redisdriver.Config{
			Addr:      cfg.RedisAddr,
			Password:  cfg.RedisPass,
			DB:        cfg.RedisDB,
			KeyPrefix: cfg.RedisPrefix,
		})
	default:
		return nil, fmt.Errorf("unsupported tasker driver: %s", cfg.Driver)
	}
}
