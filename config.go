package queue

import (
	"fmt"
	"net"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"
	"github.com/lemmego/api/config"
)

type Config struct {
	Driver          string
	DSN             string
	DriverName      string
	TablePrefix     string
	RoutePrefix     string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration

	RedisAddr     string
	RedisPass     string
	RedisDB       int
	RedisPoolSize int
	RedisPrefix   string

	DefaultQueue       string
	DefaultMaxAttempts int

	Workers         map[string]int
	EnableAutoscale bool

	HeartbeatInterval int
	RequeueInterval   int
	RequeueTimeout    int
	PruneInterval     int
	PruneAfterHours   int
}

func DefaultConfig() *Config {
	return &Config{
		Driver:             "sql",
		TablePrefix:        "tasker_",
		RoutePrefix:        "/tasker",
		MaxOpenConns:       25,
		MaxIdleConns:       10,
		RedisAddr:          "localhost:6379",
		RedisPrefix:        "tasker:",
		DefaultQueue:       "default",
		DefaultMaxAttempts: 3,
		HeartbeatInterval:  5,
		RequeueInterval:    30,
		RequeueTimeout:     60,
		PruneInterval:      24,
		PruneAfterHours:    168,
		Workers:            map[string]int{"default": 3},
	}
}

// ApplyOverrides applies every present key, including false and zero values.
func (cfg *Config) ApplyOverrides(c config.M) {
	if c == nil {
		return
	}
	applyString(c, "driver", &cfg.Driver)
	applyString(c, "dsn", &cfg.DSN)
	applyString(c, "driver_name", &cfg.DriverName)
	applyString(c, "table_prefix", &cfg.TablePrefix)
	applyString(c, "route_prefix", &cfg.RoutePrefix)
	applyInt(c, "max_open_conns", &cfg.MaxOpenConns)
	applyInt(c, "max_idle_conns", &cfg.MaxIdleConns)
	applyDuration(c, "conn_max_lifetime", &cfg.ConnMaxLifetime)
	applyString(c, "redis_addr", &cfg.RedisAddr)
	applyString(c, "redis_password", &cfg.RedisPass)
	applyInt(c, "redis_db", &cfg.RedisDB)
	applyInt(c, "redis_pool_size", &cfg.RedisPoolSize)
	applyString(c, "redis_prefix", &cfg.RedisPrefix)
	applyString(c, "queue", &cfg.DefaultQueue)
	applyInt(c, "max_attempts", &cfg.DefaultMaxAttempts)
	applyInt(c, "heartbeat_interval", &cfg.HeartbeatInterval)
	applyInt(c, "requeue_interval", &cfg.RequeueInterval)
	applyInt(c, "requeue_timeout", &cfg.RequeueTimeout)
	applyInt(c, "prune_interval", &cfg.PruneInterval)
	applyInt(c, "prune_after_hours", &cfg.PruneAfterHours)
	if v, ok := c["autoscale"].(bool); ok {
		cfg.EnableAutoscale = v
	}
	if workers, ok := workerConfig(c["workers"]); ok {
		cfg.Workers = workers
	}
}

func applyString(c config.M, key string, dst *string) {
	if v, ok := c[key].(string); ok {
		*dst = v
	}
}

func applyInt(c config.M, key string, dst *int) {
	if v, ok := c[key].(int); ok {
		*dst = v
	}
}

func applyDuration(c config.M, key string, dst *time.Duration) {
	if v, ok := c[key].(time.Duration); ok {
		*dst = v
	}
}

func workerConfig(value any) (map[string]int, bool) {
	workers := make(map[string]int)
	switch value := value.(type) {
	case map[string]int:
		for name, count := range value {
			workers[name] = count
		}
	case config.M:
		for name, raw := range value {
			count, ok := raw.(int)
			if !ok {
				return nil, false
			}
			workers[name] = count
		}
	case map[string]any:
		for name, raw := range value {
			count, ok := raw.(int)
			if !ok {
				return nil, false
			}
			workers[name] = count
		}
	default:
		return nil, false
	}
	return workers, true
}

// applyExplicit overlays non-zero fields from the public provider Config.
// Maps are copied so resolving configuration never mutates caller-owned data.
func (cfg *Config) applyExplicit(explicit *Config) {
	if explicit == nil {
		return
	}
	if explicit.Driver != "" {
		cfg.Driver = explicit.Driver
	}
	if explicit.DSN != "" {
		cfg.DSN = explicit.DSN
	}
	if explicit.DriverName != "" {
		cfg.DriverName = explicit.DriverName
	}
	if explicit.TablePrefix != "" {
		cfg.TablePrefix = explicit.TablePrefix
	}
	if explicit.RoutePrefix != "" {
		cfg.RoutePrefix = explicit.RoutePrefix
	}
	if explicit.MaxOpenConns != 0 {
		cfg.MaxOpenConns = explicit.MaxOpenConns
	}
	if explicit.MaxIdleConns != 0 {
		cfg.MaxIdleConns = explicit.MaxIdleConns
	}
	if explicit.ConnMaxLifetime != 0 {
		cfg.ConnMaxLifetime = explicit.ConnMaxLifetime
	}
	if explicit.RedisAddr != "" {
		cfg.RedisAddr = explicit.RedisAddr
	}
	if explicit.RedisPass != "" {
		cfg.RedisPass = explicit.RedisPass
	}
	if explicit.RedisDB != 0 {
		cfg.RedisDB = explicit.RedisDB
	}
	if explicit.RedisPoolSize != 0 {
		cfg.RedisPoolSize = explicit.RedisPoolSize
	}
	if explicit.RedisPrefix != "" {
		cfg.RedisPrefix = explicit.RedisPrefix
	}
	if explicit.DefaultQueue != "" {
		cfg.DefaultQueue = explicit.DefaultQueue
	}
	if explicit.DefaultMaxAttempts != 0 {
		cfg.DefaultMaxAttempts = explicit.DefaultMaxAttempts
	}
	if explicit.Workers != nil {
		cfg.Workers, _ = workerConfig(explicit.Workers)
	}
	cfg.EnableAutoscale = explicit.EnableAutoscale
	if explicit.HeartbeatInterval != 0 {
		cfg.HeartbeatInterval = explicit.HeartbeatInterval
	}
	if explicit.RequeueInterval != 0 {
		cfg.RequeueInterval = explicit.RequeueInterval
	}
	if explicit.RequeueTimeout != 0 {
		cfg.RequeueTimeout = explicit.RequeueTimeout
	}
	if explicit.PruneInterval != 0 {
		cfg.PruneInterval = explicit.PruneInterval
	}
	if explicit.PruneAfterHours != 0 {
		cfg.PruneAfterHours = explicit.PruneAfterHours
	}
}

func resolveConfig(explicit *Config, taskerConfig, sqlConfig config.M) (*Config, error) {
	cfg := DefaultConfig()
	// Apply once to select the requested connection, then again so queue-specific
	// settings retain priority over inherited database pool settings.
	cfg.ApplyOverrides(taskerConfig)
	cfg.applyExplicit(explicit)
	if err := cfg.autoFillDatabase(sqlConfig); err != nil {
		return nil, err
	}
	cfg.ApplyOverrides(taskerConfig)
	cfg.applyExplicit(explicit)
	if rawWorkers, exists := taskerConfig["workers"]; exists {
		if _, ok := workerConfig(rawWorkers); !ok {
			return nil, fmt.Errorf("tasker: workers must map queue names to integer counts")
		}
	}
	if err := cfg.normalizeAndValidate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (cfg *Config) autoFillDatabase(sqlConfig config.M) error {
	cfg.Driver = strings.ToLower(strings.TrimSpace(cfg.Driver))
	if cfg.Driver == "redis" || cfg.DSN != "" {
		return nil
	}

	requestedDriver := cfg.DriverName
	if cfg.Driver != "" && cfg.Driver != "sql" {
		requestedDriver = cfg.Driver
	}
	connections, _ := asConfigMap(sqlConfig["connections"])
	connectionName := stringValue(sqlConfig["default"])
	if requestedDriver != "" {
		for name, raw := range connections {
			connection, ok := asConfigMap(raw)
			if ok && canonicalSQLDriver(connection.String("driver", name)) == canonicalSQLDriver(requestedDriver) {
				connectionName = name
				break
			}
		}
	}
	connection, ok := asConfigMap(connections[connectionName])
	if !ok {
		if len(connections) != 0 {
			return fmt.Errorf("tasker: SQL connection %q is not configured", connectionName)
		}
		cfg.Driver = "sql"
		cfg.DriverName = "sqlite"
		cfg.DSN = "./storage/database.sqlite"
		return nil
	}

	driverName := canonicalSQLDriver(connection.String("driver", connectionName))
	if driverName == "" {
		driverName = canonicalSQLDriver(requestedDriver)
	}
	cfg.Driver = "sql"
	cfg.DriverName = driverName
	if rawURL := connection.String("url", ""); rawURL != "" {
		cfg.DSN = rawURL
	} else {
		switch driverName {
		case "sqlite":
			cfg.DSN = connection.String("database", "./storage/database.sqlite")
		case "postgres":
			cfg.DSN = postgresDSN(connection)
		case "mysql":
			cfg.DSN = mysqlDSN(connection)
		default:
			return fmt.Errorf("tasker: unsupported SQL driver %q", driverName)
		}
	}
	applyInt(connection, "max_open_conns", &cfg.MaxOpenConns)
	applyInt(connection, "max_idle_conns", &cfg.MaxIdleConns)
	applyDuration(connection, "conn_max_lifetime", &cfg.ConnMaxLifetime)
	return nil
}

func inferSQLDriver(dsn string) string {
	lowerDSN := strings.ToLower(strings.TrimSpace(dsn))
	switch {
	case strings.HasPrefix(lowerDSN, "postgres://"), strings.HasPrefix(lowerDSN, "postgresql://"):
		return "postgres"
	case strings.Contains(lowerDSN, "@tcp("), strings.Contains(lowerDSN, "@unix("):
		return "mysql"
	case lowerDSN == ":memory:", strings.HasPrefix(lowerDSN, "file:"), strings.HasSuffix(lowerDSN, ".sqlite"), strings.HasSuffix(lowerDSN, ".db"):
		return "sqlite"
	default:
		return ""
	}
}

func asConfigMap(value any) (config.M, bool) {
	switch value := value.(type) {
	case config.M:
		return value, true
	case map[string]any:
		return config.M(value), true
	default:
		return nil, false
	}
}

func stringValue(value any) string {
	valueString, _ := value.(string)
	return valueString
}

func postgresDSN(connection config.M) string {
	host := connection.String("host", "localhost")
	port := connection.Int("port", 5432)
	dsn := &url.URL{
		Scheme: "postgres",
		Host:   net.JoinHostPort(host, strconv.Itoa(port)),
		Path:   connection.String("database", ""),
	}
	user := connection.String("user", "")
	password := connection.String("password", "")
	if password != "" {
		dsn.User = url.UserPassword(user, password)
	} else if user != "" {
		dsn.User = url.User(user)
	}
	query := dsn.Query()
	query.Set("sslmode", connection.String("sslmode", "disable"))
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

func mysqlDSN(connection config.M) string {
	dsn := mysql.Config{
		User:      connection.String("user", ""),
		Passwd:    connection.String("password", ""),
		Net:       "tcp",
		Addr:      net.JoinHostPort(connection.String("host", "localhost"), strconv.Itoa(connection.Int("port", 3306))),
		DBName:    connection.String("database", ""),
		ParseTime: true,
	}
	return dsn.FormatDSN()
}

func canonicalSQLDriver(driver string) string {
	switch strings.ToLower(strings.TrimSpace(driver)) {
	case "sqlite", "sqlite3", "modernc.org/sqlite":
		return "sqlite"
	case "postgres", "postgresql", "pgsql", "pgx":
		return "postgres"
	case "mysql", "mariadb":
		return "mysql"
	default:
		return strings.ToLower(strings.TrimSpace(driver))
	}
}

var tablePrefixPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func (cfg *Config) normalizeAndValidate() error {
	cfg.Driver = strings.ToLower(strings.TrimSpace(cfg.Driver))
	if cfg.Driver != "redis" {
		if cfg.Driver != "sql" {
			cfg.DriverName = cfg.Driver
			cfg.Driver = "sql"
		}
		cfg.DriverName = canonicalSQLDriver(cfg.DriverName)
		if cfg.DriverName == "" {
			cfg.DriverName = inferSQLDriver(cfg.DSN)
		}
	}

	prefix := strings.TrimSpace(cfg.RoutePrefix)
	if prefix == "" {
		prefix = "/tasker"
	}
	if !strings.HasPrefix(prefix, "/") || strings.ContainsAny(prefix, "?#") {
		return fmt.Errorf("tasker: route_prefix must be an absolute URL path")
	}
	cfg.RoutePrefix = path.Clean(prefix)
	if cfg.RoutePrefix == "/" {
		return fmt.Errorf("tasker: route_prefix cannot be the root path")
	}

	if cfg.TablePrefix != "" && !tablePrefixPattern.MatchString(cfg.TablePrefix) {
		return fmt.Errorf("tasker: table_prefix must contain only letters, digits, and underscores and cannot start with a digit")
	}
	if cfg.DefaultQueue == "" {
		return fmt.Errorf("tasker: queue cannot be empty")
	}
	if cfg.DefaultMaxAttempts < 1 {
		return fmt.Errorf("tasker: max_attempts must be at least 1")
	}
	for name, count := range cfg.Workers {
		if strings.TrimSpace(name) == "" || count < 1 {
			return fmt.Errorf("tasker: worker queues must have a name and at least one worker")
		}
	}
	if cfg.HeartbeatInterval < 1 || cfg.RequeueInterval < 1 || cfg.RequeueTimeout < 1 || cfg.PruneInterval < 1 || cfg.PruneAfterHours < 1 {
		return fmt.Errorf("tasker: supervisor intervals must be positive")
	}

	if cfg.Driver == "redis" {
		if cfg.RedisAddr == "" {
			return fmt.Errorf("tasker: redis_addr cannot be empty")
		}
		if cfg.RedisDB < 0 || cfg.RedisPoolSize < 0 {
			return fmt.Errorf("tasker: Redis database and pool size cannot be negative")
		}
		return nil
	}
	if cfg.DriverName != "sqlite" && cfg.DriverName != "postgres" && cfg.DriverName != "mysql" {
		return fmt.Errorf("tasker: unsupported SQL driver %q", cfg.DriverName)
	}
	if strings.TrimSpace(cfg.DSN) == "" {
		return fmt.Errorf("tasker: dsn cannot be empty for SQL driver %q", cfg.DriverName)
	}
	if cfg.MaxOpenConns < 0 || cfg.MaxIdleConns < 0 || cfg.ConnMaxLifetime < 0 {
		return fmt.Errorf("tasker: SQL pool settings cannot be negative")
	}
	if cfg.MaxOpenConns > 0 && cfg.MaxIdleConns > cfg.MaxOpenConns {
		return fmt.Errorf("tasker: max_idle_conns cannot exceed max_open_conns")
	}
	return nil
}
