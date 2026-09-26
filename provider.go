package queue

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	_ "github.com/glebarez/go-sqlite"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"github.com/spf13/cobra"

	"github.com/lemmego/api/app"
	"github.com/lemmego/api/config"
	"github.com/lemmego/api/db"
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

	// An application usually has one Redis. Reading the shared connection
	// block means a project that configured Redis once does not have to
	// repeat the address under tasker, while tasker.redis_addr still wins.
	sharedRedis, _ := a.Config().Get("keyvalue.connections.redis").(config.M)

	cfg, err := resolveConfig(p.Config, taskerCfg, appCfg, sharedRedis)
	if err != nil {
		return err
	}

	// Borrow the application's connection when there is one and no DSN was
	// configured to point somewhere else. Before this, the queue re-derived
	// a DSN from the same sql block the ORM reads, with its own precedence
	// rules — and the two drifted: the ORM ignored a connection's url key
	// while the queue preferred it, so a scaffolded project ran its queue
	// against an in-memory database and lost every job on restart.
	//
	// Nothing is registered when the project has no database, and that is
	// fine: the derive path below still handles a queue with its own DSN.
	conn, _ := db.Resolve(a)

	d, err := createDriver(cfg, conn)
	if err != nil {
		return fmt.Errorf("tasker: failed to create driver: %w", err)
	}
	logDriverSource(cfg, conn)

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
			// Namespaced, because --tags is a real selector: a bare
			// "config" collides with every other package that publishes
			// one, so asking for this module's meant getting all of them.
			Tag: TagConfig,
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

func createDriver(cfg *Config, conn db.Connection) (tasker.Driver, error) {
	switch cfg.Driver {
	case "sql", "postgres", "mysql", "sqlite", "sqlite3":
		settings := sqldriver.Config{
			DSN:             cfg.DSN,
			DriverName:      cfg.DriverName,
			MaxOpenConns:    cfg.MaxOpenConns,
			MaxIdleConns:    cfg.MaxIdleConns,
			ConnMaxLifetime: cfg.ConnMaxLifetime,
			TablePrefix:     cfg.TablePrefix,
		}
		if borrow, err := shouldBorrow(cfg, conn); err != nil {
			return nil, err
		} else if borrow {
			settings.DB = conn.SQLDB()
			settings.Dialect = string(conn.Dialect())
		}
		return sqldriver.NewDriver(settings)
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

// shouldBorrow reports whether the queue should run against the application's
// connection rather than opening its own.
//
// An explicitly configured DSN always wins: that is how an application says
// "keep the queue in its own database", which is a legitimate choice for
// isolating job traffic from request traffic.
//
// A dialect disagreement is refused rather than resolved. If the queue is
// configured for postgres and the application opened mysql, one of the two is
// wrong, and guessing would generate SQL the database rejects at the first
// job rather than at boot.
func shouldBorrow(cfg *Config, conn db.Connection) (bool, error) {
	if conn == nil || cfg.dsnExplicit {
		return false, nil
	}
	if conn.SQLDB() == nil || conn.Dialect() == db.DialectUnknown {
		return false, nil
	}

	requested := cfg.DriverName
	if cfg.Driver != "" && cfg.Driver != "sql" {
		requested = cfg.Driver
	}
	if requested == "" {
		return true, nil
	}
	wanted, known := db.ParseDialect(requested)
	if !known {
		return false, fmt.Errorf("tasker: unsupported driver %q", requested)
	}
	if wanted != conn.Dialect() {
		return false, fmt.Errorf(
			"tasker: configured for %s but the application's database is %s; "+
				"set TASKER_DSN to give the queue its own database, or drop tasker.driver to share the application's",
			wanted, conn.Dialect())
	}
	return true, nil
}

// logDriverSource says which database the queue ended up on. Sharing the
// application's connection is silent and invisible otherwise, and the failure
// this replaces — a queue quietly running somewhere else — was invisible
// precisely because nothing ever said.
func logDriverSource(cfg *Config, conn db.Connection) {
	if cfg.Driver == "redis" {
		slog.Info("tasker: using redis", "addr", cfg.RedisAddr)
		return
	}
	if borrow, err := shouldBorrow(cfg, conn); err == nil && borrow {
		slog.Info("tasker: sharing the application's database connection",
			"connection", conn.Name(), "dialect", conn.Dialect())
		return
	}
	slog.Info("tasker: opening its own database connection",
		"driver", cfg.DriverName, "dsn", redactDSN(cfg.DSN))
}

// redactDSN keeps credentials out of the log while leaving enough to tell one
// database from another, which is the whole point of logging it.
func redactDSN(dsn string) string {
	if at := strings.LastIndex(dsn, "@"); at >= 0 {
		return "***@" + dsn[at+1:]
	}
	return dsn
}

// TagConfig is what `lemmego publish --tags` selects this module's
// configuration on.
const TagConfig = "queue-config"
