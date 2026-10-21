package insightpg

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
)

type AIAnalysisRepository struct {
	db *sql.DB
}

var _ insight.AIAnalysisRepository = (*AIAnalysisRepository)(nil)

func NewAIAnalysisRepository(client *postgres.Client) (*AIAnalysisRepository, error) {
	if client == nil || client.DB() == nil {
		return nil, errors.New("postgres client is required")
	}
	if err := postgres.EnsureAIAnalysisSchema(client.DB()); err != nil {
		return nil, err
	}
	return &AIAnalysisRepository{db: client.DB()}, nil
}

func (r *AIAnalysisRepository) Save(ctx context.Context, run insight.AIAnalysisRun) error {
	normalized, err := insight.NormalizeAIAnalysisRun(run)
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var result any
	if len(normalized.Result) > 0 {
		result = string(normalized.Result)
	}
	_, err = r.db.ExecContext(ctx, `
INSERT INTO insight_ai_analysis_runs (
  workspace_id, run_id, module, scope_key, data_window, input_fingerprint,
  status, result, model_provider, model_name, model_attempted, prompt_version,
  config_owner_id, config_source, config_verification_status, analysis_engine,
  fallback_policy, fallback_reason, duration_ms, run_at
)
VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, '')::jsonb, $9, $10, $11, $12,
        $13, $14, $15, $16, $17, $18, $19, $20)
ON CONFLICT (workspace_id, run_id) DO UPDATE SET
  module = EXCLUDED.module,
  scope_key = EXCLUDED.scope_key,
  data_window = EXCLUDED.data_window,
  input_fingerprint = EXCLUDED.input_fingerprint,
  status = EXCLUDED.status,
  result = EXCLUDED.result,
  model_provider = EXCLUDED.model_provider,
  model_name = EXCLUDED.model_name,
  model_attempted = EXCLUDED.model_attempted,
  prompt_version = EXCLUDED.prompt_version,
  config_owner_id = EXCLUDED.config_owner_id,
  config_source = EXCLUDED.config_source,
  config_verification_status = EXCLUDED.config_verification_status,
  analysis_engine = EXCLUDED.analysis_engine,
  fallback_policy = EXCLUDED.fallback_policy,
  fallback_reason = EXCLUDED.fallback_reason,
  duration_ms = EXCLUDED.duration_ms,
  run_at = EXCLUDED.run_at,
  updated_at = now()
`, normalized.WorkspaceID, normalized.ID, normalized.Module, normalized.ScopeKey,
		normalized.Window, normalized.InputFingerprint, normalized.Status, result,
		normalized.ModelProvider, normalized.ModelName, normalized.ModelAttempted,
		normalized.PromptVersion, normalized.ConfigOwnerID, normalized.ConfigSource,
		normalized.ConfigVerificationStatus, normalized.AnalysisEngine,
		normalized.FallbackPolicy, normalized.FallbackReason, normalized.DurationMS,
		normalized.RunAt)
	return err
}

func (r *AIAnalysisRepository) List(ctx context.Context, workspaceID, module, scopeKey, window string, limit int) ([]insight.AIAnalysisRun, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	workspaceID = strings.TrimSpace(workspaceID)
	module = strings.ToLower(strings.TrimSpace(module))
	scopeKey = strings.TrimSpace(scopeKey)
	window = strings.ToLower(strings.TrimSpace(window))
	if window != "" {
		var ok bool
		window, ok = insight.NormalizeTimeWindow(window)
		if !ok {
			return nil, insight.ErrInvalidTimeWindow
		}
	}
	if limit <= 0 || limit > insight.MaxAIAnalysisHistoryLimit {
		limit = insight.MaxAIAnalysisHistoryLimit
	}
	query := `
SELECT run_id, workspace_id, module, scope_key, data_window, input_fingerprint,
       status, COALESCE(result::text, ''), model_provider, model_name,
       model_attempted, prompt_version, config_owner_id, config_source,
       config_verification_status, analysis_engine, fallback_policy,
       fallback_reason, duration_ms, run_at
FROM insight_ai_analysis_runs

WHERE workspace_id = $1`
	args := []any{workspaceID}
	if module != "" {
		query += " AND module = $" + strconv.Itoa(len(args)+1)
		args = append(args, module)
	}
	if window != "" {
		query += " AND data_window = $" + strconv.Itoa(len(args)+1)
		args = append(args, window)
	}
	if scopeKey != "" {
		query += " AND scope_key = $" + strconv.Itoa(len(args)+1)
		args = append(args, scopeKey)
	}
	query += " ORDER BY run_at DESC, run_id DESC LIMIT $" + strconv.Itoa(len(args)+1)
	args = append(args, limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]insight.AIAnalysisRun, 0, limit)
	for rows.Next() {
		item, err := scanAIAnalysisRun(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, insight.CloneAIAnalysisRun(item))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *AIAnalysisRepository) Get(ctx context.Context, workspaceID, runID string) (insight.AIAnalysisRun, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	row := r.db.QueryRowContext(ctx, `
SELECT run_id, workspace_id, module, scope_key, data_window, input_fingerprint,
       status, COALESCE(result::text, ''), model_provider, model_name,
       model_attempted, prompt_version, config_owner_id, config_source,
       config_verification_status, analysis_engine, fallback_policy,
       fallback_reason, duration_ms, run_at
FROM insight_ai_analysis_runs
WHERE workspace_id = $1 AND run_id = $2`, strings.TrimSpace(workspaceID), strings.TrimSpace(runID))
	item, err := scanAIAnalysisRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return insight.AIAnalysisRun{}, false, nil
	}
	if err != nil {
		return insight.AIAnalysisRun{}, false, err
	}
	return insight.CloneAIAnalysisRun(item), true, nil
}

type aiAnalysisRowScanner interface {
	Scan(...any) error
}

func scanAIAnalysisRun(scanner aiAnalysisRowScanner) (insight.AIAnalysisRun, error) {
	var item insight.AIAnalysisRun
	var result string
	err := scanner.Scan(
		&item.ID, &item.WorkspaceID, &item.Module, &item.ScopeKey, &item.Window,
		&item.InputFingerprint, &item.Status, &result, &item.ModelProvider,
		&item.ModelName, &item.ModelAttempted, &item.PromptVersion, &item.ConfigOwnerID,
		&item.ConfigSource, &item.ConfigVerificationStatus, &item.AnalysisEngine,
		&item.FallbackPolicy, &item.FallbackReason, &item.DurationMS, &item.RunAt,
	)
	if err != nil {
		return insight.AIAnalysisRun{}, err
	}
	if strings.TrimSpace(result) != "" {
		item.Result = json.RawMessage(result)
	}
	normalized, err := insight.NormalizeAIAnalysisRun(item)
	if err != nil {
		return insight.AIAnalysisRun{}, err
	}
	return normalized, nil
}
