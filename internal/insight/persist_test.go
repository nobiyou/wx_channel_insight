package insight

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type recordingDashboardSnapshotRepository struct {
	snapshot Dashboard
}

type failingDailyReportRepository struct{}

type recordingDailyReportRepository struct {
	report DailyReport
}

func (failingDailyReportRepository) List(context.Context, string, string, int) ([]DailyReport, error) {
	return nil, errors.New("archive unavailable")
}

func (failingDailyReportRepository) Get(context.Context, string, string) (DailyReport, bool, error) {
	return DailyReport{}, false, errors.New("archive unavailable")
}

func (failingDailyReportRepository) Upsert(context.Context, DailyReport) error {
	return errors.New("archive unavailable")
}

func (r *recordingDailyReportRepository) List(context.Context, string, string, int) ([]DailyReport, error) {
	if r == nil || r.report.ID == "" {
		return nil, nil
	}
	return []DailyReport{CloneDailyReport(r.report)}, nil
}

func (r *recordingDailyReportRepository) Get(context.Context, string, string) (DailyReport, bool, error) {
	if r == nil || r.report.ID == "" {
		return DailyReport{}, false, nil
	}
	return CloneDailyReport(r.report), true, nil
}

func (r *recordingDailyReportRepository) Upsert(_ context.Context, report DailyReport) error {
	r.report = CloneDailyReport(report)
	return nil
}

func (r *recordingDashboardSnapshotRepository) GetDashboard(context.Context, string) (Dashboard, error) {
	return r.snapshot, nil
}

func (r *recordingDashboardSnapshotRepository) ReplaceDashboard(_ context.Context, _ string, snapshot Dashboard) error {
	r.snapshot = snapshot
	return nil
}

func (r *recordingDashboardSnapshotRepository) ResetDashboard(context.Context, string) error {
	r.snapshot = Dashboard{}
	return nil
}

func TestDashboardSnapshotForPrimaryStoreBoundsAnalysisRecommendations(t *testing.T) {
	repository := &recordingDashboardSnapshotRepository{}
	snapshot := Dashboard{
		AccountRecommendations: make([]AccountRecommendation, DashboardPageRecommendationLimit+1),
		VideoRecommendations:   make([]VideoRecommendation, DashboardPageRecommendationLimit+2),
		HotVideos:              make([]HotVideo, DashboardPageHotVideoLimit+1),
		HotVideoRankings: HotVideoRankings{
			HotVideoRankingHot: make([]HotVideo, DashboardPageHotVideoLimit+1),
		},
		HotVideoWindowRankings: HotVideoWindowRankings{
			TimeWindow24h: {
				HotVideoRankingHot: make([]HotVideo, DashboardPageHotVideoLimit+1),
			},
		},
		HotTopics: make([]HotTopic, DashboardPageHotTopicLimit+1),
	}
	for index := range snapshot.HotTopics {
		snapshot.HotTopics[index] = HotTopic{ID: fmt.Sprintf("topic-%d", index), Keyword: fmt.Sprintf("topic-%d", index)}
	}

	result, err := persistDashboardSnapshot(context.Background(), "ws-1", snapshot, repository, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("persist dashboard snapshot failed: %v", err)
	}
	projection := repository.snapshot
	if len(projection.AccountRecommendations) != DashboardPageRecommendationLimit {
		t.Fatalf("expected account recommendation projection to be capped at %d, got %d", DashboardPageRecommendationLimit, len(projection.AccountRecommendations))
	}
	if len(projection.VideoRecommendations) != DashboardPageRecommendationLimit {
		t.Fatalf("expected video recommendation projection to be capped at %d, got %d", DashboardPageRecommendationLimit, len(projection.VideoRecommendations))
	}
	if len(projection.HotVideos) != DashboardPageHotVideoLimit ||
		len(projection.HotVideoRankings[HotVideoRankingHot]) != DashboardPageHotVideoLimit ||
		len(projection.HotVideoWindowRankings[TimeWindow24h][HotVideoRankingHot]) != DashboardPageHotVideoLimit ||
		len(projection.HotTopics) != DashboardPageHotTopicLimit {
		t.Fatalf("expected persisted dashboard rankings to match page bounds, got videos=%d rankings=%d windowRankings=%d topics=%d", len(projection.HotVideos), len(projection.HotVideoRankings[HotVideoRankingHot]), len(projection.HotVideoWindowRankings[TimeWindow24h][HotVideoRankingHot]), len(projection.HotTopics))
	}
	if len(result.AccountRecommendations) != DashboardPageRecommendationLimit+1 ||
		len(result.VideoRecommendations) != DashboardPageRecommendationLimit+2 ||
		len(result.HotVideos) != DashboardPageHotVideoLimit+1 ||
		len(result.HotVideoRankings[HotVideoRankingHot]) != DashboardPageHotVideoLimit+1 ||
		len(result.HotVideoWindowRankings[TimeWindow24h][HotVideoRankingHot]) != DashboardPageHotVideoLimit+1 ||
		len(result.HotTopics) != DashboardPageHotTopicLimit+1 {
		t.Fatalf("expected full materialized snapshot to remain available to the caller, got accounts=%d videos=%d hotVideos=%d rankings=%d windowRankings=%d topics=%d", len(result.AccountRecommendations), len(result.VideoRecommendations), len(result.HotVideos), len(result.HotVideoRankings[HotVideoRankingHot]), len(result.HotVideoWindowRankings[TimeWindow24h][HotVideoRankingHot]), len(result.HotTopics))
	}
}

func TestPersistDashboardSnapshotDoesNotBlockWhenDailyArchiveFails(t *testing.T) {
	repository := &recordingDashboardSnapshotRepository{}
	if _, err := persistDashboardSnapshot(
		context.Background(), "ws-1", Dashboard{GeneratedAt: time.Now()}, repository, nil, nil, nil,
		failingDailyReportRepository{}, nil,
	); err != nil {
		t.Fatalf("expected archive failure to be isolated, got %v", err)
	}
}

func TestPersistDashboardSnapshotArchivesOnlyTheLatest72Hours(t *testing.T) {
	now := time.Date(2026, 8, 23, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	previous, err := NewDailyReport("ws-1", "2026-08-22", TimeWindow72h, now.Add(-24*time.Hour), Dashboard{})
	if err != nil {
		t.Fatalf("new previous daily report failed: %v", err)
	}
	dailyRepo := &recordingDailyReportRepository{report: previous}
	snapshot := Dashboard{
		GeneratedAt: now,
		HotTopics: []HotTopic{
			{ID: "fresh-topic", Keyword: "新话题", ObservedAt: now.Add(-24 * time.Hour)},
			{ID: "stale-topic", Keyword: "旧话题", ObservedAt: now.Add(-73 * time.Hour)},
		},
	}
	if _, err := persistDashboardSnapshot(context.Background(), "ws-1", snapshot, &recordingDashboardSnapshotRepository{}, nil, nil, nil, dailyRepo, nil); err != nil {
		t.Fatalf("persist dashboard snapshot failed: %v", err)
	}
	if dailyRepo.report.DataWindow != TimeWindow72h {
		t.Fatalf("expected daily archive window %q, got %q", TimeWindow72h, dailyRepo.report.DataWindow)
	}
	if len(dailyRepo.report.Snapshot.HotTopics) != 1 || dailyRepo.report.Snapshot.HotTopics[0].ID != "fresh-topic" {
		t.Fatalf("expected stale hot topics to be excluded, got %+v", dailyRepo.report.Snapshot.HotTopics)
	}
}
