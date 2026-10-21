package insight

import (
	"context"
	"sort"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

// Keep one immutable collection-window snapshot, not a second copy of the full
// observation library. Operator decisions are read separately on every query.
type dataCenterVideoEvidence struct {
	workspaceID string
	hubUserID   string
	from, to    time.Time
	builtAt     time.Time
	expiresAt   time.Time
	rows        []DataCenterItem
	discoveries map[string][]time.Time
	knownIDs    map[string]bool
}

func (s *DataCenterService) videoEvidenceRows(ctx context.Context, workspaceID, hubUserID string, from, to time.Time, refresh bool) (*dataCenterVideoEvidence, error) {
	requestedAt := time.Now()
	s.videoEvidenceMu.Lock()
	defer s.videoEvidenceMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cache := s.videoEvidence
	if s.videoEvidenceTTL > 0 && cache != nil && cache.workspaceID == workspaceID && cache.hubUserID == hubUserID && cache.from.Equal(from) && cache.to.Equal(to) && s.now().Before(cache.expiresAt) && (!refresh || cache.builtAt.After(requestedAt)) {
		return cache, nil
	}
	rows, histories, err := s.buildVideoEvidenceRows(ctx, hubUserID)
	if err != nil {
		return nil, err
	}
	cache = &dataCenterVideoEvidence{workspaceID: workspaceID, hubUserID: hubUserID, from: from, to: to, rows: []DataCenterItem{}, discoveries: map[string][]time.Time{}, knownIDs: map[string]bool{}}
	for _, row := range rows {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cache.knownIDs[row.ID] = true
		history := histories[row.ID]
		row, inWindow := videoFactsInWindow(row, history, from, to)
		if !inWindow {
			continue
		}
		cache.rows = append(cache.rows, row)
		cache.discoveries[row.ID] = compactVideoDiscoveryHistory(history, from, to)
	}
	cache.builtAt, cache.expiresAt = time.Now(), s.now().Add(s.videoEvidenceTTL)
	if s.videoEvidenceTTL > 0 {
		s.videoEvidence = cache
	}
	return cache, nil
}

// The workbench only needs videos. Building account and topic histories for
// every tag multiplies both allocations and latency without changing its facts.
func (s *DataCenterService) buildVideoEvidenceRows(ctx context.Context, hubUserID string) ([]DataCenterItem, map[string][]DataCenterHistoryItem, error) {
	if s == nil || s.dataProvider == nil {
		return nil, nil, ErrDataCenterServiceUnavailable
	}
	events, err := s.dataProvider.ListEvents(ctx, ingest.SummaryQuery{HubUserID: hubUserID})
	if err != nil {
		return nil, nil, err
	}
	values := map[string]DataCenterItem{}
	histories := map[string][]DataCenterHistoryItem{}
	add := func(item DataCenterItem, evidenceType, id string) {
		if item.ID == "" {
			return
		}
		values[item.ID] = mergeDataCenterItems(values[item.ID], item)
		histories[item.ID] = append(histories[item.ID], historyFromItem(item, evidenceType, id))
	}
	for _, event := range events {
		if event.EventType == "video_snapshot" {
			add(eventItemFromIngestEvent(event), "event", event.ID)
		}
	}
	observations, err := s.dataProvider.ListVideoMetricObservations(ctx, ingest.SummaryQuery{HubUserID: hubUserID})
	if err != nil {
		return nil, nil, err
	}
	for _, observation := range observations {
		add(itemFromVideoObservation(observation), "metric_observation", observation.ID)
	}
	rows := make([]DataCenterItem, 0, len(values))
	for id, row := range values {
		history := histories[id]
		sort.SliceStable(history, func(i, j int) bool { return history[i].ObservedAt.After(history[j].ObservedAt) })
		histories[id] = dedupeDataCenterHistory(history)
		row.HistoryAvailable = len(histories[id]) > 0
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, histories, nil
}

func (s *DataCenterService) videoEvidenceIDs(ctx context.Context, workspaceID, hubUserID string) (map[string]bool, error) {
	s.videoEvidenceMu.Lock()
	cache := s.videoEvidence
	if s.videoEvidenceTTL > 0 && cache != nil && cache.workspaceID == workspaceID && cache.hubUserID == hubUserID && s.now().Before(cache.expiresAt) {
		s.videoEvidenceMu.Unlock()
		return cache.knownIDs, nil
	}
	s.videoEvidenceMu.Unlock()
	cache, err := s.videoEvidenceRows(ctx, workspaceID, hubUserID, time.Time{}, time.Time{}, false)
	if err != nil {
		return nil, err
	}
	return cache.knownIDs, nil
}

// Keep first discovery and distinct observed days as real timestamps. The first
// point may be outside the window; otherwise old videos could appear newly found.
func compactVideoDiscoveryHistory(history []DataCenterHistoryItem, from, to time.Time) []time.Time {
	var first time.Time
	days := map[string]time.Time{}
	zone := time.FixedZone("Asia/Shanghai", 8*60*60)
	for _, point := range history {
		at := point.ObservedAt
		if at.IsZero() {
			continue
		}
		if first.IsZero() || at.Before(first) {
			first = at
		}
		if !from.IsZero() && at.Before(from) || !to.IsZero() && at.After(to) {
			continue
		}
		day := at.In(zone).Format("2006-01-02")
		if previous, ok := days[day]; !ok || at.Before(previous) {
			days[day] = at
		}
	}
	points := make([]time.Time, 0, len(days)+1)
	if !first.IsZero() {
		points = append(points, first)
	}
	for _, at := range days {
		if !at.Equal(first) {
			points = append(points, at)
		}
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Before(points[j]) })
	return points
}
