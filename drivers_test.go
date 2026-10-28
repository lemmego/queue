package queue

// The driver this package's tests open, imported here rather than by the
// package itself.
//
// provider.go used to blank-import sqlite, MySQL and Postgres, so every
// application linked all three and paid for the two it had never configured —
// the pure-Go sqlite driver alone is twenty-nine packages of translated libc.
// Registering a driver is the application's choice, which is the arrangement
// database/sql was designed around.
//
// config.go's own import of go-sql-driver/mysql is different and stays: it
// uses mysql.Config to build a DSN, which is a functional use rather than a
// registration.
import _ "github.com/glebarez/go-sqlite"
