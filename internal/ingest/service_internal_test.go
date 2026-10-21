package ingest

import "testing"

func TestNormalizeSummaryFailureMessageUsesLatestFailedSource(t *testing.T) {
	reasonsBySource := map[string]string{
		RunSourceWXCommentExport: "comment export has no ready page client：请先切回目标视频",
		RunSourceWXFeedList:      "feed list has no ready page client：请先检查 127.0.0.1:2025 代理",
	}

	message := "wx_channel 已启动，但当前没有就绪的视频号客户端。请稍后重试"
	got := normalizeSummaryFailureMessage(message, RunSourceWXFeedList, reasonsBySource)

	if got != reasonsBySource[RunSourceWXFeedList] {
		t.Fatalf("expected latest failed source to win, got %q", got)
	}
}

func TestNormalizeSummaryFailureMessageKeepsConcreteFailureWhenSourceDoesNotMatch(t *testing.T) {
	reasonsBySource := map[string]string{
		RunSourceWXCommentExport: "comment export has no ready page client：请先切回目标视频",
		RunSourceWXFeedList:      "feed list has no ready page client：请先检查 127.0.0.1:2025 代理",
	}

	message := "wx_channel feed list returned no usable videos"
	got := normalizeSummaryFailureMessage(message, RunSourceWXCommentExport, reasonsBySource)

	if got != message {
		t.Fatalf("expected concrete non-binding failure to stay unchanged, got %q", got)
	}
}
