package ingest_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	ingest "github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
)

func TestImportWXCommentExportPersistsRawCommentsWithoutChangingInsightEvents(t *testing.T) {
	exportPath := filepath.Join(t.TempDir(), "comment.json")
	payload := []byte(`{
  "objectId":"video-raw-1",
  "objectNonceId":"nonce-raw-1",
  "title":"原始评论视频",
  "author":"账号一",
  "source":"finderGetCommentList",
  "commentInfo":[
    {"commentId":"comment-1","username":"user-1","nickname":"用户一","content":"顶层评论","createtime":"1723802400","likeCount":2,"expandCommentCount":1,
     "levelTwoComment":[{"replyCommentId":"reply-1","content":"回复内容"}]}
  ]
}`)
	if err := os.WriteFile(exportPath, payload, 0o644); err != nil {
		t.Fatalf("write export: %v", err)
	}
	rawRepo := ingestdemo.NewCommentRepository()
	service := ingest.NewService(ingestdemo.NewRepository(), ingest.ServiceConfig{
		DefaultHubUserID:           "hub-raw",
		RawCommentRepository:       rawRepo,
		VideoCommentSyncRepository: rawRepo,
	})
	if _, err := service.ImportWXCommentExportForAccount(context.Background(), ingest.SummaryQuery{HubUserID: "hub-raw"}, "acct-raw", exportPath); err != nil {
		t.Fatalf("import comment export: %v", err)
	}
	page, err := service.ListRawComments(context.Background(), ingest.SummaryQuery{HubUserID: "hub-raw"}, ingest.RawCommentQuery{AccountID: "acct-raw"})
	if err != nil {
		t.Fatalf("list raw comments: %v", err)
	}
	if page.Total != 2 || page.Items[0].AccountID != "acct-raw" {
		t.Fatalf("expected two account raw comments, got %+v", page)
	}
	foundReply := false
	for _, item := range page.Items {
		if item.CommentID == "reply-1" && item.ParentCommentID == "comment-1" {
			foundReply = true
		}
	}
	if !foundReply {
		t.Fatalf("expected reply parent relation, got %+v", page.Items)
	}
}
