// Package sqlite owns the embedded database and its transactional document
// primitives. Domain adapters keep their existing interfaces; PostgreSQL remains
// the indexed relational backend for larger deployments.
package sqlite

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type Client struct{ db *sql.DB }

func Open(path string) (*Client, error) {
	if strings.TrimSpace(path) == "" || path == ":memory:" {
		return nil, errors.New("sqlite requires a local database file path")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0700); err != nil {
		return nil, fmt.Errorf("create sqlite directory: %w", err)
	}
	// Create with restricted permissions before the driver opens it (umask may
	// tighten these further). Never truncate an existing database.
	file, err := os.OpenFile(abs, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open sqlite file: %w", err)
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	uri := &url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(abs), "/")}
	q := uri.Query()
	q.Add("_pragma", "busy_timeout(10000)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(NORMAL)")
	q.Set("_txlock", "immediate")
	uri.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS insight_documents (
 seq INTEGER PRIMARY KEY AUTOINCREMENT,
 bucket TEXT NOT NULL, scope TEXT NOT NULL, id TEXT NOT NULL,
 payload BLOB NOT NULL CHECK(json_valid(payload)),
 UNIQUE(bucket, scope, id)
)`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize sqlite: %w", err)
	}
	return &Client{db: db}, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	return c.db.Close()
}

type queryer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func (c *Client) update(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func get[T any](ctx context.Context, q queryer, bucket, scope, id string) (value T, found bool, err error) {
	var data []byte
	err = q.QueryRowContext(ctx, `SELECT payload FROM insight_documents WHERE bucket=? AND scope=? AND id=?`, bucket, scope, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return value, false, nil
	}
	if err != nil {
		return value, false, err
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return value, false, fmt.Errorf("decode sqlite %s document: %w", bucket, err)
	}
	return value, true, nil
}

func put(ctx context.Context, q queryer, bucket, scope, id string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = q.ExecContext(ctx, `INSERT INTO insight_documents(bucket,scope,id,payload) VALUES(?,?,?,?) ON CONFLICT(bucket,scope,id) DO UPDATE SET payload=excluded.payload`, bucket, scope, id, data)
	return err
}

func list[T any](ctx context.Context, q queryer, bucket, scope string) ([]T, error) {
	rows, err := q.QueryContext(ctx, `SELECT payload FROM insight_documents WHERE bucket=? AND scope=? ORDER BY seq`, bucket, scope)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]T, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item T
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, fmt.Errorf("decode sqlite %s document: %w", bucket, err)
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func remove(ctx context.Context, q queryer, bucket, scope, id string) (bool, error) {
	result, err := q.ExecContext(ctx, `DELETE FROM insight_documents WHERE bucket=? AND scope=? AND id=?`, bucket, scope, id)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func clear(ctx context.Context, q queryer, bucket, scope string) error {
	_, err := q.ExecContext(ctx, `DELETE FROM insight_documents WHERE bucket=? AND scope=?`, bucket, scope)
	return err
}

func appendDocument(ctx context.Context, q queryer, bucket, scope string, value any) error {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return err
	}
	return put(ctx, q, bucket, scope, hex.EncodeToString(id[:]), value)
}
