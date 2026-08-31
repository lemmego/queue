package queue

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
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

	mu           sync.RWMutex
	resolved     *Config
	manager      *tasker.Manager
	supervisor   *supervisor.Supervisor
	driver       tasker.Driver
	shutdownOnce sync.Once
	shutdownErr  error
}

func (p *Provider) Provide(a app.App) error {
	taskerCfg, _ := a.Config().Get("tasker").(config.M)
	appCfg, _ := a.Config().Get("sql").(config.M)
	cfg, err := resolveConfig(p.Config, taskerCfg, appCfg)
	if err != nil {
		return err
	}

	d, err := createDriver(cfg)
	if err != nil {
		return fmt.Errorf("tasker: failed to create driver: %w", err)
	}

	if sd, ok := d.(interface{ Migrate(context.Context) error }); ok {
		if err := sd.Migrate(context.Background()); err != nil {
			_ = d.Close()
			return fmt.Errorf("tasker: failed to migrate: %w", err)
		}
	}

	mgr := tasker.NewConfiguredManager(tasker.Config{
		Driver:             d,
		DefaultQueue:       tasker.QueueName(cfg.DefaultQueue),
		DefaultMaxAttempts: cfg.DefaultMaxAttempts,
	})
	tasker.SetGlobal(mgr)
	a.AddService(cfg)
	a.AddService(mgr)

	p.mu.Lock()
	p.resolved = cfg
	p.manager = mgr
	p.driver = d
	p.mu.Unlock()

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
		if cfg.RequeueInterval > 0 {
			supCfg.RequeueInterval = time.Duration(cfg.RequeueInterval) * time.Second
		}
		if cfg.PruneAfterHours > 0 {
			supCfg.PruneAfter = time.Duration(cfg.PruneAfterHours) * time.Hour
		}
		if cfg.PruneInterval > 0 {
			supCfg.PruneInterval = time.Duration(cfg.PruneInterval) * time.Hour
		}

		sup := supervisor.New(mgr, supCfg)
		a.AddService(sup)

		srv := newWebServer(mgr, sup, cfg.RoutePrefix)
		a.AddService(srv)
		p.mu.Lock()
		p.supervisor = sup
		p.mu.Unlock()
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
		service := a.Service((*web.Server)(nil))
		srv, ok := service.(*web.Server)
		if !ok || srv == nil {
			return
		}
		p.mu.RLock()
		cfg := p.resolved
		p.mu.RUnlock()
		if cfg == nil {
			return
		}
		a.Router().Handle(routePattern(cfg.RoutePrefix), srv)
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
	p.shutdownOnce.Do(func() {
		p.mu.RLock()
		sup := p.supervisor
		mgr := p.manager
		driver := p.driver
		p.mu.RUnlock()

		var shutdownErrors []error
		if sup != nil {
			if err := sup.Stop(ctx); err != nil {
				shutdownErrors = append(shutdownErrors, fmt.Errorf("stop supervisor: %w", err))
			}
		}
		if mgr != nil {
			if err := mgr.Stop(ctx); err != nil {
				shutdownErrors = append(shutdownErrors, fmt.Errorf("stop manager: %w", err))
			}
			if tasker.Global() == mgr {
				tasker.SetGlobal(nil)
			}
		}
		if driver != nil {
			if err := driver.Close(); err != nil {
				shutdownErrors = append(shutdownErrors, fmt.Errorf("close driver: %w", err))
			}
		}
		p.shutdownErr = errors.Join(shutdownErrors...)
	})
	return p.shutdownErr
}

func routePattern(prefix string) string {
	return prefix + "/"
}

func newWebServer(mgr *tasker.Manager, sup *supervisor.Supervisor, prefix string) *web.Server {
	return web.NewWithPrefix(mgr, sup, prefix)
}

func createDriver(cfg *Config) (tasker.Driver, error) {
	switch cfg.Driver {
	case "sql", "postgres", "mysql", "sqlite", "sqlite3":
		return sqldriver.NewDriver(sqldriver.Config{
			DSN:             cfg.DSN,
			DriverName:      cfg.DriverName,
			MaxOpenConns:    cfg.MaxOpenConns,
			MaxIdleConns:    cfg.MaxIdleConns,
			ConnMaxLifetime: cfg.ConnMaxLifetime,
			TablePrefix:     cfg.TablePrefix,
		})
	case "redis":
		return redisdriver.NewDriver(redisdriver.Config{
			Addr:      cfg.RedisAddr,
			Password:  cfg.RedisPass,
			DB:        cfg.RedisDB,
			PoolSize:  cfg.RedisPoolSize,
			KeyPrefix: cfg.RedisPrefix,
		})
	default:
		return nil, fmt.Errorf("unsupported tasker driver: %s", cfg.Driver)
	}
}
