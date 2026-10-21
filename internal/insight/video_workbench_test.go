package insight

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func TestVideoAssessmentUsesAccountAgePeersAndProtectsUnknowns(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	rows := []DataCenterItem{}
	for i, total := range []int{5, 100, 100, 100, 500} {
		rows = append(rows, DataCenterItem{ID: fmt.Sprint(i), ChannelUsername: "a", PublishedAt: now.Add(-5 * 24 * time.Hour), ObservedAt: now, Likes: intPtr(total), Comments: intPtr(0), Favorites: intPtr(0), Forwards: intPtr(0), GrowthStatus: "available"})
	}
	rows = append(rows,
		DataCenterItem{ID: "missing", ChannelUsername: "a", PublishedAt: now.Add(-5 * 24 * time.Hour), ObservedAt: now, Likes: intPtr(0)},
		DataCenterItem{ID: "other-account", ChannelUsername: "b", PublishedAt: now.Add(-5 * 24 * time.Hour), ObservedAt: now, Likes: intPtr(1), Comments: intPtr(0), Favorites: intPtr(0), Forwards: intPtr(0)},
		DataCenterItem{ID: "young", ChannelUsername: "a", PublishedAt: now.Add(-time.Hour), ObservedAt: now, Likes: intPtr(1), Comments: intPtr(0), Favorites: intPtr(0), Forwards: intPtr(0)},
	)
	assessVideoPerformance(rows, now)
	if rows[0].Performance.Tier != "underperforming" || rows[4].Performance.Tier != "standout" || rows[4].Performance.PeerCount != 5 || *rows[4].Performance.RelativeToMedian != 5 {
		t.Fatalf("incorrect age/account comparison: %+v / %+v", rows[0].Performance, rows[4].Performance)
	}
	for _, row := range rows[5:] {
		if row.Performance.Tier != "insufficient_data" {
			t.Fatalf("unqualified video rated: %+v", row)
		}
	}
	rows[0].ObservedAt = now.Add(-48 * time.Hour)
	rows[0].PublishedAt = rows[0].ObservedAt.Add(-5 * 24 * time.Hour)
	assessVideoPerformance(rows, now)
	if rows[0].Performance.Tier == "underperforming" || rows[0].Performance.Freshness != "stale" {
		t.Fatal("stale evidence used to reject content")
	}
}

func TestVideoAssessmentNormalizesRealGrowthAndRejectsCorrections(t *testing.T) {
	now := time.Now().UTC()
	rows := []DataCenterItem{{ID: "a", GrowthStatus: "available", GrowthWindow: VideoGrowthWindow{BaselineObservedAt: now.Add(-2 * time.Hour), LatestObservedAt: now}, ObservedAt: now, Likes: intPtr(100), Comments: intPtr(10), Favorites: intPtr(10), Forwards: intPtr(0)}}
	rows[0].growthInteractions = intPtr(40)
	assessVideoPerformance(rows, now)
	if rows[0].Performance.GrowthPerHour == nil || *rows[0].Performance.GrowthPerHour != 20 {
		t.Fatalf("wrong growth speed: %+v", rows[0].Performance)
	}
	rows[0].GrowthStatus = "counter_correction"
	assessVideoPerformance(rows, now)
	if rows[0].Performance.GrowthPerHour != nil {
		t.Fatal("correction became positive growth")
	}
}

type curationTestStore struct {
	items  map[string]VideoCuration
	writes int
}

func (s *curationTestStore) List(_ context.Context, workspace string) ([]VideoCuration, error) {
	out := []VideoCuration{}
	for _, r := range s.items {
		if r.WorkspaceID == workspace {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *curationTestStore) SaveBatch(_ context.Context, items []VideoCuration) error {
	s.writes++
	for _, r := range items {
		s.items[r.WorkspaceID+"|"+r.VideoID] = r
	}
	return nil
}

func TestVideoCurationValidatesWholeBatchAndSurvivesNewCollection(t *testing.T) {
	now := time.Now().UTC()
	store := &curationTestStore{items: map[string]VideoCuration{}}
	svc := NewDataCenterService(DataCenterServiceConfig{DataProvider: dataCenterProviderStub{events: []ingest.Event{{ID: "e", ObjectID: "real", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: now, LikeCount: 10}}}, CurationRepository: store})
	q := DataCenterQuery{Entity: DataCenterEntityVideos, HubUserID: "owner"}
	status := "selected"
	if _, err := svc.CurateVideos(context.Background(), q, VideoCurationPatch{IDs: []string{"video:real", "video:absent"}, Status: &status}, "tester"); err == nil || store.writes != 0 {
		t.Fatal("invalid batch partially saved")
	}
	note := "学习开场与评论互动"
	if _, err := svc.CurateVideos(context.Background(), q, VideoCurationPatch{IDs: []string{"video:real"}, Status: &status, Note: &note}, "tester"); err != nil {
		t.Fatal(err)
	}
	q.Curation = "selected"
	p, err := svc.Query(context.Background(), q)
	if err != nil || p.Total != 1 || p.Items[0].Curation.Note != note || p.Management.Selected != 1 {
		t.Fatalf("selection not persisted: %+v %v", p, err)
	}
	status = "excluded"
	if _, err = svc.CurateVideos(context.Background(), q, VideoCurationPatch{IDs: []string{"video:real"}, Status: &status}, "tester"); err != nil {
		t.Fatal(err)
	}
	q.Curation = ""
	p, err = svc.Query(context.Background(), q)
	if err != nil || p.Total != 0 || p.Management.Excluded != 1 {
		t.Fatalf("excluded video remained in active list: %+v %v", p, err)
	}
	q.Curation = "excluded"
	p, err = svc.Query(context.Background(), q)
	if err != nil || p.Total != 1 || p.Items[0].Curation.Note != note {
		t.Fatal("exclusion lost video or note")
	}
	status = "unreviewed"
	if _, err = svc.CurateVideos(context.Background(), q, VideoCurationPatch{IDs: []string{"video:real"}, Status: &status}, "tester"); err != nil {
		t.Fatal(err)
	}
	q.Curation = ""
	p, err = svc.Query(context.Background(), q)
	if err != nil || p.Total != 1 {
		t.Fatal("restore did not recover active video")
	}
}

func TestVideoWorkbenchFiltersBeforePagination(t *testing.T) {
	now := time.Now().UTC()
	events := []ingest.Event{}
	for i := 0; i < 30; i++ {
		events = append(events, ingest.Event{ID: fmt.Sprint(i), ObjectID: fmt.Sprint(i), EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: now, LikeCount: i})
	}
	svc := NewDataCenterService(DataCenterServiceConfig{DataProvider: dataCenterProviderStub{events: events}})
	min := 20
	p, err := svc.Query(context.Background(), DataCenterQuery{Entity: DataCenterEntityVideos, MinInteractions: &min, Sort: "interactions", Direction: "desc"})
	if err != nil || p.Total != 10 || *p.Items[0].Performance.Interactions != 29 {
		t.Fatalf("wrong complete-set filtering: %+v %v", p, err)
	}
}

func TestVideoPerformanceIsStableWhenSearchingForOneVideo(t *testing.T) {
	now := time.Now().UTC()
	events := []ingest.Event{}
	for i := 0; i < 6; i++ {
		likes := 100
		title := "peer"
		if i == 5 {
			likes = 500
			title = "unique winner"
		}
		events = append(events, ingest.Event{ID: fmt.Sprint(i), ObjectID: fmt.Sprint(i), Title: title, ChannelUsername: "a", PublishedAt: now.Add(-5 * 24 * time.Hour), EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: now, LikeCount: likes})
	}
	svc := NewDataCenterService(DataCenterServiceConfig{DataProvider: dataCenterProviderStub{events: events}})
	p, err := svc.Query(context.Background(), DataCenterQuery{Entity: DataCenterEntityVideos, Keyword: "unique winner"})
	if err != nil || p.Total != 1 || p.Items[0].Performance.Tier != "standout" || p.Items[0].Performance.PeerCount != 6 {
		t.Fatalf("search changed the comparison cohort: %+v %v", p, err)
	}
}

func TestVideoEvidenceCacheReusesFactsButNeverOperatorDecisions(t *testing.T) {
	now := time.Now().UTC()
	provider := &videoHistoryDataCenterProviderStub{events: []ingest.Event{{ID: "e", ObjectID: "real", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: now, LikeCount: 20}}}
	store := &curationTestStore{items: map[string]VideoCuration{}}
	svc := NewDataCenterService(DataCenterServiceConfig{DataProvider: provider, CurationRepository: store, VideoEvidenceTTL: 30 * time.Second, Now: func() time.Time { return now }})
	q := DataCenterQuery{Entity: DataCenterEntityVideos}
	if _, err := svc.Query(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	calls := provider.fullCalls
	if _, err := svc.Query(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if provider.fullCalls != calls {
		t.Fatal("filter rebuilt unchanged evidence")
	}
	store.items["real"] = VideoCuration{VideoID: "video:real", Status: "selected"}
	q.Curation = "selected"
	p, err := svc.Query(context.Background(), q)
	if err != nil || p.Total != 1 {
		t.Fatal("cache hid an operator decision")
	}
	q.Refresh = true
	if _, err := svc.Query(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if provider.fullCalls == calls {
		t.Fatal("explicit refresh did not bypass cache")
	}
	calls = provider.fullCalls
	q.Refresh = false
	now = now.Add(time.Minute)
	if _, err := svc.Query(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if provider.fullCalls == calls {
		t.Fatal("expired evidence reused")
	}
	calls = provider.fullCalls
	q.HubUserID = "other"
	if _, err := svc.Query(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	if provider.fullCalls == calls {
		t.Fatal("cache leaked another owner's facts")
	}
}

func TestGrowthSpeedUsesQualifiedMetricsInsteadOfSameTimeEvent(t *testing.T) {
	base := time.Now().UTC().Add(-time.Hour)
	history := []DataCenterHistoryItem{
		{ID: "event", EvidenceType: "event", ObservedAt: base, Likes: intPtr(500), Comments: intPtr(0), Favorites: intPtr(0), Forwards: intPtr(0)},
		{ID: "metric-base", EvidenceType: "metric_observation", ObservedAt: base, Likes: intPtr(10), Comments: intPtr(0), Favorites: intPtr(0), Forwards: intPtr(0)},
		{ID: "metric-latest", EvidenceType: "metric_observation", ObservedAt: base.Add(time.Hour), Likes: intPtr(30), Comments: intPtr(0), Favorites: intPtr(0), Forwards: intPtr(0)},
	}
	row, _ := videoFactsInWindow(DataCenterItem{ID: "a", ObjectID: "a"}, history, time.Time{}, time.Time{})
	rows := []DataCenterItem{row}
	assessVideoPerformance(rows, base.Add(time.Hour))
	if rows[0].Performance.GrowthPerHour == nil || *rows[0].Performance.GrowthPerHour != 20 {
		t.Fatalf("unqualified event changed metric growth: %+v", rows[0].Performance)
	}
}

func TestVideoEvidenceWindowKeepsGrowthDiscoveryAndAllCurationIdentities(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	provider := &videoHistoryDataCenterProviderStub{events: []ingest.Event{
		{ID: "e-old", ObjectID: "outside", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: start.Add(-48 * time.Hour)},
	}, observations: []ingest.VideoMetricObservation{
		{ID: "a0", ObjectID: "a", Source: ingest.RunSourceWXFeedList, ObservedAt: start.Add(-time.Hour), LikeCount: 5},
		{ID: "a1", ObjectID: "a", Source: ingest.RunSourceWXFeedList, ObservedAt: start.Add(time.Hour), LikeCount: 10},
		{ID: "a2", ObjectID: "a", Source: ingest.RunSourceWXFeedList, ObservedAt: start.Add(2 * time.Hour), LikeCount: 30},
		{ID: "a3", ObjectID: "a", Source: ingest.RunSourceWXFeedList, ObservedAt: start.Add(26 * time.Hour), LikeCount: 40},
	}}
	store := &curationTestStore{items: map[string]VideoCuration{}}
	svc := NewDataCenterService(DataCenterServiceConfig{DataProvider: provider, CurationRepository: store, VideoEvidenceTTL: time.Minute})
	q := DataCenterQuery{Entity: DataCenterEntityVideos, From: start, To: start.Add(24 * time.Hour)}
	for i := 0; i < 2; i++ {
		p, err := svc.Query(context.Background(), q)
		if err != nil || p.Total != 1 || *p.Items[0].Likes != 30 || p.Items[0].Performance.GrowthPerHour == nil || *p.Items[0].Performance.GrowthPerHour != 20 || !p.Items[0].HistoryAvailable {
			t.Fatalf("window facts changed on read %d: %+v %v", i, p, err)
		}
		if p.Analysis.NewVideos == nil || *p.Analysis.NewVideos != 0 || len(p.Analysis.DiscoveryTrend) != 1 || p.Analysis.DiscoveryTrend[0].ObservedVideos != 1 {
			t.Fatalf("daily evidence changed on read %d: %+v", i, p.Analysis)
		}
	}
	calls := provider.fullCalls
	status := "selected"
	if _, err := svc.CurateVideos(context.Background(), DataCenterQuery{}, VideoCurationPatch{IDs: []string{"video:outside"}, Status: &status}, "tester"); err != nil {
		t.Fatal(err)
	}
	if calls != provider.fullCalls {
		t.Fatal("curating an out-of-window identity reread all evidence")
	}
	q.From, q.To = time.Time{}, time.Time{}
	p, err := svc.Query(context.Background(), q)
	if err != nil || p.Total != 2 {
		t.Fatalf("window cache leaked into an unbounded query: %+v %v", p, err)
	}
}

func TestCompactDiscoveryHistoryKeepsFirstEvidenceAndWindowDays(t *testing.T) {
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	history := []DataCenterHistoryItem{{ObservedAt: start.Add(-48 * time.Hour)}, {ObservedAt: start.Add(time.Hour)}, {ObservedAt: start.Add(2 * time.Hour)}, {ObservedAt: start.Add(25 * time.Hour)}, {ObservedAt: start.Add(26 * time.Hour)}, {ObservedAt: start.Add(72 * time.Hour)}}
	points := compactVideoDiscoveryHistory(history, start, start.Add(48*time.Hour))
	if len(points) != 3 || !points[0].Equal(history[0].ObservedAt) {
		t.Fatalf("want first evidence plus two observed days: %+v", points)
	}
}

func TestWorkbenchVideoOnlyBuildPreservesFullBuilderVideoFacts(t *testing.T) {
	now := time.Now().UTC()
	provider := &videoHistoryDataCenterProviderStub{events: []ingest.Event{
		{ID: "old", ObjectID: "v", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: now.Add(-time.Hour), ChannelUsername: "a", Title: "old", Keyword: "秋天", Tags: []string{"风景"}, LikeCount: 10},
		{ID: "new", ObjectID: "v", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: now, ChannelUsername: "a", Title: "new", Keyword: "旅行", Tags: []string{"美景"}, LikeCount: 20},
		{ID: "other", ObjectID: "other", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: now, LikeCount: 2},
	}, observations: []ingest.VideoMetricObservation{{ID: "metric", ObjectID: "v", Source: ingest.RunSourceWXFeedList, ObservedAt: now.Add(time.Minute), ChannelUsername: "a", Title: "metric", Keyword: "秋天", LikeCount: 30}}}
	svc := NewDataCenterService(DataCenterServiceConfig{DataProvider: provider})
	wantRows, wantHistory, err := svc.buildRowsWithRawComments(context.Background(), "", "owner", false, false)
	if err != nil {
		t.Fatal(err)
	}
	rows, history, err := svc.buildVideoEvidenceRows(context.Background(), "owner")
	if err != nil || !reflect.DeepEqual(wantRows[DataCenterEntityVideos], rows) {
		t.Fatalf("video facts differ: %v", err)
	}
	for _, row := range rows {
		if !reflect.DeepEqual(wantHistory[row.ID], history[row.ID]) {
			t.Fatalf("history differs for %s", row.ID)
		}
	}
	if len(history) != len(rows) {
		t.Fatal("non-video histories retained")
	}
}
