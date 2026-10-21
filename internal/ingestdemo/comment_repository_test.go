package ingestdemo

import (
	"context"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func TestCommentRepositoryUpsertIsIdempotentAndPages(t *testing.T) {
	repo := NewCommentRepository()
	items := []ingest.RawCommentRecord{{HubUserID: "hub", VideoObjectID: "video", CommentID: "c1", Content: "第一条", CollectedAt: time.Now()}}
	if err := repo.Upsert(context.Background(), "hub", items); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	items[0].Content = "更新后的内容"
	if err := repo.Upsert(context.Background(), "hub", items); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	page, err := repo.List(context.Background(), "hub", ingest.RawCommentQuery{Page: 1, PageSize: 20})
	if err != nil || page.Total != 1 || page.Items[0].Content != "更新后的内容" {
		t.Fatalf("unexpected page: %+v err=%v", page, err)
	}
}
