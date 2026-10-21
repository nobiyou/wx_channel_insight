package ingestpg_test

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestpg"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/testpostgres"
)

func TestCommentRepositoryQueriesPostgresAndExportsRawPayloadOnlyOnExport(t *testing.T) {
	db, _ := testpostgres.Open(t)
	ctx := context.Background()
	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatalf("new postgres client: %v", err)
	}
	repo, err := ingestpg.NewCommentRepository(client)
	if err != nil {
		t.Fatalf("new comment repository: %v", err)
	}
	hubUserID := "hub:user:comment-query-test"
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM insight_comment_records WHERE hub_user_id = $1", hubUserID)
		_, _ = db.ExecContext(ctx, "DELETE FROM insight_video_comment_sync WHERE hub_user_id = $1", hubUserID)
	})

	base := time.Date(2026, 8, 23, 1, 0, 0, 0, time.UTC)
	if err := repo.Upsert(ctx, hubUserID, []ingest.RawCommentRecord{
		{VideoObjectID: "video-1", CommentID: "old", Content: "旧评论", CreatedAt: base, CollectedAt: base, RawPayload: json.RawMessage(`{"source":"raw-old"}`)},
		{VideoObjectID: "video-1", CommentID: "new", Content: "新问题", CreatedAt: base.Add(time.Hour), CollectedAt: base.Add(time.Hour), RawPayload: json.RawMessage(`{"source":"raw-new"}`)},
		{VideoObjectID: "video-2", CommentID: "other", Content: "其他视频", CreatedAt: base.Add(2 * time.Hour), CollectedAt: base.Add(2 * time.Hour), RawPayload: json.RawMessage(`{"source":"raw-other"}`)},
	}); err != nil {
		t.Fatalf("upsert comments: %v", err)
	}

	page, err := repo.List(ctx, hubUserID, ingest.RawCommentQuery{ObjectID: "video-1", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("list comments: %v", err)
	}
	if page.Total != 2 || page.TotalPages != 1 || len(page.Items) != 2 {
		t.Fatalf("unexpected page metadata: %+v", page)
	}
	if page.Items[0].CommentID != "new" || page.Items[0].RawPayload != nil {
		t.Fatalf("expected SQL activity order without raw payload, got %+v", page.Items[0])
	}
	filtered, err := repo.List(ctx, hubUserID, ingest.RawCommentQuery{Keyword: "问题", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("search comments: %v", err)
	}
	if filtered.Total != 1 || filtered.Items[0].CommentID != "new" {
		t.Fatalf("expected SQL keyword filter, got %+v", filtered)
	}

	var output bytes.Buffer
	if err := repo.ExportJSON(ctx, hubUserID, ingest.RawCommentQuery{ObjectID: "video-1"}, &output); err != nil {
		t.Fatalf("export comments: %v", err)
	}
	var exported struct {
		Total int                       `json:"total"`
		Items []ingest.RawCommentRecord `json:"items"`
	}
	if err := json.Unmarshal(output.Bytes(), &exported); err != nil {
		t.Fatalf("decode exported comments: %v body=%s", err, output.String())
	}
	if exported.Total != 2 || len(exported.Items) != 2 || string(exported.Items[0].RawPayload) != `{"source":"raw-new"}` {
		t.Fatalf("expected export to include database raw payload, got %+v", exported)
	}
}
