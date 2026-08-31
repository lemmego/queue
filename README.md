# Queue — Lemmego Tasker Plugin

**queue** is a standalone, self-contained module that wraps **tasker** as a lemmego-compatible provider. It implements `Provider`, `CommandProvider`, `RouteProvider`, `PublishableProvider`, and `ShutdownProvider` interfaces.

## Usage

### In `bootstrap/providers.go`

```go
import "github.com/lemmego/queue"

func LoadProviders() []app.Provider {
    return []app.Provider{
        &queue.Provider{},  // auto-configures from app config
    }
}
```

That's it. The provider auto-reads database credentials from your existing `configs/database.go` (same `sql.connections.<default>` that `gormconnector` uses). No hardcoded DSN needed.

### With explicit configuration

```go
&queue.Provider{
    Config: &queue.Config{
        Driver:             "redis",
        RedisAddr:          ":6379",
        RoutePrefix:        "/background-jobs",  // default: "/tasker"
        DefaultQueue:       "default",
        DefaultMaxAttempts: 3,
        Workers:            map[string]int{"default": 3, "email": 2},
        EnableAutoscale:    false,
    },
}
```

## Configuration Priority

```
1. Explicit Config fields (highest)
2. App config (published tasker.go + env vars)
3. Sensible defaults (lowest)
```

### `.env` overrides

```env
TASKER_ROUTE_PREFIX=/jobs
TASKER_DRIVER=sql
TASKER_DSN=
TASKER_SQL_DRIVER=
TASKER_TABLE_PREFIX=myapp_
TASKER_MAX_OPEN_CONNS=25
TASKER_MAX_IDLE_CONNS=10
TASKER_CONN_MAX_LIFETIME_SEC=0
TASKER_QUEUE=default
TASKER_MAX_ATTEMPTS=3
TASKER_WORKERS=3
TASKER_HEARTBEAT_SEC=5
TASKER_REQUEUE_INTERVAL_SEC=30
TASKER_REQUEUE_SEC=60
TASKER_PRUNE_INTERVAL_HOURS=24
TASKER_PRUNE_HOURS=168
TASKER_AUTOSCALE=false
```

### When Config is nil (auto-mode)

- Reads `sql.connections.<default>` from app config (same source as gormconnector)
- Picks up `driver`, `database`, `host`, `port`, `user`, `password`
- Works with SQLite, MySQL, PostgreSQL out of the box
- Uses `tasker_` table prefix, `/tasker` route prefix

### When Driver is `"redis"`

Skips SQL config entirely — use `RedisAddr`, `RedisPass`, `RedisDB` fields.

## What Gets Created

### Always (HTTP + Console)

- **Driver** — database connection (SQL or Redis)
- **Migration** — tables auto-created (`CREATE TABLE IF NOT EXISTS`)
- **Manager** — job dispatcher, registered as `tasker.Global()`

### HTTP only (skipped in console mode)

- **Supervisor** — registered for dashboard controls, but not automatically started
- **Web server** — dashboard UI + API at `RoutePrefix`

HTTP startup does not implicitly run workers because the application service container has no start lifecycle. Run `tasker:work` as a separately managed process. The provider stops any active queue resources and closes its database or Redis connection during application shutdown.

## Commands

Registered automatically via `CommandProvider`:

```bash
lemmego run tasker:work --queue=default --workers=3
```

## Routes

Mounted automatically via `RouteProvider`:

| Prefix | Description |
|---|---|
| `GET /tasker/` | Dashboard UI |
| `GET /tasker/api/*` | JSON API |
| `POST /tasker/api/*` | Actions (retry, cancel, pause, resume, prune) |

Customize with `Config.RoutePrefix`:

```go
Config: &Config{RoutePrefix: "/jobs"}  // → GET /jobs/api/stats
```

## Publishing Config

The provider publishes `internal/configs/tasker.go` via `AddPublishables()`:

```bash
lemmego run publish --tags=config
```

This creates a config file with env-var-driven defaults for all tasker settings.

## Dependencies

- `github.com/lemmego/tasker` — core job system
- `github.com/lemmego/api` — lemmego framework interfaces
- `github.com/spf13/cobra` — CLI commands

No GPA dependency. The driver manages its own connection pool directly.

## License

MIT
