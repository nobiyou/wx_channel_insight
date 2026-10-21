package testpostgres

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// Open connects repository tests to an isolated database. An explicit test DSN
// makes connection failures fatal so configured integration runs cannot skip.
func Open(t testing.TB) (*sql.DB, string) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("INSIGHT_POSTGRES_TEST_DSN"))
	configured := dsn != ""
	if !configured {
		dsn = "postgres://postgres:postgres@127.0.0.1:15432/wx_channel_insight_test?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err = db.PingContext(ctx)
		cancel()
	}
	if err != nil {
		if db != nil {
			_ = db.Close()
		}
		if configured {
			t.Fatalf("configured PostgreSQL test database is unavailable: %v", err)
		}
		t.Skipf("PostgreSQL test database is unavailable: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, dsn
}
