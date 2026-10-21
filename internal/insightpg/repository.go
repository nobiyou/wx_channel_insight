package insightpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type DashboardRepository struct {
	client *postgres.Client
	db     *sql.DB
	seed   insight.Dashboard
}

var _ insight.DashboardRepository = (*DashboardRepository)(nil)

func NewDashboardRepository(client *postgres.Client, seed insight.Dashboard) (*DashboardRepository, error) {
	if client == nil {
		return nil, errors.New("postgres client is required")
	}
	db := client.DB()
	if err := postgres.EnsureDashboardSchema(db); err != nil {
		return nil, err
	}
	repo := &DashboardRepository{client: client, db: db, seed: seed}
	if err := repo.ensureSeed(context.Background(), ""); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *DashboardRepository) GetDashboard(ctx context.Context, workspaceID string) (insight.Dashboard, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `SELECT payload FROM insight_dashboard_snapshots WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return r.seed, nil
		}
		return insight.Dashboard{}, err
	}
	var snapshot insight.Dashboard
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return insight.Dashboard{}, err
	}
	return snapshot, nil
}

// GetDashboardReadOnly reads the bounded analysis snapshot persisted by the dashboard owner.
func (r *DashboardRepository) GetDashboardReadOnly(ctx context.Context, workspaceID string) (insight.Dashboard, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `SELECT payload FROM insight_dashboard_snapshots WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return r.seed, nil
		}
		return insight.Dashboard{}, err
	}
	var snapshot insight.Dashboard
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return insight.Dashboard{}, err
	}
	return snapshot, nil
}

func (r *DashboardRepository) GetDashboardLaunchReadiness(ctx context.Context, workspaceID string) (insight.Dashboard, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `
SELECT jsonb_build_object(
  'generatedAt', payload->'generatedAt',
  'readModel', payload->'readModel',
  'workspaceBridge', payload->'workspaceBridge'
)
FROM insight_dashboard_snapshots
WHERE snapshot_id = $1
`, snapshotKey(workspaceID))
	var payload []byte
	if err := row.Scan(&payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return r.seed, nil
		}
		return insight.Dashboard{}, err
	}
	var snapshot insight.Dashboard
	if err := json.Unmarshal(payload, &snapshot); err != nil {
		return insight.Dashboard{}, err
	}
	return snapshot, nil
}

func (r *DashboardRepository) ReplaceDashboard(ctx context.Context, workspaceID string, snapshot insight.Dashboard) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.save(ctx, workspaceID, snapshot)
}

func (r *DashboardRepository) ResetDashboard(ctx context.Context, workspaceID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.save(ctx, workspaceID, r.seed)
}

func (r *DashboardRepository) ensureSeed(ctx context.Context, workspaceID string) error {
	row := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM insight_dashboard_snapshots WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var count int
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return r.save(ctx, workspaceID, r.seed)
}

func (r *DashboardRepository) save(ctx context.Context, workspaceID string, snapshot insight.Dashboard) error {
	if snapshot.Workspace.Name == "" {
		return errors.New("dashboard workspace name is required")
	}
	payload, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_dashboard_snapshots (snapshot_id, payload)
VALUES ($1, $2::jsonb)
ON CONFLICT (snapshot_id) DO UPDATE SET
  payload = EXCLUDED.payload,
  updated_at = now()
`, snapshotKey(workspaceID), string(payload))
	return err
}

func snapshotKey(workspaceID string) string {
	if workspaceID == "" {
		return "default"
	}
	return workspaceID
}
