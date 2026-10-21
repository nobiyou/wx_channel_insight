package ingestfile

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func TestCommentRepositoryPersistsCommentsAndSyncState(t *testing.T) {
	repo, err := NewCommentRepository(filepath.Join(t.TempDir(), "comments.json"))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	if err := repo.Upsert(context.Background(), "hub", []ingest.RawCommentRecord{{VideoObjectID: "video", CommentID: "c1", Content: "评论"}}); err != nil {
		t.Fatalf("upsert comment: %v", err)
	}
	state := ingest.VideoCommentSyncState{HubUserID: "hub", AccountID: "acct", VideoObjectID: "video", Status: ingest.VideoCommentSyncStatusSuccess}
	if err := repo.UpsertState(context.Background(), state); err != nil {
		t.Fatalf("upsert state: %v", err)
	}
	reloaded, err := NewCommentRepository(repo.path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	page, err := reloaded.List(context.Background(), "hub", ingest.RawCommentQuery{})
	if err != nil || page.Total != 1 {
		t.Fatalf("comments not persisted: %+v err=%v", page, err)
	}
	stored, found, err := reloaded.GetState(context.Background(), "hub", "acct", "video")
	if err != nil || !found || stored.Status != ingest.VideoCommentSyncStatusSuccess {
		t.Fatalf("sync state not persisted: %+v found=%v err=%v", stored, found, err)
	}
}
