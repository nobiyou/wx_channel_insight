package insightpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type ReportSummaryRepository struct {
	client *postgres.Client
	db     *sql.DB
	seed   insight.ReportSummary
}

type reportSummarySnapshotPayload struct {
	Item insight.ReportSummary         `json:"item"`
	Meta insight.ReadModelArtifactMeta `json:"meta"`
}

var _ insight.ReportSummaryRepository = (*ReportSummaryRepository)(nil)

func NewReportSummaryRepository(client *postgres.Client, seed insight.ReportSummary) (*ReportSummaryRepository, error) {
	if client == nil {
		return nil, errors.New("postgres client is required")
	}
	db := client.DB()
	if err := postgres.EnsureReportSummarySchema(db); err != nil {
		return nil, err
	}
	repo := &ReportSummaryRepository{client: client, db: db, seed: seed}
	if err := repo.ensureSeed(context.Background(), ""); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *ReportSummaryRepository) GetReportSummary(ctx context.Context, workspaceID string) (insight.ReportSummary, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `SELECT payload, updated_at FROM insight_report_summaries WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var payload []byte
	var updatedAt time.Time
	if err := row.Scan(&payload, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return r.seed, false, nil
		}
		return insight.ReportSummary{}, false, err
	}
	summary, _, err := parseReportSummarySnapshotPayload(payload, updatedAt)
	if err != nil {
		return insight.ReportSummary{}, true, err
	}
	return summary, true, nil
}

func (r *ReportSummaryRepository) ReplaceReportSummary(ctx context.Context, workspaceID string, summary insight.ReportSummary) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.save(ctx, workspaceID, summary)
}

func (r *ReportSummaryRepository) ResetReportSummary(ctx context.Context, workspaceID string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return r.save(ctx, workspaceID, r.seed)
}

func (r *ReportSummaryRepository) GetMeta(ctx context.Context, workspaceID string) (insight.ReadModelArtifactMeta, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `SELECT payload, updated_at FROM insight_report_summaries WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var payload []byte
	var updatedAt time.Time
	if err := row.Scan(&payload, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return insight.ReadModelArtifactMeta{}, false, nil
		}
		return insight.ReadModelArtifactMeta{}, false, err
	}
	_, meta, err := parseReportSummarySnapshotPayload(payload, updatedAt)
	if err != nil {
		return insight.ReadModelArtifactMeta{}, true, err
	}
	return meta, true, nil
}

func (r *ReportSummaryRepository) ReplaceReportSummaryMeta(ctx context.Context, workspaceID string, meta insight.ReadModelArtifactMeta) error {
	if ctx == nil {
		ctx = context.Background()
	}
	item, found, err := r.GetReportSummary(ctx, workspaceID)
	if err != nil {
		return err
	}
	if !found {
		item = r.seed
	}
	payload, err := json.Marshal(reportSummarySnapshotPayload{
		Item: item,
		Meta: meta,
	})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_report_summaries (snapshot_id, payload)
VALUES ($1, $2::jsonb)
ON CONFLICT (snapshot_id) DO UPDATE SET
  payload = EXCLUDED.payload,
  updated_at = now()
`, snapshotKey(workspaceID), string(payload))
	return err
}

func (r *ReportSummaryRepository) ensureSeed(ctx context.Context, workspaceID string) error {
	row := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM insight_report_summaries WHERE snapshot_id = $1`, snapshotKey(workspaceID))
	var count int
	if err := row.Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	return r.save(ctx, workspaceID, r.seed)
}

func (r *ReportSummaryRepository) save(ctx context.Context, workspaceID string, summary insight.ReportSummary) error {
	if err := insight.ValidateReportSummary(summary); err != nil {
		return err
	}
	meta, _, err := r.GetMeta(ctx, workspaceID)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(reportSummarySnapshotPayload{
		Item: summary,
		Meta: meta,
	})
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_report_summaries (snapshot_id, payload)
VALUES ($1, $2::jsonb)
ON CONFLICT (snapshot_id) DO UPDATE SET
  payload = EXCLUDED.payload,
  updated_at = now()
`, snapshotKey(workspaceID), string(payload))
	return err
}

func parseReportSummarySnapshotPayload(payload []byte, updatedAt time.Time) (insight.ReportSummary, insight.ReadModelArtifactMeta, error) {
	var wrapped reportSummarySnapshotPayload
	if err := json.Unmarshal(payload, &wrapped); err != nil {
		return insight.ReportSummary{}, insight.ReadModelArtifactMeta{}, err
	}
	if err := insight.ValidateReportSummary(wrapped.Item); err != nil {
		return insight.ReportSummary{}, insight.ReadModelArtifactMeta{}, err
	}
	if wrapped.Meta.MaterializedAt.IsZero() {
		wrapped.Meta.MaterializedAt = updatedAt
	}
	return wrapped.Item, wrapped.Meta, nil
}
