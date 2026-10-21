package insight

import (
	"context"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

type dataCenterProviderStub struct {
	events       []ingest.Event
	observations []ingest.VideoMetricObservation
}

type dataCenterRawCommentProviderStub struct {
	comments []ingest.RawCommentRecord
	queries  []ingest.RawCommentQuery
}

func (s *dataCenterRawCommentProviderStub) List(_ context.Context, _ string, query ingest.RawCommentQuery) (ingest.RawCommentPage, error) {
	s.queries = append(s.queries, query)
	return ingest.PageRawComments(s.comments, query), nil
}

func (s dataCenterProviderStub) ListEvents(context.Context, ingest.SummaryQuery) ([]ingest.Event, error) {
	return append([]ingest.Event(nil), s.events...), nil
}

func (s dataCenterProviderStub) ListVideoMetricObservations(context.Context, ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error) {
	return append([]ingest.VideoMetricObservation(nil), s.observations...), nil
}

type latestDataCenterProviderStub struct {
	dataCenterProviderStub
	latest      []ingest.VideoMetricObservation
	latestCalls int
}

type videoHistoryDataCenterProviderStub struct {
	events       []ingest.Event
	observations []ingest.VideoMetricObservation
	fullCalls    int
	historyCalls int
}

func (s *videoHistoryDataCenterProviderStub) ListEvents(context.Context, ingest.SummaryQuery) ([]ingest.Event, error) {
	s.fullCalls++
	return append([]ingest.Event(nil), s.events...), nil
}

func (s *videoHistoryDataCenterProviderStub) ListVideoMetricObservations(context.Context, ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error) {
	s.fullCalls++
	return append([]ingest.VideoMetricObservation(nil), s.observations...), nil
}

func (s *videoHistoryDataCenterProviderStub) ListVideoHistory(context.Context, ingest.SummaryQuery, string) ([]ingest.Event, []ingest.VideoMetricObservation, error) {
	s.historyCalls++
	return append([]ingest.Event(nil), s.events...), append([]ingest.VideoMetricObservation(nil), s.observations...), nil
}

func (s *latestDataCenterProviderStub) ListLatestVideoMetricObservations(context.Context, ingest.SummaryQuery) ([]ingest.VideoMetricObservation, error) {
	s.latestCalls++
	return append([]ingest.VideoMetricObservation(nil), s.latest...), nil
}

type dataCenterExclusionStub struct {
	items map[string]DataCenterExclusion
}

func newDataCenterExclusionStub() *dataCenterExclusionStub {
	return &dataCenterExclusionStub{items: map[string]DataCenterExclusion{}}
}

func (s *dataCenterExclusionStub) List(_ context.Context, workspaceID string) ([]DataCenterExclusion, error) {
	result := make([]DataCenterExclusion, 0, len(s.items))
	for _, item := range s.items {
		if item.WorkspaceID == workspaceID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (s *dataCenterExclusionStub) Upsert(_ context.Context, exclusion DataCenterExclusion) (DataCenterExclusion, error) {
	key := exclusionKey(exclusion.Entity, exclusion.EntityID)
	if existing, ok := s.items[key]; ok {
		return existing, nil
	}
	s.items[key] = exclusion
	return exclusion, nil
}

func (s *dataCenterExclusionStub) Restore(_ context.Context, workspaceID string, entity DataCenterEntity, entityID string) (bool, error) {
	key := exclusionKey(entity, entityID)
	item, ok := s.items[key]
	if !ok || item.WorkspaceID != workspaceID {
		return false, nil
	}
	delete(s.items, key)
	return true, nil
}

func TestDataCenterQueryBuildsFullCurrentViews(t *testing.T) {
	observedAt := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	provider := dataCenterProviderStub{events: []ingest.Event{
		{ID: "v1-old", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: observedAt, ObjectID: "v1", Title: "旧标题", ChannelUsername: "@monitored", ChannelName: "已监控账号", LikeCount: 2, CommentCount: 3, FavCount: 1, ForwardCount: 1, VideoCount: 1, Keyword: "全量话题"},
		{ID: "v1-new", EventType: "video_snapshot", Source: ingest.RunSourceWXFeedList, CollectedAt: observedAt.Add(time.Hour), ObjectID: "v1", Title: "新标题", ChannelUsername: "@monitored", ChannelName: "已监控账号", LikeCount: 5, CommentCount: 4, FavCount: 2, ForwardCount: 2, VideoCount: 1, Keyword: "全量话题"},
		{ID: "v2", EventType: "video_snapshot", Source: ingest.RunSourceWXVideoSearch, CollectedAt: observedAt.Add(2 * time.Hour), ObjectID: "v2", Title: "搜索视频", ChannelUsername: "@unmonitored", ChannelName: "未监控账号", LikeCount: 8, CommentCount: 1, FavCount: 0, ForwardCount: 0, VideoCount: 1, Tags: []string{"长尾话题"}},
		{ID: "topic-1", EventType: "topic_cluster", Source: ingest.RunSourceWXVideoSearch, CollectedAt: observedAt.Add(3 * time.Hour), Keyword: "长尾话题", TopicCount: 12, VideoCount: 3, CommentCount: 7, ChannelUsername: "@unmonitored"},
		{ID: "comment-1", EventType: "comment_snapshot", Source: ingest.RunSourceWXCommentExport, CollectedAt: observedAt.Add(4 * time.Hour), Keyword: "评论主题", RepresentativeComment: "请问如何落地", CommentCount: 20, ChannelUsername: "@unmonitored"},
	}}
	service := NewDataCenterService(DataCenterServiceConfig{DataProvider: provider})

	videos, err := service.Query(context.Background(), DataCenterQuery{HubUserID: "hub:user:test", Entity: DataCenterEntityVideos})
	if err != nil {
		t.Fatalf("query videos: %v", err)
	}
	if videos.Total != 2 || len(videos.Items) != 2 {
		t.Fatalf("expected two deduplicated videos, got total=%d items=%d", videos.Total, len(videos.Items))
	}
	if videos.Summary.Videos != 2 || videos.Summary.Topics != 0 || videos.Summary.Accounts != 0 || videos.Summary.Comments != 0 {
		t.Fatalf("expected filtered video summary videos=2 and no global counts, got %+v", videos.Summary)
	}
	var latest DataCenterItem
	for _, item := range videos.Items {
		if item.ID == "video:v1" {
			latest = item
		}
	}
	if latest.Title != "新标题" || latest.Likes == nil || *latest.Likes != 5 {
		t.Fatalf("expected latest v1 evidence, got %+v", latest)
	}

	accounts, err := service.Query(context.Background(), DataCenterQuery{HubUserID: "hub:user:test", Entity: DataCenterEntityAccounts})
	if err != nil {
		t.Fatalf("query accounts: %v", err)
	}
	if accounts.Total != 2 {
		t.Fatalf("expected monitored and unmonitored accounts, got total=%d items=%+v", accounts.Total, accounts.Items)
	}

	topics, err := service.Query(context.Background(), DataCenterQuery{HubUserID: "hub:user:test", Entity: DataCenterEntityTopics})
	if err != nil {
		t.Fatalf("query topics: %v", err)
	}
	if topics.Total < 2 {
		t.Fatalf("expected full topic set, got total=%d items=%+v", topics.Total, topics.Items)
	}

	comments, err := service.Query(context.Background(), DataCenterQuery{HubUserID: "hub:user:test", Entity: DataCenterEntityComments})
	if err != nil {
		t.Fatalf("query comments: %v", err)
	}
	if comments.Total != 1 || comments.Items[0].DataBasis == "" {
		t.Fatalf("expected structured comment insight, got %+v", comments.Items)
	}
}

func TestDataCenterCommentsUseRawRepositoryPagination(t *testing.T) {
	base := time.Date(2026, 8, 23, 1, 0, 0, 0, time.UTC)
	raw := &dataCenterRawCommentProviderStub{}
	for index := 0; index < 25; index++ {
		raw.comments = append(raw.comments, ingest.RawCommentRecord{
			VideoObjectID: "video-1", CommentID: "comment-" + string(rune('a'+index)),
			ChannelUsername: "@target", Content: "真实评论", CollectedAt: base.Add(time.Duration(index) * time.Minute),
			Source: ingest.RunSourceWXCommentExport,
		})
	}
	service := NewDataCenterService(DataCenterServiceConfig{
		DataProvider:        dataCenterProviderStub{},
		RawCommentProvider:  raw,
		ExclusionRepository: newDataCenterExclusionStub(),
	})

	page, err := service.Query(context.Background(), DataCenterQuery{
		HubUserID: "hub:user:test", Entity: DataCenterEntityComments, Account: "@target", Page: 2, PageSize: 20,
	})
	if err != nil {
		t.Fatalf("query raw comments: %v", err)
	}
	if page.Total != 25 || page.TotalPages != 2 || page.Page != 2 || len(page.Items) != 5 {
		t.Fatalf("expected repository pagination, got %+v", page)
	}
	if page.Summary.Comments != 25 || len(raw.queries) != 2 {
		t.Fatalf("expected comment summary and bounded repository queries, summary=%+v queries=%d", page.Summary, len(raw.queries))
	}
	if page.Items[0].Entity != DataCenterEntityComments || page.Items[0].ChannelUsername != "@target" {
		t.Fatalf("expected raw comment row, got %+v", page.Items[0])
	}
}

func TestDataCenterAccountRowsUseLatestMetricProvider(t *testing.T) {
	base := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	older := ingest.VideoMetricObservation{
		ID: "v1-old", ObservedAt: base, ObjectID: "v1", Source: ingest.RunSourceWXFeedList,
		ChannelUsername: "@account", ChannelName: "账号", LikeCount: 2,
	}
	latest := older
	latest.ID = "v1-new"
	latest.ObservedAt = base.Add(time.Hour)
	latest.LikeCount = 9
	provider := &latestDataCenterProviderStub{
		dataCenterProviderStub: dataCenterProviderStub{
			observations: []ingest.VideoMetricObservation{older, latest},
		},
		latest: []ingest.VideoMetricObservation{latest},
	}
	service := NewDataCenterService(DataCenterServiceConfig{DataProvider: provider})

	rows, err := service.ListAccountRows(context.Background(), DataCenterQuery{
		HubUserID: "hub:user:test", Entity: DataCenterEntityAccounts,
	})
	if err != nil {
		t.Fatalf("list account rows: %v", err)
	}
	if provider.latestCalls != 1 {
		t.Fatalf("expected account rows to use latest metric provider once, got %d", provider.latestCalls)
	}
	if len(rows) != 1 || rows[0].Likes == nil || *rows[0].Likes != 9 {
		t.Fatalf("expected latest account metric, got %+v", rows)
	}
}

func TestDataCenterVideoQueryRetainsFullEvidenceForCurrentViews(t *testing.T) {
	base := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	older := ingest.VideoMetricObservation{
		ID: "v1-old", ObservedAt: base, ObjectID: "v1", Source: ingest.RunSourceWXFeedList,
		Title: "旧标题", ChannelUsername: "@account", LikeCount: 2,
	}
	latest := older
	latest.ID = "v1-new"
	latest.ObservedAt = base.Add(time.Hour)
	latest.Title = "新标题"
	latest.LikeCount = 9
	provider := &latestDataCenterProviderStub{
		dataCenterProviderStub: dataCenterProviderStub{observations: []ingest.VideoMetricObservation{older, latest}},
		latest:                 []ingest.VideoMetricObservation{latest},
	}
	service := NewDataCenterService(DataCenterServiceConfig{DataProvider: provider})

	page, err := service.Query(context.Background(), DataCenterQuery{HubUserID: "hub:user:test", Entity: DataCenterEntityVideos})
	if err != nil {
		t.Fatalf("query current data-center view: %v", err)
	}
	if provider.latestCalls != 0 {
		t.Fatalf("video current view needs full relation and growth evidence, got latestCalls=%d", provider.latestCalls)
	}
	if len(page.Items) != 1 || page.Items[0].Title != "新标题" || page.Items[0].Likes == nil || *page.Items[0].Likes != 9 {
		t.Fatalf("expected latest video snapshot in current view, got %+v", page.Items)
	}
}

func TestVideoCommentRefreshKeepsPublicationAndMetricEvidence(t *testing.T) {
	observed := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	published := observed.Add(-24 * time.Hour)
	provider := dataCenterProviderStub{
		observations: []ingest.VideoMetricObservation{{ID: "metric-v1", ObjectID: "v1", Source: ingest.RunSourceWXFeedList, ObservedAt: observed, PublishedAt: published, LikeCount: 9}},
		events:       []ingest.Event{{ID: "comment-video-v1", EventType: "video_snapshot", ObjectID: "v1", Source: ingest.RunSourceWXCommentExport, CollectedAt: observed.Add(time.Minute), LikeCount: 2}},
	}
	service := NewDataCenterService(DataCenterServiceConfig{DataProvider: provider})
	page, err := service.Query(context.Background(), DataCenterQuery{HubUserID: "hub:user:test", Entity: DataCenterEntityVideos})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("unexpected videos: %+v", page.Items)
	}
	item := page.Items[0]
	if !item.PublishedAt.Equal(published) || !item.ObservedAt.Equal(observed) || item.Likes == nil || *item.Likes != 9 || item.ObservationCount != 1 || item.DataBasis != "来自追加式 VideoMetricObservation。" {
		t.Fatalf("comment refresh replaced metric evidence: %+v", item)
	}
}

func TestDataCenterHistoryAndSoftDeleteRestore(t *testing.T) {
	base := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	provider := dataCenterProviderStub{events: []ingest.Event{
		{ID: "v1-a", EventType: "video_snapshot", Source: "source-a", CollectedAt: base, ObjectID: "v1", Title: "A", ChannelName: "账号", LikeCount: 1, VideoCount: 1},
		{ID: "v1-b", EventType: "video_snapshot", Source: "source-b", CollectedAt: base.Add(time.Hour), ObjectID: "v1", Title: "B", ChannelName: "账号", LikeCount: 2, VideoCount: 1},
	}}
	exclusions := newDataCenterExclusionStub()
	service := NewDataCenterService(DataCenterServiceConfig{
		DataProvider:        provider,
		ExclusionRepository: exclusions,
		Now:                 func() time.Time { return base.Add(2 * time.Hour) },
	})
	query := DataCenterQuery{HubUserID: "hub:user:test", Entity: DataCenterEntityVideos}

	history, err := service.History(context.Background(), query, "video:v1")
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	if history.Total != 2 || !history.HistoryAvailable || history.Items[0].Source != "source-b" {
		t.Fatalf("expected two newest-first observations, got %+v", history)
	}

	deleted, err := service.SoftDelete(context.Background(), query, "video:v1", "operator:test")
	if err != nil {
		t.Fatalf("soft delete: %v", err)
	}
	if deleted.DeletionMode != "soft" || deleted.DeletedBy != "operator:test" {
		t.Fatalf("unexpected deletion response: %+v", deleted)
	}
	current, err := service.Query(context.Background(), query)
	if err != nil {
		t.Fatalf("query after delete: %v", err)
	}
	if current.Total != 0 {
		t.Fatalf("expected deleted row to leave current view, got %+v", current.Items)
	}
	if current.Summary.Videos != 0 {
		t.Fatalf("expected deleted video to leave summary, got %+v", current.Summary)
	}
	archived, err := service.Query(context.Background(), DataCenterQuery{HubUserID: "hub:user:test", Entity: DataCenterEntityVideos, IncludeArchived: true})
	if err != nil {
		t.Fatalf("query archived: %v", err)
	}
	if archived.Total != 1 || archived.Items[0].Status != dataCenterStatusArchived {
		t.Fatalf("expected archived row, got %+v", archived.Items)
	}
	if _, err := service.SoftDelete(context.Background(), query, "video:v1", "operator:other"); err != nil {
		t.Fatalf("idempotent soft delete: %v", err)
	}
	if _, err := service.Restore(context.Background(), query, "video:v1"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored, err := service.Query(context.Background(), query)
	if err != nil {
		t.Fatalf("query after restore: %v", err)
	}
	if restored.Total != 1 || restored.Items[0].Status != dataCenterStatusCurrent {
		t.Fatalf("expected restored row, got %+v", restored.Items)
	}
	if restored.Summary.Videos != 1 {
		t.Fatalf("expected restored video in summary, got %+v", restored.Summary)
	}
}

func TestDataCenterVideoHistoryUsesBoundedProviderAndKeepsPagination(t *testing.T) {
	base := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	provider := &videoHistoryDataCenterProviderStub{}
	for index := 0; index < 25; index++ {
		provider.events = append(provider.events, ingest.Event{
			ID: "event-" + string(rune('a'+index)), EventType: "video_snapshot", Source: "source-events",
			CollectedAt: base.Add(time.Duration(index) * time.Minute), ObjectID: "video-1", Title: "视频事件",
			ChannelName: "账号", VideoCount: 1,
		})
	}
	service := NewDataCenterService(DataCenterServiceConfig{DataProvider: provider})

	page, err := service.History(context.Background(), DataCenterQuery{
		HubUserID: "hub:user:test", Entity: DataCenterEntityVideos, Page: 2, PageSize: 20,
	}, "video:video-1")
	if err != nil {
		t.Fatalf("query bounded video history: %v", err)
	}
	if provider.historyCalls != 1 || provider.fullCalls != 0 {
		t.Fatalf("expected one bounded history call and no full scans, history=%d full=%d", provider.historyCalls, provider.fullCalls)
	}
	if page.Total != 25 || page.TotalPages != 2 || page.Page != 2 || len(page.Items) != 5 {
		t.Fatalf("expected paginated video history, got %+v", page)
	}
}

func TestDataCenterTopicSourceAccountsAreAggregatedAcrossObservations(t *testing.T) {
	base := time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)
	service := NewDataCenterService(DataCenterServiceConfig{DataProvider: dataCenterProviderStub{
		observations: []ingest.VideoMetricObservation{
			{ID: "observation-a", ObjectID: "video-a", Keyword: "同一话题", ChannelName: "账号 A", ObservedAt: base},
			{ID: "observation-b", ObjectID: "video-b", Keyword: "同一话题", ChannelName: "账号 B", ObservedAt: base.Add(time.Hour)},
			{ID: "observation-c", ObjectID: "video-c", Keyword: "同一话题", ChannelName: "账号 A", ObservedAt: base.Add(2 * time.Hour)},
		},
	}})

	page, err := service.Query(context.Background(), DataCenterQuery{HubUserID: "hub:user:test", Entity: DataCenterEntityTopics})
	if err != nil {
		t.Fatalf("query topics: %v", err)
	}
	if page.Total != 1 || len(page.Items[0].SourceAccounts) != 2 {
		t.Fatalf("expected two distinct source accounts, got %+v", page.Items)
	}
	if page.Items[0].SourceAccounts[0] != "账号 A" || page.Items[0].SourceAccounts[1] != "账号 B" {
		t.Fatalf("expected sorted source accounts, got %v", page.Items[0].SourceAccounts)
	}
}

func TestDataCenterIgnoresRemovedManualTopics(t *testing.T) {
	observedAt := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	service := NewDataCenterService(DataCenterServiceConfig{DataProvider: dataCenterProviderStub{
		events: []ingest.Event{{
			ID: "manual-topic:AI", EventType: "topic_manual", Source: "manual",
			CollectedAt: observedAt, Keyword: "AI",
		}},
	}})

	page, err := service.Query(context.Background(), DataCenterQuery{HubUserID: "hub:user:test", Entity: DataCenterEntityTopics})
	if err != nil {
		t.Fatalf("query manual topic: %v", err)
	}
	if page.Total != 0 {
		t.Fatalf("removed manual topics must not enter data center analysis, got %+v", page.Items)
	}
}

func TestDataCenterQueryRejectsUnsupportedPageSizeAndSort(t *testing.T) {
	service := NewDataCenterService(DataCenterServiceConfig{DataProvider: dataCenterProviderStub{}})
	_, err := service.Query(context.Background(), DataCenterQuery{Entity: DataCenterEntityVideos, PageSize: 10})
	if err == nil {
		t.Fatal("expected page size validation error")
	}
	_, err = service.Query(context.Background(), DataCenterQuery{Entity: DataCenterEntityComments, Sort: "likes"})
	if err == nil {
		t.Fatal("expected entity sort validation error")
	}
}

func TestDataCenterQueryMatchesRawCommentFields(t *testing.T) {
	item := DataCenterItem{
		ID:             "comment:video-1:comment-1",
		Entity:         DataCenterEntityComments,
		CommentID:      "comment-1",
		AuthorNickname: "小明",
		Content:        "这个功能什么时候开放？",
		IPRegion:       "广东",
	}

	for _, query := range []DataCenterQuery{
		{Keyword: "什么时候开放"},
		{Keyword: "小明"},
		{Keyword: "comment-1"},
		{Keyword: "广东"},
	} {
		if !matchesDataCenterQuery(item, query) {
			t.Fatalf("expected raw comment query to match %+v", query)
		}
	}
}
