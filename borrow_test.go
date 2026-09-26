package queue

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/lemmego/api/db"
)

type fakeConn struct {
	pool    *sql.DB
	dialect db.Dialect
}

func (f *fakeConn) SQLDB() *sql.DB      { return f.pool }
func (f *fakeConn) Dialect() db.Dialect { return f.dialect }
func (f *fakeConn) Name() string        { return "primary" }

func sqliteConn(t *testing.T) *fakeConn {
	t.Helper()
	pool, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "app.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return &fakeConn{pool: pool, dialect: db.SQLite}
}

// The queue used to re-derive a DSN from the same sql block the ORM reads,
// with different precedence rules. The two drifted and the queue silently ran
// against a different database, losing every job on restart. When the
// application has a connection and nothing says otherwise, share it.
func TestBorrowsTheApplicationConnection(t *testing.T) {
	conn := sqliteConn(t)
	cfg := DefaultConfig()

	borrow, err := shouldBorrow(cfg, conn)
	if err != nil {
		t.Fatal(err)
	}
	if !borrow {
		t.Fatal("the queue did not borrow the application's connection")
	}

	driver, err := createDriver(cfg, conn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = driver.Close() })

	// Closing the driver must leave the application's pool alone.
	if err := driver.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := conn.pool.Ping(); err != nil {
		t.Fatalf("shutting the queue down closed the application's pool: %v", err)
	}
}

// An explicitly configured DSN is how an application says "keep the queue in
// its own database". That has to win, or the option would do nothing.
func TestAnExplicitDSNIsNotOverriddenByTheSeam(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DSN = filepath.Join(t.TempDir(), "queue-only.sqlite")
	cfg.dsnExplicit = true

	borrow, err := shouldBorrow(cfg, sqliteConn(t))
	if err != nil {
		t.Fatal(err)
	}
	if borrow {
		t.Fatal("a configured DSN was ignored in favour of the application's connection")
	}
}

// One of the two is misconfigured and the server cannot tell which, so
// refusing at boot beats generating SQL the database rejects at the first job.
func TestADialectDisagreementIsRefused(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Driver = "postgres"

	_, err := shouldBorrow(cfg, sqliteConn(t))
	if err == nil {
		t.Fatal("a postgres queue silently accepted a sqlite connection")
	}
}

// A project scaffolded without a database is ordinary. The queue must fall
// back to its own DSN rather than dereference a connection that is not there.
func TestNoConnectionFallsBackToItsOwnDSN(t *testing.T) {
	borrow, err := shouldBorrow(DefaultConfig(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if borrow {
		t.Fatal("shouldBorrow reported true with no connection to borrow")
	}
}
