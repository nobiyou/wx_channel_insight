package operator_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
	operatorflow "github.com/nobiyou/wx_channel_insight/internal/operator"
)

type operatorRunContextRepository struct {
	*ingestdemo.Repository
}

func (r *operatorRunContextRepository) AppendOperatorRun(ctx context.Context, hubUserID string, run ingest.OperatorRun) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("operator terminal state used canceled context: %w", err)
	}
	return r.Repository.AppendOperatorRun(ctx, hubUserID, run)
}

type failingOperatorRunRepository struct {
	*ingestdemo.Repository
	err error
}

func (r *failingOperatorRunRepository) AppendOperatorRun(context.Context, string, ingest.OperatorRun) error {
	return r.err
}

type stubReadModelRefresher struct{}

func TestRunCycleReportsIncompletePaginationAsPartial(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":0,"data":{"objectList":[{"id":"video"}],"lastBuffer":"next"}}`)
	}))
	defer upstream.Close()
	repo := ingestdemo.NewRepository()
	if err := repo.UpsertBinding(context.Background(), "owner", ingest.SourceBinding{Source: ingest.RunSourceWXFeedList, Username: "account", BaseURL: upstream.URL}); err != nil {
		t.Fatal(err)
	}
	ingestService := ingest.NewService(repo, ingest.ServiceConfig{DefaultHubUserID: "owner", WXFeedListClient: ingest.WXFeedListClient{BaseURL: upstream.URL, Client: upstream.Client(), MaxPages: 1}})
	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.RunCycle(context.Background(), operatorflow.Options{HubUserID: "owner", RequestedSources: map[string]struct{}{ingest.RunSourceWXFeedList: {}}})
	if err != nil || result.Status != "partial" {
		t.Fatalf("incomplete collection reported success: %+v %v", result, err)
	}
	runs, err := repo.ListOperatorRuns(context.Background(), "owner")
	if err != nil || len(runs) != 1 || runs[0].Status != "partial" || len(runs[0].Outcomes) != 1 || runs[0].Outcomes[0].Status != "partial" {
		t.Fatalf("saved outcome lost coverage: %+v %v", runs, err)
	}
}

func (stubReadModelRefresher) RefreshDashboard(context.Context, insight.DashboardQuery, string) (insight.Dashboard, error) {
	return insight.Dashboard{
		ReadModel: insight.ReadModelState{
			Mode:          "materialized",
			LatestBatchID: "batch-operator-001",
		},
		WorkspaceBridge: insight.WorkspaceBridge{
			WorkspaceID: "ws-default-hub-user-alice",
			HubUserID:   "hub:user:alice",
		},
		IngestSummary: insight.IngestSummary{
			FreshnessState: "fresh",
			LatestBatchID:  "batch-operator-001",
		},
		ReportSummary: insight.ReportSummary{
			Headline: "operator headline",
		},
	}, nil
}

func TestRunCycleMarksMissingDefaultSourceAsPartial(t *testing.T) {
	repo := ingestdemo.NewRepository()
	ingestService := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 3, 15, 0, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	commentPath := filepath.Join(t.TempDir(), "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-301",
  "objectNonceId": "nid-301",
  "title": "operator partial test",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"需要允许默认 partial","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        commentPath,
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 3, 14, 40, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert binding failed: %v", err)
	}

	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service failed: %v", err)
	}

	result, runErr := service.RunCycle(context.Background(), operatorflow.Options{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	})
	if runErr != nil {
		t.Fatalf("expected non-strict partial run to succeed, got %v", runErr)
	}
	if result.Status != "partial" || result.FinalStatus != "partial" {
		t.Fatalf("expected partial status for missing default source, got %+v", result)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Source != ingest.RunSourceWXVideoSearch {
		t.Fatalf("expected missing video search to be skipped, got %+v", result.Skipped)
	}
}

func TestRunCyclePersistsTerminalHistoryAfterCallerCancellation(t *testing.T) {
	repo := &operatorRunContextRepository{Repository: ingestdemo.NewRepository()}
	ingestService := ingest.NewService(repo, ingest.ServiceConfig{DefaultHubUserID: "hub:user:canceled"})
	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, runErr := service.RunCycle(ctx, operatorflow.Options{HubUserID: "hub:user:canceled"})
	if runErr == nil || errors.Is(runErr, context.Canceled) {
		t.Fatalf("expected the original no-bindings execution error, got result=%#v err=%v", result, runErr)
	}
	runs, err := repo.ListOperatorRuns(context.Background(), "hub:user:canceled")
	if err != nil {
		t.Fatalf("list operator runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID == "" || runs[0].Status != "failed" {
		t.Fatalf("expected one persisted failed operator run, got %#v", runs)
	}
}

func TestRunCyclePreservesExecutionAndHistoryFailuresWithoutExposingRunID(t *testing.T) {
	persistErr := errors.New("operator history unavailable")
	repo := &failingOperatorRunRepository{Repository: ingestdemo.NewRepository(), err: persistErr}
	ingestService := ingest.NewService(repo, ingest.ServiceConfig{DefaultHubUserID: "hub:user:persist-failure"})
	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service: %v", err)
	}

	result, runErr := service.RunCycle(context.Background(), operatorflow.Options{HubUserID: "hub:user:persist-failure"})
	var executionErr operatorflow.ExecutionError
	if !errors.As(runErr, &executionErr) || !errors.Is(runErr, persistErr) {
		t.Fatalf("expected execution and persistence failures, got %v", runErr)
	}
	if result.OperatorRunID != "" {
		t.Fatalf("failed persistence exposed nonexistent operator run id %q", result.OperatorRunID)
	}
	if executionErr.Err == nil || !strings.Contains(runErr.Error(), executionErr.Err.Error()) {
		t.Fatalf("execution cause was lost: %v", runErr)
	}
}

func TestRunCycleStrictFailsWhenRequestedSourceIsMissing(t *testing.T) {
	repo := ingestdemo.NewRepository()
	ingestService := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 3, 15, 20, 0, 0, time.FixedZone("CST", 8*3600))
		},
	})

	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service failed: %v", err)
	}

	requested, err := operatorflow.NormalizeRequestedSources([]string{ingest.RunSourceWXCommentExport})
	if err != nil {
		t.Fatalf("normalize requested sources failed: %v", err)
	}

	result, runErr := service.RunCycle(context.Background(), operatorflow.Options{
		HubUserID:        "hub:user:alice",
		DeviceCount:      2,
		RequestedSources: requested,
		Strict:           true,
	})
	if runErr == nil {
		t.Fatal("expected strict run to fail when requested source is missing")
	}
	if result.Status != "failed" || result.FinalStatus != "failed" {
		t.Fatalf("expected strict run to end failed, got %+v", result)
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("expected no successful sync payload when no sources can run, got %+v", result.Skipped)
	}
}

func TestRunCycleRecordsPerSourceOutcomes(t *testing.T) {
	repo := ingestdemo.NewRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code": 0,
			"message": "success",
			"data": {
				"object": [{
					"id": "feed-operator-1",
					"objectNonceId": "nonce-operator-1",
					"contact": { "nickname": "Operator Feed Channel" },
					"objectDesc": {
						"description": "operator outcome source tracking sample",
						"media": [{ "commentCount": 44 }]
					}
				}]
			}
		}`))
	}))
	defer server.Close()

	ingestService := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 3, 15, 40, 0, 0, time.FixedZone("CST", 8*3600))
		},
		WXFeedListClient: ingest.WXFeedListClient{
			BaseURL: server.URL,
		},
	})

	commentPath := filepath.Join(t.TempDir(), "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-302",
  "objectNonceId": "nid-302",
  "title": "operator outcomes test",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"需要记录每个来源的结果","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        commentPath,
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 3, 15, 10, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert comment binding failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXFeedList,
		BaseURL:     server.URL,
		Username:    "gh_operator_source_tracking",
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 3, 15, 20, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert feed binding failed: %v", err)
	}

	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service failed: %v", err)
	}

	if _, err := service.RunCycle(context.Background(), operatorflow.Options{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	}); err != nil {
		t.Fatalf("expected non-strict partial run success, got %v", err)
	}

	runs, err := ingestService.ListOperatorRuns(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list operator runs failed: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one operator run, got %+v", runs)
	}
	if len(runs[0].Outcomes) < 2 {
		t.Fatalf("expected per-source outcomes to be recorded, got %+v", runs[0])
	}
	outcomesBySource := map[string]ingest.OperatorRunOutcome{}
	for _, item := range runs[0].Outcomes {
		outcomesBySource[item.Source] = item
	}
	for _, source := range []string{ingest.RunSourceWXCommentExport, ingest.RunSourceWXFeedList} {
		outcome, ok := outcomesBySource[source]
		if !ok {
			t.Fatalf("expected operator outcome for %s, got %+v", source, runs[0].Outcomes)
		}
		if outcome.Status != "success" {
			t.Fatalf("expected %s outcome success, got %+v", source, outcome)
		}
	}
	if outcome := outcomesBySource[ingest.RunSourceWXCommentExport]; outcome.Summary == "" || outcome.Summary == outcomesBySource[ingest.RunSourceWXFeedList].Summary {
		t.Fatalf("expected source-scoped operator outcome summaries, got %+v", runs[0].Outcomes)
	}
	if outcome := outcomesBySource[ingest.RunSourceWXCommentExport]; !strings.Contains(outcome.Summary, "批次 ") {
		t.Fatalf("expected comment export outcome summary to expose its own batch metadata, got %+v", outcome)
	}
}

func TestRunCycleRefreshesSavedManualShareBinding(t *testing.T) {
	repo := ingestdemo.NewRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/feed/profile" || r.URL.Query().Get("url") != "https://weixin.qq.com/sph/manual-monitor-video" {
			t.Fatalf("unexpected manual share request: %s?%s", r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
  "code": 0,
  "data": {"object": {
    "id": "manual-monitor-video",
    "objectNonceId": "manual-monitor-nonce",
    "createtime": 1785901095,
    "likeCount": 31,
    "commentCount": 7,
    "favCount": 5,
    "forwardCount": 2,
    "contact": {"username": "manual@finder", "nickname": "手动监控账号"},
    "objectDesc": {"description": "默认任务持续采集的视频", "media": [{"thumbUrl": "https://cdn.example.test/manual-monitor.jpg"}]}
  }}
}`))
	}))
	defer server.Close()

	now := time.Date(2026, 8, 11, 11, 0, 0, 0, time.FixedZone("CST", 8*3600))
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:        ingest.RunSourceWXManualShare,
		BaseURL:       server.URL,
		ShareURL:      "https://weixin.qq.com/sph/manual-monitor-video",
		ObjectID:      "manual-monitor-video",
		ObjectNonceID: "manual-monitor-nonce",
		DeviceCount:   1,
		UpdatedAt:     now,
	}); err != nil {
		t.Fatalf("upsert manual share binding failed: %v", err)
	}

	ingestService := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now:              func() time.Time { return now },
		WXFeedListClient: ingest.WXFeedListClient{BaseURL: server.URL, Client: server.Client()},
	})
	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service failed: %v", err)
	}

	result, err := service.RunCycle(context.Background(), operatorflow.Options{
		HubUserID:   "hub:user:alice",
		DeviceCount: 1,
	})
	if err != nil {
		t.Fatalf("refresh manual share binding: %v", err)
	}
	if len(result.Imports) != 1 || result.Imports[0].Source != ingest.RunSourceWXManualShare {
		t.Fatalf("expected manual share import in operator result, got %+v", result)
	}

	observations, err := ingestService.ListVideoMetricObservations(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:alice"})
	if err != nil {
		t.Fatalf("list manual share observations: %v", err)
	}
	if len(observations) != 1 || observations[0].ObjectID != "manual-monitor-video" || observations[0].LikeCount != 31 {
		t.Fatalf("expected refreshed manual share observation, got %+v", observations)
	}
}

func TestRunCycleImportsAllSavedFeedTargets(t *testing.T) {
	repo := ingestdemo.NewRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channels/contact/feed/list" {
			http.NotFound(w, r)
			return
		}
		username := r.URL.Query().Get("username")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"message":"success","data":{"object":[{"id":%q,"objectNonceId":"nonce-multi-target","contact":{"username":%q,"nickname":%q},"objectDesc":{"description":%q,"media":[{"likeCount":12,"commentCount":4,"favCount":3,"forwardCount":2}]}}]}}`,
			"video-"+username, username, "账号-"+username, "视频-"+username)
	}))
	defer server.Close()

	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXFeedList,
		BaseURL:     server.URL,
		Username:    "account-one@finder",
		DeviceCount: 1,
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("upsert first feed binding failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXFeedList,
		BaseURL:     server.URL,
		Username:    "account-two@finder",
		DeviceCount: 1,
		UpdatedAt:   now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("upsert second feed binding failed: %v", err)
	}

	ingestService := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now:              func() time.Time { return now.Add(2 * time.Minute) },
		WXFeedListClient: ingest.WXFeedListClient{BaseURL: server.URL, Client: server.Client()},
	})
	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service failed: %v", err)
	}

	result, err := service.RunCycle(context.Background(), operatorflow.Options{
		HubUserID:   "hub:user:alice",
		DeviceCount: 1,
	})
	if err != nil {
		t.Fatalf("expected multi-target cycle to complete, got %v", err)
	}
	if result.AttemptedSources != 2 || len(result.Imports) != 2 {
		t.Fatalf("expected both feed targets to run, got attempted=%d imports=%d result=%+v", result.AttemptedSources, len(result.Imports), result)
	}

	events, err := ingestService.ListEvents(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:alice"})
	if err != nil {
		t.Fatalf("list imported events failed: %v", err)
	}
	seen := map[string]bool{}
	for _, event := range events {
		seen[event.ChannelName] = true
	}
	if !seen["账号-account-one@finder"] || !seen["账号-account-two@finder"] {
		t.Fatalf("expected events from both monitored accounts, got %+v", events)
	}
}

func TestRunCycleLimitsAutomaticCommentExportToLatestBinding(t *testing.T) {
	repo := ingestdemo.NewRepository()
	commentDir := t.TempDir()
	writeExport := func(path, objectID, title string) {
		t.Helper()
		payload := fmt.Sprintf(`{
  "objectId": %q,
  "objectNonceId": %q,
  "title": %q,
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"评论内容","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": {"commentCount": 1},
  "originalCommentCount": 1,
  "savedAt": "2026-08-06T10:00:00+08:00",
  "source": "wx_channel_comment_export"
}`, objectID, objectID+"-nonce", title)
		if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
			t.Fatalf("write comment export %s: %v", path, err)
		}
	}
	olderPath := filepath.Join(commentDir, "comment-export-older.json")
	latestPath := filepath.Join(commentDir, "comment-export-latest.json")
	writeExport(olderPath, "oid-older", "older comment export")
	writeExport(latestPath, "oid-latest", "latest comment export")

	now := time.Date(2026, 8, 6, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	for index, path := range []string{olderPath, latestPath} {
		if err := repo.UpsertBinding(context.Background(), "hub:user:auto-comments", ingest.SourceBinding{
			Source: ingest.RunSourceWXCommentExport, Path: path, DeviceCount: 1,
			UpdatedAt: now.Add(time.Duration(index) * time.Minute),
		}); err != nil {
			t.Fatalf("upsert comment binding %s: %v", path, err)
		}
	}

	ingestService := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now:              func() time.Time { return now.Add(2 * time.Minute) },
	})
	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service failed: %v", err)
	}

	result, err := service.RunCycle(context.Background(), operatorflow.Options{
		HubUserID: "hub:user:auto-comments", DeviceCount: 1,
		TriggerMode: ingest.OperatorTriggerScheduled,
	})
	if err != nil {
		t.Fatalf("automatic comment cycle failed: %v", err)
	}
	if result.AttemptedSources != 1 || len(result.Imports) != 1 {
		t.Fatalf("expected only latest comment binding to run, got attempted=%d imports=%d result=%+v", result.AttemptedSources, len(result.Imports), result)
	}
	events, err := ingestService.ListEvents(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:auto-comments"})
	if err != nil {
		t.Fatalf("list automatic comment events: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected events from latest comment export, got %+v", events)
	}
	for _, event := range events {
		if event.ObjectID != "oid-latest" {
			t.Fatalf("expected only latest comment export events, got %+v", events)
		}
	}
}

func TestRunCycleExplicitCommentTargetOverridesAutomaticLimit(t *testing.T) {
	repo := ingestdemo.NewRepository()
	commentDir := t.TempDir()
	writeExport := func(path, objectID string) {
		t.Helper()
		payload := fmt.Sprintf(`{
  "objectId": %q,
  "objectNonceId": %q,
  "title": "selected comment export",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"指定目标评论","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": {"commentCount": 1},
  "originalCommentCount": 1,
  "savedAt": "2026-08-06T10:00:00+08:00",
  "source": "wx_channel_comment_export"
}`, objectID, objectID+"-nonce")
		if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
			t.Fatalf("write selected comment export %s: %v", path, err)
		}
	}
	olderPath := filepath.Join(commentDir, "comment-export-older.json")
	latestPath := filepath.Join(commentDir, "comment-export-latest.json")
	writeExport(olderPath, "oid-explicit-older")
	writeExport(latestPath, "oid-explicit-latest")

	now := time.Date(2026, 8, 6, 11, 0, 0, 0, time.FixedZone("CST", 8*3600))
	for index, path := range []string{olderPath, latestPath} {
		if err := repo.UpsertBinding(context.Background(), "hub:user:explicit-comment", ingest.SourceBinding{
			Source: ingest.RunSourceWXCommentExport, Path: path, DeviceCount: 1,
			UpdatedAt: now.Add(time.Duration(index) * time.Minute),
		}); err != nil {
			t.Fatalf("upsert explicit comment binding %s: %v", path, err)
		}
	}

	ingestService := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now:              func() time.Time { return now.Add(2 * time.Minute) },
	})
	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service failed: %v", err)
	}

	result, err := service.RunCycle(context.Background(), operatorflow.Options{
		HubUserID: "hub:user:explicit-comment", DeviceCount: 1,
		TriggerMode: ingest.OperatorTriggerLoop,
		TargetRefs: []monitoring.BindingRef{{
			Source: ingest.RunSourceWXCommentExport, TargetKey: olderPath,
		}},
	})
	if err != nil {
		t.Fatalf("explicit comment target cycle failed: %v", err)
	}
	if result.AttemptedSources != 1 || len(result.Imports) != 1 {
		t.Fatalf("expected one explicitly selected comment binding, got attempted=%d imports=%d result=%+v", result.AttemptedSources, len(result.Imports), result)
	}
	events, err := ingestService.ListEvents(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:explicit-comment"})
	if err != nil {
		t.Fatalf("list explicit comment events: %v", err)
	}
	if len(events) == 0 {
		t.Fatalf("expected events from explicitly selected comment export, got %+v", events)
	}
	for _, event := range events {
		if event.ObjectID != "oid-explicit-older" {
			t.Fatalf("expected explicitly selected older comment export events, got %+v", events)
		}
	}
}

func TestRunCycleImportsOnlySelectedTargetAndReturnsOperatorRunID(t *testing.T) {
	repo := ingestdemo.NewRepository()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := r.URL.Query().Get("username")
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"message":"success","data":{"object":[{"id":%q,"contact":{"username":%q,"nickname":%q},"objectDesc":{"description":"selected video","media":[{"likeCount":8,"commentCount":2,"favCount":1,"forwardCount":1}]}}]}}`,
			"selected-"+username, username, "账号-"+username)
	}))
	defer server.Close()
	now := time.Date(2026, 8, 6, 12, 0, 0, 0, time.FixedZone("CST", 8*3600))
	for index, username := range []string{"account-one@finder", "account-two@finder"} {
		if err := repo.UpsertBinding(context.Background(), "hub:user:selected", ingest.SourceBinding{
			Source: ingest.RunSourceWXFeedList, BaseURL: server.URL, Username: username,
			DeviceCount: 1, UpdatedAt: now.Add(time.Duration(index) * time.Minute),
		}); err != nil {
			t.Fatalf("upsert selected binding %s: %v", username, err)
		}
	}
	ingestService := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now:              func() time.Time { return now.Add(2 * time.Minute) },
		WXFeedListClient: ingest.WXFeedListClient{BaseURL: server.URL, Client: server.Client()},
	})
	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service failed: %v", err)
	}

	result, err := service.RunCycle(context.Background(), operatorflow.Options{
		HubUserID: "hub:user:selected", DeviceCount: 1,
		TargetRefs: []monitoring.BindingRef{{
			Source: ingest.RunSourceWXFeedList, TargetKey: "account-two@finder",
		}},
	})
	if err != nil {
		t.Fatalf("selected target cycle failed: %v", err)
	}
	if result.AttemptedSources != 1 || len(result.Imports) != 1 || result.OperatorRunID == "" {
		t.Fatalf("expected one selected import and operator run id, got %+v", result)
	}
	events, err := ingestService.ListEvents(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:selected"})
	if err != nil {
		t.Fatalf("list selected events: %v", err)
	}
	if len(events) != 1 || events[0].ChannelUsername != "account-two@finder" {
		t.Fatalf("expected only selected account event, got %+v", events)
	}
	runs, err := ingestService.ListOperatorRuns(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:selected"})
	if err != nil {
		t.Fatalf("list operator runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ID != result.OperatorRunID {
		t.Fatalf("expected result operator run id to match history, result=%q runs=%+v", result.OperatorRunID, runs)
	}
}

func TestRunCycleRefreshesSavedCommentBindingViaWXChannelExport(t *testing.T) {
	repo := ingestdemo.NewRepository()
	commentDir := t.TempDir()
	stalePath := filepath.Join(commentDir, "comment-export-stale.json")
	if err := os.WriteFile(stalePath, []byte(`{
  "objectId": "oid-operator-refresh-1",
  "objectNonceId": "nid-operator-refresh-1",
  "title": "operator stale comment",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"旧评论","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-03T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`), 0o644); err != nil {
		t.Fatalf("write stale export failed: %v", err)
	}
	refreshedPath := filepath.Join(commentDir, "comment-export-refreshed.json")
	if err := os.WriteFile(refreshedPath, []byte(`{
  "objectId": "oid-operator-refresh-1",
  "objectNonceId": "nid-operator-refresh-1",
  "title": "operator refreshed comment",
  "author": "Insight Test",
  "commentInfo": [
    {"commentId":"c1","content":"新评论一","nickname":"用户A","username":"user_a","createtime":"1719914400","likeCount":3,"expandCommentCount":0,"levelTwoComment":[]},
    {"commentId":"c2","content":"新评论二","nickname":"用户B","username":"user_b","createtime":"1719918000","likeCount":2,"expandCommentCount":0,"levelTwoComment":[]}
  ],
  "countInfo": { "commentCount": 2 },
  "originalCommentCount": 2,
  "savedAt": "2026-07-05T10:05:00+08:00",
  "source": "finderGetCommentList"
}`), 0o644); err != nil {
		t.Fatalf("write refreshed export failed: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/channels/feed/comment/export":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(fmt.Sprintf(`{"code":0,"message":"success","data":{"object_id":"oid-operator-refresh-1","saved_path":%q,"relative_path":"comment_data/2026-07-05/comment-export-refreshed.json","title":"operator refreshed comment","author":"Insight Test","source":"finderGetCommentList"}}`, refreshedPath)))
		case "/api/channels/contact/feed/list":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"object":[{"id":"feed-operator-refresh-1","objectNonceId":"nonce-operator-refresh-1","contact":{"nickname":"Operator Feed Channel"},"objectDesc":{"description":"feed item","media":[{"commentCount":21}]}}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        stalePath,
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 5, 10, 0, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert comment binding failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXFeedList,
		BaseURL:     server.URL,
		Username:    "gh_operator_refresh",
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 5, 10, 1, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert feed binding failed: %v", err)
	}

	ingestService := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		Now: func() time.Time {
			return time.Date(2026, 7, 5, 10, 6, 0, 0, time.FixedZone("CST", 8*3600))
		},
		WXFeedListClient: ingest.WXFeedListClient{BaseURL: server.URL, Client: server.Client()},
		WXCommentClient:  ingest.WXCommentExportClient{BaseURL: server.URL, Client: server.Client()},
	})
	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service failed: %v", err)
	}

	result, err := service.RunCycle(context.Background(), operatorflow.Options{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	})
	if err != nil {
		t.Fatalf("expected operator cycle success, got %v", err)
	}
	if result.Status != "ok" {
		t.Fatalf("expected operator cycle ok status, got %+v", result)
	}
	binding, ok := findCommentBindingBySource(mustListBindings(t, repo), ingest.RunSourceWXCommentExport)
	if !ok {
		t.Fatalf("expected saved comment binding after operator refresh")
	}
	if filepath.Clean(binding.Path) != filepath.Clean(refreshedPath) {
		t.Fatalf("expected operator to promote refreshed comment path, got %+v", binding)
	}
}

func mustListBindings(t *testing.T, repo *ingestdemo.Repository) []ingest.SourceBinding {
	t.Helper()
	bindings, err := repo.ListBindings(context.Background(), "hub:user:alice")
	if err != nil {
		t.Fatalf("list bindings failed: %v", err)
	}
	return bindings
}

func findCommentBindingBySource(bindings []ingest.SourceBinding, source string) (ingest.SourceBinding, bool) {
	for _, binding := range bindings {
		if strings.TrimSpace(binding.Source) == strings.TrimSpace(source) {
			return binding, true
		}
	}
	return ingest.SourceBinding{}, false
}

func TestRunCycleSkipsFeedListWhenLiveBindingStateIsBlocked(t *testing.T) {
	repo := ingestdemo.NewRepository()
	feedListCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ws/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"clients":0}`))
			return
		case "/api/channels/contact/feed/list":
			feedListCalls++
			t.Fatalf("feed list endpoint should not be called when live binding state is blocked")
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ingestService := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "hub:user:demo",
		EnableLiveChecks: true,
		Now: func() time.Time {
			return time.Date(2026, 7, 4, 10, 15, 0, 0, time.FixedZone("CST", 8*3600))
		},
		WXFeedListClient: ingest.WXFeedListClient{
			Client: server.Client(),
		},
	})

	commentPath := filepath.Join(t.TempDir(), "comment-export.json")
	if err := os.WriteFile(commentPath, []byte(`{
  "objectId": "oid-303",
  "objectNonceId": "nid-303",
  "title": "operator blocked feed list test",
  "author": "Insight Test",
  "commentInfo": [{"commentId":"c1","content":"当 feed list 页面客户端未就绪时应该跳过而不是失败","nickname":"用户A","username":"user_a","createtime":"1719828000","likeCount":1,"expandCommentCount":0,"levelTwoComment":[]}],
  "countInfo": { "commentCount": 1 },
  "originalCommentCount": 1,
  "savedAt": "2026-07-04T10:14:00+08:00",
  "source": "wx_channel_comment_export"
}`), 0o644); err != nil {
		t.Fatalf("write export file failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXCommentExport,
		Path:        commentPath,
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 4, 10, 5, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert comment binding failed: %v", err)
	}
	if err := repo.UpsertBinding(context.Background(), "hub:user:alice", ingest.SourceBinding{
		Source:      ingest.RunSourceWXFeedList,
		BaseURL:     server.URL,
		Username:    "gh_blocked_feed_list",
		DeviceCount: 2,
		UpdatedAt:   time.Date(2026, 7, 4, 10, 6, 0, 0, time.FixedZone("CST", 8*3600)),
	}); err != nil {
		t.Fatalf("upsert feed binding failed: %v", err)
	}

	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service failed: %v", err)
	}

	result, runErr := service.RunCycle(context.Background(), operatorflow.Options{
		HubUserID:   "hub:user:alice",
		DeviceCount: 2,
	})
	if runErr != nil {
		t.Fatalf("expected blocked feed list to downgrade to partial success, got %v", runErr)
	}
	if result.Status != "partial" || result.FinalStatus != "partial" {
		t.Fatalf("expected partial result, got %+v", result)
	}
	if len(result.Failures) != 0 {
		t.Fatalf("expected no failures when feed list is preflight-blocked, got %+v", result.Failures)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Source != ingest.RunSourceWXFeedList {
		t.Fatalf("expected feed list skipped, got %+v", result.Skipped)
	}
	if !strings.Contains(result.Skipped[0].Reason, "feed list has no ready page client") {
		t.Fatalf("expected live skipped reason, got %+v", result.Skipped)
	}
	if feedListCalls != 0 {
		t.Fatalf("expected zero feed list fetch calls, got %d", feedListCalls)
	}

	runs, err := ingestService.ListOperatorRuns(context.Background(), ingest.SummaryQuery{
		HubUserID: "hub:user:alice",
	})
	if err != nil {
		t.Fatalf("list operator runs failed: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one operator run, got %+v", runs)
	}
	outcomesBySource := map[string]ingest.OperatorRunOutcome{}
	for _, item := range runs[0].Outcomes {
		outcomesBySource[item.Source] = item
	}
	if outcome, ok := outcomesBySource[ingest.RunSourceWXFeedList]; !ok || outcome.Status != "skipped" || !strings.Contains(outcome.Summary, "feed list has no ready page client") {
		t.Fatalf("expected skipped feed list outcome, got %+v", runs[0].Outcomes)
	}
}
