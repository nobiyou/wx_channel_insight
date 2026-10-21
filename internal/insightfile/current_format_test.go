package insightfile_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightfile"
)

func TestRepositoriesRejectOldFormatsWithoutRewritingFiles(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload string
		open    func(string) error
	}{
		{"dashboard", `{"hotVideos":[{"id":"old"}]}`, func(path string) error {
			_, err := insightfile.NewDashboardRepository(path, insight.Dashboard{})
			return err
		}},
		{"summary", `{"headline":"old summary"}`, func(path string) error {
			_, err := insightfile.NewReportSummaryRepository(path, insight.ReportSummary{})
			return err
		}},
		{"comments-array", `[{"id":"old"}]`, func(path string) error { _, err := insightfile.NewCommentInsightRepository(path, nil); return err }},
		{"comments-dashboard", `{"commentInsights":[{"id":"old"}]}`, func(path string) error { _, err := insightfile.NewCommentInsightRepository(path, nil); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "snapshot.json")
			original := []byte(test.payload)
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := test.open(path); err == nil {
				t.Fatal("old storage format was accepted")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, original) {
				t.Fatal("rejected file was rewritten")
			}
		})
	}
}
