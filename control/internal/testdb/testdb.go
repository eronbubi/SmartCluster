// Package testdb gives tests a real, migrated Postgres database.
//
// One embedded Postgres 18 runs per test binary; every call to New creates a
// fresh database in it. The first run downloads the Postgres binaries into
// the embedded-postgres cache; nothing is installed on the system.
package testdb

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5"

	"github.com/eronbubi/smartcluster/control/internal/db"
	"github.com/eronbubi/smartcluster/pkg/testenv"
)

var (
	startOnce sync.Once
	startErr  error
	server    *embeddedpostgres.EmbeddedPostgres
	runtime   string
	port      uint32
	counter   atomic.Int64
)

// Main runs the tests of a package and stops Postgres afterwards. Use it as
// TestMain: func TestMain(m *testing.M) { testdb.Main(m) }
func Main(m *testing.M) {
	code := m.Run()
	if server != nil {
		_ = server.Stop()
		_ = os.RemoveAll(runtime)
	}
	os.Exit(code)
}

func start() error {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	port = uint32(l.Addr().(*net.TCPAddr).Port)
	l.Close()
	runtime, err = os.MkdirTemp("", "smartcluster-pg-*")
	if err != nil {
		return err
	}
	server = embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Version(embeddedpostgres.V18).
		Port(port).
		RuntimePath(runtime).
		DataPath(filepath.Join(runtime, "data")).
		Username("smartcluster").Password("test").Database("postgres").
		StartTimeout(2 * time.Minute).
		Logger(io.Discard))
	return server.Start()
}

func url(database string) string {
	return fmt.Sprintf("postgres://smartcluster:test@127.0.0.1:%d/%s?sslmode=disable", port, database)
}

// New returns the URL of a new, migrated database.
func New(t testing.TB) string {
	t.Helper()
	testenv.RequireServers(t)
	startOnce.Do(func() { startErr = start() })
	if startErr != nil {
		t.Fatalf("testdb: start embedded postgres: %v", startErr)
	}
	ctx := context.Background()
	name := fmt.Sprintf("test_%d_%d", os.Getpid(), counter.Add(1))
	conn, err := pgx.Connect(ctx, url("postgres"))
	if err != nil {
		t.Fatalf("testdb: connect: %v", err)
	}
	defer conn.Close(ctx)
	if _, err := conn.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatalf("testdb: create database: %v", err)
	}
	u := url(name)
	if err := db.Migrate(ctx, u); err != nil {
		t.Fatalf("testdb: migrate: %v", err)
	}
	return u
}
