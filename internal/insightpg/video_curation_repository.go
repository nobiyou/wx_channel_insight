package insightpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type VideoCurationRepository struct{ db *sql.DB }

func NewVideoCurationRepository(client *postgres.Client) (*VideoCurationRepository, error) {
	if client == nil || client.DB() == nil {
		return nil, errors.New("postgres client is required")
	}
	_, err := client.DB().Exec(`CREATE TABLE IF NOT EXISTS insight_video_curations (
workspace_id TEXT NOT NULL, video_id TEXT NOT NULL,
status TEXT NOT NULL CHECK (status IN ('unreviewed','selected','watching','excluded')),
note TEXT NOT NULL DEFAULT '', tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
updated_at TIMESTAMPTZ NOT NULL, updated_by TEXT NOT NULL,
PRIMARY KEY (workspace_id,video_id))`)
	if err != nil {
		return nil, err
	}
	if _, err := client.DB().Exec(`CREATE TABLE IF NOT EXISTS insight_operations_agent (
workspace_id TEXT PRIMARY KEY, state_json JSONB NOT NULL DEFAULT '{}'::jsonb)`); err != nil {
		return nil, err
	}
	return &VideoCurationRepository{db: client.DB()}, nil
}

func (r *VideoCurationRepository) List(ctx context.Context, workspaceID string) ([]insight.VideoCuration, error) {
	return listCurations(ctx, r.db, workspaceID)
}

type curationQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listCurations(ctx context.Context, db curationQuerier, workspaceID string) ([]insight.VideoCuration, error) {
	rows, err := db.QueryContext(ctx, `SELECT video_id,status,note,tags_json,updated_at,updated_by FROM insight_video_curations WHERE workspace_id=$1 ORDER BY video_id`, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []insight.VideoCuration{}
	for rows.Next() {
		var item insight.VideoCuration
		var tags []byte
		item.WorkspaceID = workspaceID
		if err := rows.Scan(&item.VideoID, &item.Status, &item.Note, &tags, &item.UpdatedAt, &item.UpdatedBy); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(tags, &item.Tags); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *VideoCurationRepository) SaveBatch(ctx context.Context, items []insight.VideoCuration) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	workspaces := map[string]bool{}
	for _, item := range items {
		workspaces[item.WorkspaceID] = true
	}
	keys := make([]string, 0, len(workspaces))
	for key := range workspaces {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := lockOperationsWorkspace(ctx, tx, key); err != nil {
			return err
		}
	}
	if err := saveCurations(ctx, tx, items); err != nil {
		return err
	}
	return tx.Commit()
}

func saveCurations(ctx context.Context, tx *sql.Tx, items []insight.VideoCuration) error {
	for _, item := range items {
		tags, err := json.Marshal(item.Tags)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO insight_video_curations (workspace_id,video_id,status,note,tags_json,updated_at,updated_by) VALUES ($1,$2,$3,$4,$5,$6,$7)
ON CONFLICT (workspace_id,video_id) DO UPDATE SET status=EXCLUDED.status,note=EXCLUDED.note,tags_json=EXCLUDED.tags_json,updated_at=EXCLUDED.updated_at,updated_by=EXCLUDED.updated_by`, item.WorkspaceID, item.VideoID, item.Status, item.Note, tags, item.UpdatedAt, item.UpdatedBy)
		if err != nil {
			return err
		}
	}
	return nil
}

func lockOperationsWorkspace(ctx context.Context, tx *sql.Tx, workspace string) error {
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "insight-curation:"+workspace)
	return err
}

func (r *VideoCurationRepository) MutateCurations(ctx context.Context, workspace string, mutate func(map[string]insight.VideoCuration) ([]insight.VideoCuration, error)) error {
	return r.mutateOperations(ctx, workspace, false, func(_ *insight.OperationsState, current map[string]insight.VideoCuration) ([]insight.VideoCuration, error) {
		return mutate(current)
	})
}

func (r *VideoCurationRepository) LoadOperations(ctx context.Context, workspace string) (insight.OperationsState, error) {
	var raw []byte
	err := r.db.QueryRowContext(ctx, `SELECT state_json FROM insight_operations_agent WHERE workspace_id=$1`, workspace).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return insight.OperationsState{}, nil
	}
	if err != nil {
		return insight.OperationsState{}, err
	}
	var state insight.OperationsState
	err = json.Unmarshal(raw, &state)
	return state, err
}

func (r *VideoCurationRepository) MutateOperations(ctx context.Context, workspace string, mutate insight.OperationsMutation) error {
	return r.mutateOperations(ctx, workspace, true, mutate)
}

func (r *VideoCurationRepository) mutateOperations(ctx context.Context, workspace string, persistState bool, mutate insight.OperationsMutation) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = lockOperationsWorkspace(ctx, tx, workspace); err != nil {
		return err
	}
	var state insight.OperationsState
	if persistState {
		var raw []byte
		err = tx.QueryRowContext(ctx, `SELECT state_json FROM insight_operations_agent WHERE workspace_id=$1`, workspace).Scan(&raw)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if len(raw) > 0 {
			if err = json.Unmarshal(raw, &state); err != nil {
				return err
			}
		}
	}
	items, err := listCurations(ctx, tx, workspace)
	if err != nil {
		return err
	}
	current := map[string]insight.VideoCuration{}
	for _, item := range items {
		current[item.VideoID] = item
	}
	writes, err := mutate(&state, current)
	if err != nil {
		return err
	}
	for _, item := range writes {
		if item.WorkspaceID != workspace {
			return errors.New("curation write outside locked workspace")
		}
	}
	if err = saveCurations(ctx, tx, writes); err != nil {
		return err
	}
	if persistState {
		raw, err := json.Marshal(state)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO insight_operations_agent (workspace_id,state_json) VALUES ($1,$2) ON CONFLICT (workspace_id) DO UPDATE SET state_json=EXCLUDED.state_json`, workspace, raw); err != nil {
			return err
		}
	}
	return tx.Commit()
}
