package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
)

const cycleLockTestDriverName = "insight_cycle_lock_test"

var (
	registerCycleLockTestDriver sync.Once
	cycleLockTestConnections    = make(chan *cycleLockTestConn, 8)
)

type cycleLockTestDriver struct{}

func (cycleLockTestDriver) Open(name string) (driver.Conn, error) {
	conn := &cycleLockTestConn{acquired: name == "acquired"}
	cycleLockTestConnections <- conn
	return conn, nil
}

type cycleLockTestConn struct {
	mu        sync.Mutex
	acquired  bool
	query     string
	rollbacks int
}

func (*cycleLockTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepared statements are not supported")
}

func (c *cycleLockTestConn) Close() error { return nil }

func (c *cycleLockTestConn) Begin() (driver.Tx, error) {
	return &cycleLockTestTx{conn: c}, nil
}

func (c *cycleLockTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return c.Begin()
}

func (c *cycleLockTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	c.mu.Lock()
	c.query = query
	acquired := c.acquired
	c.mu.Unlock()
	return &cycleLockTestRows{acquired: acquired}, nil
}

func (c *cycleLockTestConn) CheckNamedValue(*driver.NamedValue) error { return nil }

type cycleLockTestTx struct {
	conn *cycleLockTestConn
}

func (t *cycleLockTestTx) Commit() error { return nil }

func (t *cycleLockTestTx) Rollback() error {
	t.conn.mu.Lock()
	t.conn.rollbacks++
	t.conn.mu.Unlock()
	return nil
}

type cycleLockTestRows struct {
	acquired bool
	read     bool
}

func (*cycleLockTestRows) Columns() []string { return []string{"acquired"} }
func (*cycleLockTestRows) Close() error      { return nil }

func (r *cycleLockTestRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	dest[0] = r.acquired
	return nil
}

func openCycleLockTestDB(t *testing.T, acquired bool) (*sql.DB, *cycleLockTestConn) {
	t.Helper()
	registerCycleLockTestDriver.Do(func() {
		sql.Register(cycleLockTestDriverName, cycleLockTestDriver{})
	})
	name := "unavailable"
	if acquired {
		name = "acquired"
	}
	db, err := sql.Open(cycleLockTestDriverName, name)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("ping test database: %v", err)
	}
	return db, <-cycleLockTestConnections
}

func TestAdvisoryCycleLockerAcquiresAndReleasesTransactionLock(t *testing.T) {
	db, conn := openCycleLockTestDB(t, true)
	defer db.Close()
	client, err := NewClientFromDB(db, "postgres://cycle-lock-test")
	if err != nil {
		t.Fatalf("new postgres client: %v", err)
	}
	locker, err := NewAdvisoryCycleLocker(client)
	if err != nil {
		t.Fatalf("new advisory locker: %v", err)
	}

	release, acquired, err := locker.TryAcquire(context.Background(), "hub:user:alice")
	if err != nil || !acquired {
		t.Fatalf("expected advisory lock acquisition, acquired=%t err=%v", acquired, err)
	}
	if release == nil {
		t.Fatal("expected release function")
	}
	release()
	release()

	conn.mu.Lock()
	query, rollbacks := conn.query, conn.rollbacks
	conn.mu.Unlock()
	if !strings.Contains(query, "pg_try_advisory_xact_lock") {
		t.Fatalf("expected nonblocking advisory lock query, got %q", query)
	}
	if rollbacks != 1 {
		t.Fatalf("expected one idempotent transaction rollback, got %d", rollbacks)
	}
}

func TestAdvisoryCycleLockerRollsBackWhenLockIsContended(t *testing.T) {
	db, conn := openCycleLockTestDB(t, false)
	defer db.Close()
	client, err := NewClientFromDB(db, "postgres://cycle-lock-test")
	if err != nil {
		t.Fatalf("new postgres client: %v", err)
	}
	locker, err := NewAdvisoryCycleLocker(client)
	if err != nil {
		t.Fatalf("new advisory locker: %v", err)
	}

	release, acquired, err := locker.TryAcquire(context.Background(), "hub:user:alice")
	if err != nil || acquired || release != nil {
		t.Fatalf("expected nonblocking contention, release_present=%t acquired=%t err=%v", release != nil, acquired, err)
	}
	conn.mu.Lock()
	rollbacks := conn.rollbacks
	conn.mu.Unlock()
	if rollbacks != 1 {
		t.Fatalf("expected immediate rollback after contention, got %d", rollbacks)
	}
}

func TestAdvisoryCycleLockerRejectsEmptyKey(t *testing.T) {
	db, _ := openCycleLockTestDB(t, true)
	defer db.Close()
	client, err := NewClientFromDB(db, "postgres://cycle-lock-test")
	if err != nil {
		t.Fatalf("new postgres client: %v", err)
	}
	locker, err := NewAdvisoryCycleLocker(client)
	if err != nil {
		t.Fatalf("new advisory locker: %v", err)
	}
	_, acquired, err := locker.TryAcquire(context.Background(), " ")
	if err == nil || acquired || !strings.Contains(err.Error(), "workspace id is required") {
		t.Fatalf("expected empty key validation error, acquired=%t err=%v", acquired, err)
	}
}

func TestNewAdvisoryCycleLockerRejectsMissingClient(t *testing.T) {
	if _, err := NewAdvisoryCycleLocker(nil); err == nil {
		t.Fatal("expected missing postgres client error")
	}
}

var _ driver.Conn = (*cycleLockTestConn)(nil)
var _ driver.ConnBeginTx = (*cycleLockTestConn)(nil)
var _ driver.QueryerContext = (*cycleLockTestConn)(nil)
var _ driver.NamedValueChecker = (*cycleLockTestConn)(nil)
