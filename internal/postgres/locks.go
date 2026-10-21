package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

func AcquireWorkspaceAdvisoryLock(ctx context.Context, tx *sql.Tx, namespace, workspaceID string) error {
	if tx == nil {
		return errors.New("postgres transaction is required")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return errors.New("postgres advisory lock namespace is required")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return errors.New("postgres advisory lock workspace id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))`, namespace, workspaceID)
	return err
}

func TryAcquireWorkspaceAdvisoryLock(ctx context.Context, tx *sql.Tx, namespace, workspaceID string) (bool, error) {
	if tx == nil {
		return false, errors.New("postgres transaction is required")
	}
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return false, errors.New("postgres advisory lock namespace is required")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return false, errors.New("postgres advisory lock workspace id is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var acquired bool
	if err := tx.QueryRowContext(ctx, `SELECT pg_try_advisory_xact_lock(hashtext($1), hashtext($2))`, namespace, workspaceID).Scan(&acquired); err != nil {
		return false, err
	}
	return acquired, nil
}
