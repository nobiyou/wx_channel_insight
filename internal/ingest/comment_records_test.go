package ingest

import (
	"testing"
	"time"
)

func TestFlattenWXCommentExportPreservesReplyRelationAndStableIDs(t *testing.T) {
	collectedAt := time.Date(2026, 8, 16, 10, 0, 0, 0, time.UTC)
	payload := WXCommentExportFile{
		ObjectID:      "video-1",
		ObjectNonceID: "nonce-1",
		Title:         "视频标题",
		Author:        "账号昵称",
		Source:        "feed_comment_list",
		CommentInfo: []WXCommentEntry{{
			CommentID:  "comment-1",
			Username:   "user-1",
			Nickname:   "用户一",
			Content:    "顶层评论",
			Createtime: "1723802400",
			LevelTwoComment: []WXCommentEntry{{
				ReplyCommentID: "reply-1",
				Content:        "回复评论",
			}},
		}},
	}
	items := FlattenWXCommentExport("hub-1", "acct-1", payload, collectedAt)
	if len(items) != 2 {
		t.Fatalf("expected top-level and reply, got %d", len(items))
	}
	if items[0].CommentID != "comment-1" || items[0].ParentCommentID != "" || items[0].AccountID != "acct-1" {
		t.Fatalf("unexpected top-level record: %+v", items[0])
	}
	if items[1].CommentID != "reply-1" || items[1].ParentCommentID != "comment-1" || items[1].VideoObjectNonceID != "nonce-1" {
		t.Fatalf("unexpected reply record: %+v", items[1])
	}
	if items[0].ID != "video-1\x00comment-1" || items[1].ID != "video-1\x00reply-1" {
		t.Fatalf("unexpected stable IDs: %+v", items)
	}
}

func TestFlattenWXCommentExportSkipsEntriesWithoutCommentID(t *testing.T) {
	items := FlattenWXCommentExport("hub-1", "", WXCommentExportFile{
		ObjectID: "video-1",
		CommentInfo: []WXCommentEntry{{
			Content: "没有评论 ID",
		}},
	}, time.Now())
	if len(items) != 0 {
		t.Fatalf("expected unstable comment to be skipped, got %+v", items)
	}
}

func TestPageRawCommentsKeepsRepliesAfterTheirParent(t *testing.T) {
	base := time.Date(2026, 8, 17, 8, 0, 0, 0, time.UTC)
	items := []RawCommentRecord{
		{ID: "video-1\x00root", VideoObjectID: "video-1", CommentID: "root", Content: "主评论", CreatedAt: base.Add(2 * time.Hour), CollectedAt: base.Add(3 * time.Hour)},
		{ID: "video-1\x00reply", VideoObjectID: "video-1", CommentID: "reply", ParentCommentID: "root", Content: "回复", CreatedAt: base.Add(4 * time.Hour), CollectedAt: base.Add(5 * time.Hour)},
		{ID: "video-1\x00nested", VideoObjectID: "video-1", CommentID: "nested", ParentCommentID: "reply", Content: "二级回复", CreatedAt: base.Add(5 * time.Hour), CollectedAt: base.Add(6 * time.Hour)},
		{ID: "video-1\x00other", VideoObjectID: "video-1", CommentID: "other", Content: "另一条主评论", CreatedAt: base.Add(time.Hour), CollectedAt: base.Add(7 * time.Hour)},
	}

	page := PageRawComments(items, RawCommentQuery{Page: 1, PageSize: 20})
	if len(page.Items) != len(items) {
		t.Fatalf("expected all comments, got %+v", page)
	}
	want := []string{"root", "reply", "nested", "other"}
	for i, commentID := range want {
		if page.Items[i].CommentID != commentID {
			t.Fatalf("expected threaded order %v, got %+v", want, page.Items)
		}
	}
}
