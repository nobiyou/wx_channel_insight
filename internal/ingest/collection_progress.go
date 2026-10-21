package ingest

import (
	"context"
	"sort"
	"time"
)

// CollectionProgress belongs to one source/target and is saved alongside its
// ingest run. A completed pass has no continuation cursor; the next pass
// starts at the newest page again.
type CollectionProgress struct {
	Source     string    `json:"source"`
	TargetKey  string    `json:"targetKey"`
	PageCount  int       `json:"pageCount"`
	VideoCount int       `json:"videoCount"`
	Complete   bool      `json:"complete"`
	NextMarker string    `json:"nextMarker,omitempty"`
	Notice     string    `json:"notice,omitempty"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

func (s *Service) ListCollectionProgress(ctx context.Context, query SummaryQuery) ([]CollectionProgress, error) {
	runs, err := s.repo.ListRuns(ctx, s.resolveHubUserID(query.HubUserID))
	if err != nil {
		return nil, err
	}
	latest := map[string]CollectionProgress{}
	for _, run := range runs {
		if run.Collection == nil {
			continue
		}
		item := *run.Collection
		key := item.Source + "\x00" + item.TargetKey
		if old, ok := latest[key]; !ok || item.UpdatedAt.After(old.UpdatedAt) {
			latest[key] = item
		}
	}
	items := make([]CollectionProgress, 0, len(latest))
	for _, item := range latest {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items, nil
}

func (s *Service) collectionStart(ctx context.Context, query SummaryQuery, source, target string) (CollectionProgress, error) {
	items, err := s.ListCollectionProgress(ctx, query)
	if err != nil {
		return CollectionProgress{}, err
	}
	for _, item := range items {
		if item.Source == source && item.TargetKey == target && !item.Complete {
			return item, nil
		}
	}
	return CollectionProgress{Source: source, TargetKey: target}, nil
}

func progressFromResult(previous CollectionProgress, result WXFeedListResult, now time.Time) CollectionProgress {
	previous.PageCount += result.PageCount - result.refreshedHeadPages
	previous.VideoCount += len(result.Items) - result.refreshedHeadItems
	previous.Complete = result.Complete
	previous.NextMarker = result.NextMarker
	previous.Notice = result.Notice
	previous.UpdatedAt = now
	return previous
}
