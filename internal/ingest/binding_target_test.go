package ingest

import "testing"

func TestBuildSourceBindingTargetKeepsSourceIdentityAccurate(t *testing.T) {
	facts := EventSummaryFacts{
		RecentEvents: []Event{
			{
				ID:        "video-other",
				Source:    RunSourceWXVideoSearch,
				EventType: "video_snapshot",
				Keyword:   "其他关键词",
				Title:     "不相关的视频",
			},
			{
				ID:              "video-match",
				Source:          RunSourceWXVideoSearch,
				EventType:       "video_snapshot",
				Keyword:         "AI 选题",
				Title:           "AI 选题如何从数据中验证",
				ChannelName:     "增长实验室",
				ChannelUsername: "growth@finder",
			},
		},
	}

	target := buildSourceBindingTarget(
		SourceBinding{Source: RunSourceWXVideoSearch, Keyword: "AI 选题"},
		SourceBindingAssessment{},
		facts,
	)
	if target == nil {
		t.Fatal("expected a target projection")
	}
	if target.Keyword != "AI 选题" || target.Title != "AI 选题如何从数据中验证" {
		t.Fatalf("expected exact keyword video target, got %+v", target)
	}
	if target.Title == "不相关的视频" {
		t.Fatalf("selected an event from another keyword: %+v", target)
	}
}

func TestBuildSourceBindingTargetMergesCommentExportAndVideoEvidence(t *testing.T) {
	commentTarget := buildSourceBindingTargetFromCommentExport(WXCommentExportFile{
		ObjectID:      "video-comment-1",
		ObjectNonceID: "nonce-comment-1",
		Title:         "评论区高频问题拆解",
		Author:        "内容研究院",
		CommentInfo: []WXCommentEntry{{
			Content:   "这个方法有没有真实数据依据？",
			LikeCount: 8,
		}},
		CountInfo: WXCommentExportCount{CommentCount: 42},
	})

	target := buildSourceBindingTarget(
		SourceBinding{Source: RunSourceWXCommentExport, ObjectID: "video-comment-1"},
		SourceBindingAssessment{Target: commentTarget},
		EventSummaryFacts{RecentEvents: []Event{{
			ID:              "comment-video-1",
			Source:          RunSourceWXCommentExport,
			EventType:       "video_snapshot",
			ObjectID:        "video-comment-1",
			ChannelUsername: "content@finder",
			PublishedAt:     commentTarget.PublishedAt,
		}, {
			ID:                    "comment-detail-1",
			Source:                RunSourceWXCommentExport,
			EventType:             "comment_snapshot",
			ObjectID:              "video-comment-1",
			RepresentativeComment: "这个方法有没有真实数据依据？",
			CommentCount:          42,
		}}},
	)
	if target == nil {
		t.Fatal("expected comment target projection")
	}
	if target.Title != "评论区高频问题拆解" || target.ChannelName != "内容研究院" || target.ChannelUsername != "content@finder" {
		t.Fatalf("expected comment video and account metadata, got %+v", target)
	}
	if target.Comment != "这个方法有没有真实数据依据？" || target.CommentCount != 42 {
		t.Fatalf("expected comment evidence, got %+v", target)
	}
}

func TestBuildSourceBindingTargetMatchesManualShareByObjectID(t *testing.T) {
	target := buildSourceBindingTarget(
		SourceBinding{Source: RunSourceWXManualShare, ObjectID: "manual-video-1"},
		SourceBindingAssessment{},
		EventSummaryFacts{RecentEvents: []Event{{
			ID:        "manual-video-2",
			Source:    RunSourceWXManualShare,
			EventType: "video_snapshot",
			ObjectID:  "manual-video-2",
			Title:     "另一个分享视频",
		}, {
			ID:          "manual-video-1",
			Source:      RunSourceWXManualShare,
			EventType:   "video_snapshot",
			ObjectID:    "manual-video-1",
			Title:       "手动分享的视频标题",
			ChannelName: "增长实验室",
		}}},
	)
	if target == nil || target.Title != "手动分享的视频标题" || target.ChannelName != "增长实验室" {
		t.Fatalf("expected exact manual-share target, got %+v", target)
	}
}
