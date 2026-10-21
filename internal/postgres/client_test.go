package postgres

import (
	"database/sql"
	"testing"
)

func TestNewClientRejectsEmptyDSN(t *testing.T) {
	client, err := NewClient(" ")
	if err == nil {
		t.Fatalf("expected error, got client=%v", client)
	}
}

func TestNewClientAcceptsDSN(t *testing.T) {
	client, err := NewClientFromDB(&sql.DB{}, "postgres://demo")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if client.DSN() != "postgres://demo" {
		t.Fatalf("expected dsn to be preserved, got %q", client.DSN())
	}
}
