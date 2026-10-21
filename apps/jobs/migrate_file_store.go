package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestpg"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightai"
	"github.com/nobiyou/wx_channel_insight/internal/insightaipg"
	"github.com/nobiyou/wx_channel_insight/internal/insightpg"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
	"github.com/nobiyou/wx_channel_insight/internal/monitoringpg"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/prompt"
	"github.com/nobiyou/wx_channel_insight/internal/promptpg"
	runtimecfg "github.com/nobiyou/wx_channel_insight/internal/runtime"
	"github.com/nobiyou/wx_channel_insight/internal/workspace"
	"github.com/nobiyou/wx_channel_insight/internal/workspacepg"
)

// Migration is explicit and only accepts an empty destination. Files remain the
// rollback copy; the runtime must not switch until the imported data is checked.
func runFileStoreMigration(args []string, cfg runtimecfg.Config, logf func(string, ...any)) error {
	flags := flag.NewFlagSet("migrate-file-store", flag.ContinueOnError)
	dir := flags.String("source-dir", "var", "directory containing file stores")
	apply := flags.Bool("apply", false, "import into an empty PostgreSQL database")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("unexpected migration arguments")
	}
	files := map[string]json.RawMessage{}
	for _, name := range []string{"ingest-state", "accounts", "workspace-state", "monitoring-tasks", "comment-records", "dashboard-snapshot", "rankings", "recommendations", "comment-insights", "report-summary", "daily-reports", "data-center-exclusions", "prompt-settings", "ai-service-settings", "ai-analysis-runs"} {
		data, err := os.ReadFile(filepath.Join(*dir, name+".json"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if !json.Valid(data) {
			return fmt.Errorf("invalid JSON in %s", name)
		}
		files[name] = data
	}
	if len(files["ingest-state"]) == 0 || len(files["workspace-state"]) == 0 {
		return errors.New("source must contain ingest-state.json and workspace-state.json")
	}
	var codec *insightai.SecretCodec
	if cfg.AIConfigEncryptionKey != "" {
		var err error
		codec, err = insightai.NewSecretCodec(cfg.AIConfigEncryptionKey)
		if err != nil {
			return err
		}
	}
	settings, err := decodeStore[map[string]insightai.StoredServiceSettings](files, "ai-service-settings")
	if err != nil {
		return err
	}
	for _, item := range settings {
		if (item.APIKey != "" || item.APIKeyCiphertext != "") && codec == nil {
			return errors.New("AI config encryption key is required before migrating saved API keys")
		}
		if item.APIKeyCiphertext != "" {
			if _, err := codec.Decrypt(item.APIKeyCiphertext, item.APIKeyNonce); err != nil {
				return errors.New("source AI settings cannot be decrypted with the configured encryption key")
			}
		}
	}
	logf("migration source validated files=%d apply=%t", len(files), *apply)
	if !*apply {
		return nil
	}
	client, err := postgres.NewClient(cfg.Postgres.DSN)
	if err != nil {
		return errors.New("cannot connect to migration destination PostgreSQL")
	}
	defer client.Close()
	ctx := context.Background()
	// This check is deliberately broader than the current workspace. Never merge
	// unrelated databases or overwrite their rows as a side effect of migration.
	rows, err := client.DB().QueryContext(ctx, `SELECT tablename FROM pg_tables WHERE schemaname='public' AND tablename LIKE 'insight_%'`)
	if err != nil {
		return err
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, table := range tables {
		var exists bool
		if err := client.DB().QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM "`+strings.ReplaceAll(table, `"`, `""`)+`" LIMIT 1)`).Scan(&exists); err != nil {
			return err
		}
		if exists {
			return fmt.Errorf("destination is not empty: %s; choose a new database", table)
		}
	}
	if err := postgres.RunMigrationsFromDir(client.DB(), "migrations/postgres"); err != nil {
		return err
	}
	if err := importFileStores(ctx, client, files, codec); err != nil {
		return fmt.Errorf("migration incomplete; runtime was not switched: %w", err)
	}
	logf("migration completed; original files retained; verify counts before switching runtime")
	return nil
}

func decodeStore[T any](files map[string]json.RawMessage, name string) (T, error) {
	var value T
	if data := files[name]; len(data) > 0 {
		if err := json.Unmarshal(data, &value); err != nil {
			return value, fmt.Errorf("decode %s: %w", name, err)
		}
	}
	return value, nil
}

func importFileStores(ctx context.Context, client *postgres.Client, files map[string]json.RawMessage, codec *insightai.SecretCodec) error {
	workspaces, err := decodeStore[map[string]workspace.DefaultWorkspace](files, "workspace-state")
	if err != nil {
		return err
	}
	wr, err := workspacepg.NewDefaultWorkspaceRepository(client)
	if err != nil {
		return err
	}
	for _, item := range workspaces {
		if err := wr.Save(ctx, item); err != nil {
			return err
		}
	}
	accounts, err := decodeStore[struct {
		Items []insight.Account `json:"items"`
	}](files, "accounts")
	if err != nil {
		return err
	}
	if _, err := insightpg.NewAccountRepository(client); err != nil {
		return err
	}
	for _, item := range accounts.Items {
		if err := insertMigrationRecord(ctx, client.DB(), "insight_accounts", item, map[string]string{"id": "account_id", "tags": "tags_json"}); err != nil {
			return err
		}
	}
	state, err := decodeStore[struct {
		Items     map[string][]ingest.Event                  `json:"items"`
		History   map[string][]ingest.VideoMetricObservation `json:"metricHistory"`
		Runs      map[string][]ingest.Run                    `json:"runs"`
		Operators map[string][]ingest.OperatorRun            `json:"operatorRuns"`
		Bindings  map[string][]ingest.SourceBinding          `json:"bindings"`
	}](files, "ingest-state")
	if err != nil {
		return err
	}
	ir, err := ingestpg.NewRepository(client)
	if err != nil {
		return err
	}
	for owner, items := range state.Items {
		if err := ir.AppendEvents(ctx, owner, items); err != nil {
			return err
		}
	}
	for owner, items := range state.History {
		if err := ir.AppendVideoMetricObservations(ctx, owner, items); err != nil {
			return err
		}
	}
	for owner, items := range state.Runs {
		for _, item := range items {
			if err := ir.AppendRun(ctx, owner, item); err != nil {
				return err
			}
		}
	}
	for owner, items := range state.Operators {
		for _, item := range items {
			if err := ir.AppendOperatorRun(ctx, owner, item); err != nil {
				return err
			}
		}
	}
	for owner, items := range state.Bindings {
		for _, item := range items {
			if err := ir.UpsertBinding(ctx, owner, item); err != nil {
				return err
			}
		}
	}
	comments, err := decodeStore[struct {
		Comments map[string][]ingest.RawCommentRecord    `json:"comments"`
		Syncs    map[string]ingest.VideoCommentSyncState `json:"syncs"`
	}](files, "comment-records")
	if err != nil {
		return err
	}
	cr, err := ingestpg.NewCommentRepository(client)
	if err != nil {
		return err
	}
	for owner, items := range comments.Comments {
		if err := cr.Upsert(ctx, owner, items); err != nil {
			return err
		}
	}
	for _, item := range comments.Syncs {
		if err := cr.UpsertState(ctx, item); err != nil {
			return err
		}
	}
	tasks, err := decodeStore[monitoring.MemoryState](files, "monitoring-tasks")
	if err != nil {
		return err
	}
	mr, err := monitoringpg.NewRepository(client)
	if err != nil {
		return err
	}
	for _, task := range tasks.Tasks {
		if err := mr.CreateTask(ctx, task); err != nil {
			return err
		}
	}
	for _, runs := range tasks.Runs {
		for _, run := range runs {
			data, _ := json.Marshal(run)
			var record map[string]any
			if err := json.Unmarshal(data, &record); err != nil {
				return err
			}
			delete(record, "counts")
			record["attemptedCount"] = run.Counts.Attempted
			record["succeededCount"] = run.Counts.Succeeded
			record["failedCount"] = run.Counts.Failed
			record["skippedCount"] = run.Counts.Skipped
			if err := insertMigrationRecord(ctx, client.DB(), "insight_monitoring_task_runs", record, map[string]string{"id": "run_id", "errors": "errors_json"}); err != nil {
				return err
			}
		}
	}
	return importFileReadModels(ctx, client, files, codec)
}

// SQL identifiers come only from the PostgreSQL catalog and fixed mappings;
// user content is always passed as a bound value. Unknown model fields fail.
func insertMigrationRecord(ctx context.Context, db *sql.DB, table string, item any, aliases map[string]string) error {
	data, err := json.Marshal(item)
	if err != nil {
		return err
	}
	var record map[string]any
	if err := json.Unmarshal(data, &record); err != nil {
		return err
	}
	columns, err := db.QueryContext(ctx, `SELECT column_name FROM information_schema.columns WHERE table_schema='public' AND table_name=$1`, table)
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for columns.Next() {
		var name string
		if err := columns.Scan(&name); err != nil {
			columns.Close()
			return err
		}
		allowed[name] = true
	}
	if err := columns.Err(); err != nil {
		columns.Close()
		return err
	}
	columns.Close()
	names := make([]string, 0, len(record))
	values := map[string]any{}
	for key, value := range record {
		name := aliases[key]
		if name == "" {
			name = snakeColumn(key)
		}
		if !allowed[name] {
			return fmt.Errorf("unmapped migration field %s.%s", table, key)
		}
		if strings.HasSuffix(name, "_json") {
			raw, err := json.Marshal(value)
			if err != nil {
				return err
			}
			value = string(raw)
		}
		names = append(names, name)
		values[name] = value
	}
	sort.Strings(names)
	placeholders := make([]string, len(names))
	args := make([]any, len(names))
	for i, name := range names {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = values[name]
		names[i] = `"` + name + `"`
	}
	_, err = db.ExecContext(ctx, `INSERT INTO "`+table+`" (`+strings.Join(names, ",")+`) VALUES (`+strings.Join(placeholders, ",")+`)`, args...)
	if err != nil {
		return fmt.Errorf("insert %s: %w", table, err)
	}
	return nil
}

func snakeColumn(key string) string {
	var out strings.Builder
	for i, ch := range key {
		if unicode.IsUpper(ch) {
			if i > 0 {
				out.WriteByte('_')
			}
			out.WriteRune(unicode.ToLower(ch))
		} else {
			out.WriteRune(ch)
		}
	}
	return out.String()
}

func importFileReadModels(ctx context.Context, client *postgres.Client, files map[string]json.RawMessage, codec *insightai.SecretCodec) error {
	dash, err := decodeStore[struct {
		Items map[string]insight.Dashboard `json:"items"`
	}](files, "dashboard-snapshot")
	if err != nil {
		return err
	}
	seed := dash.Items[""]
	if seed.Workspace.Name == "" {
		for _, item := range dash.Items {
			seed = item
			break
		}
	}
	dr, err := insightpg.NewDashboardRepository(client, seed)
	if err != nil {
		return err
	}
	for id, item := range dash.Items {
		if err := dr.ReplaceDashboard(ctx, id, item); err != nil {
			return err
		}
	}
	ranking, err := decodeStore[struct {
		Videos    map[string][]insight.HotVideo            `json:"hotVideos"`
		Ranks     map[string]insight.HotVideoRankings      `json:"hotVideoRankings"`
		Topics    map[string][]insight.HotTopic            `json:"hotTopics"`
		VideoMeta map[string]insight.ReadModelArtifactMeta `json:"hotVideoMeta"`
		TopicMeta map[string]insight.ReadModelArtifactMeta `json:"hotTopicMeta"`
	}](files, "rankings")
	if err != nil {
		return err
	}
	rr, err := insightpg.NewRankingRepository(client)
	if err != nil {
		return err
	}
	for id, item := range ranking.Videos {
		if err := rr.ReplaceHotVideos(ctx, id, item); err != nil {
			return err
		}
	}
	for id, item := range ranking.Ranks {
		if err := rr.ReplaceHotVideoRankings(ctx, id, item); err != nil {
			return err
		}
	}
	for id, item := range ranking.Topics {
		if err := rr.ReplaceHotTopics(ctx, id, item); err != nil {
			return err
		}
	}
	for id, item := range ranking.VideoMeta {
		if err := rr.ReplaceHotVideoMeta(ctx, id, item); err != nil {
			return err
		}
	}
	for id, item := range ranking.TopicMeta {
		if err := rr.ReplaceHotTopicMeta(ctx, id, item); err != nil {
			return err
		}
	}
	recommendations, err := decodeStore[struct {
		Items map[string]insight.RecommendationSnapshot `json:"snapshots"`
	}](files, "recommendations")
	if err != nil {
		return err
	}
	rec, err := insightpg.NewRecommendationRepository(client)
	if err != nil {
		return err
	}
	for id, item := range recommendations.Items {
		if err := rec.ReplaceRecommendations(ctx, id, item); err != nil {
			return err
		}
	}
	insights, err := decodeStore[struct {
		Items map[string][]insight.CommentInsight      `json:"items"`
		Meta  map[string]insight.ReadModelArtifactMeta `json:"meta"`
	}](files, "comment-insights")
	if err != nil {
		return err
	}
	ci, err := insightpg.NewCommentInsightRepository(client, []insight.CommentInsight{})
	if err != nil {
		return err
	}
	for id, item := range insights.Items {
		if err := ci.ReplaceCommentInsights(ctx, id, item); err != nil {
			return err
		}
	}
	for id, item := range insights.Meta {
		if err := ci.ReplaceCommentInsightMeta(ctx, id, item); err != nil {
			return err
		}
	}
	reports, err := decodeStore[struct {
		Items map[string]insight.ReportSummary         `json:"items"`
		Meta  map[string]insight.ReadModelArtifactMeta `json:"meta"`
	}](files, "report-summary")
	if err != nil {
		return err
	}
	reportSeed := reports.Items[""]
	if reportSeed.Period == "" {
		reportSeed = seed.ReportSummary
	}
	rs, err := insightpg.NewReportSummaryRepository(client, reportSeed)
	if err != nil {
		return err
	}
	for id, item := range reports.Items {
		if err := rs.ReplaceReportSummary(ctx, id, item); err != nil {
			return err
		}
	}
	for id, item := range reports.Meta {
		if err := rs.ReplaceReportSummaryMeta(ctx, id, item); err != nil {
			return err
		}
	}
	daily, err := decodeStore[struct {
		Items map[string]insight.DailyReport `json:"items"`
	}](files, "daily-reports")
	if err != nil {
		return err
	}
	dailyRepo, err := insightpg.NewDailyReportRepository(client)
	if err != nil {
		return err
	}
	for _, item := range daily.Items {
		if err := dailyRepo.Upsert(ctx, item); err != nil {
			return err
		}
	}
	exclusions, err := decodeStore[struct {
		Items []insight.DataCenterExclusion `json:"items"`
	}](files, "data-center-exclusions")
	if err != nil {
		return err
	}
	ex, err := insightpg.NewDataCenterExclusionRepository(client)
	if err != nil {
		return err
	}
	for _, item := range exclusions.Items {
		if _, err := ex.Upsert(ctx, item); err != nil {
			return err
		}
	}
	prompts, err := decodeStore[map[string]prompt.Settings](files, "prompt-settings")
	if err != nil {
		return err
	}
	pr, err := promptpg.NewRepository(client)
	if err != nil {
		return err
	}
	for owner, item := range prompts {
		if err := pr.Save(ctx, owner, item); err != nil {
			return err
		}
	}
	settings, err := decodeStore[map[string]insightai.StoredServiceSettings](files, "ai-service-settings")
	if err != nil {
		return err
	}
	ar, err := insightaipg.NewRepository(client, codec)
	if err != nil {
		return err
	}
	for owner, item := range settings {
		if err := ar.Save(ctx, owner, item); err != nil {
			return err
		}
	}
	analyses, err := decodeStore[struct {
		Items []insight.AIAnalysisRun `json:"items"`
	}](files, "ai-analysis-runs")
	if err != nil {
		return err
	}
	an, err := insightpg.NewAIAnalysisRepository(client)
	if err != nil {
		return err
	}
	for _, item := range analyses.Items {
		if err := an.Save(ctx, item); err != nil {
			return err
		}
	}
	return nil
}
