package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type Client struct {
	db  *sql.DB
	dsn string
}

func NewClient(dsn string) (*Client, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, errors.New("postgres dsn is required")
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return NewClientFromDB(db, dsn)
}

func NewClientFromDB(db *sql.DB, dsn string) (*Client, error) {
	if db == nil {
		return nil, errors.New("postgres db is required")
	}
	return &Client{
		db:  db,
		dsn: strings.TrimSpace(dsn),
	}, nil
}

func (c *Client) DB() *sql.DB {
	if c == nil {
		return nil
	}
	return c.db
}

func (c *Client) DSN() string {
	if c == nil {
		return ""
	}
	return c.dsn
}

func (c *Client) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}
