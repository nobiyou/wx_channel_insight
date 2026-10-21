package ingest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// RunSourceWXVideoCoverBackfill identifies a metadata-only repair operation.
// It is intentionally not a saved binding and is not used as a ranking source.
const RunSourceWXVideoCoverBackfill = "wx_video_cover_backfill"

const videoCoverBackfillProfileLimit = 120

type VideoCoverBackfillResult struct {
	Status               string   `json:"status"`
	HubUserID            string   `json:"hubUserId"`
	BatchID              string   `json:"batchId"`
	KeywordCount         int      `json:"keywordCount"`
	CandidateCount       int      `json:"candidateCount"`
	MatchedCount         int      `json:"matchedCount"`
	EventsUpdated        int      `json:"eventsUpdated"`
	ObservationsAppended int      `json:"observationsAppended"`
	ProfileAttempts      int      `json:"profileAttempts"`
	ProfileMatched       int      `json:"profileMatched"`
	SearchFailures       []string `json:"searchFailures,omitempty"`
	ProfileFailures      []string `json:"profileFailures,omitempty"`
}

type videoCoverBackfillTarget struct {
	event             Event
	latestObservation VideoMetricObservation
	hasObservation    bool
	keyword           string
}

// BackfillWXVideoCovers re-queries the live wx_channel video-search API for
// existing wx_video_search observations whose latest event/observation lacks a
// cover. It updates the current event snapshot and appends one metadata-rich
// observation without creating a monitoring binding or changing metric values.
func (s *Service) BackfillWXVideoCovers(ctx context.Context, query SummaryQuery) (VideoCoverBackfillResult, error) {
	if s == nil || s.repo == nil {
		return VideoCoverBackfillResult{}, errors.New("ingest service is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	hubUserID := s.resolveHubUserID(query.HubUserID)
	startedAt := time.Now()
	if s.now != nil {
		startedAt = s.now()
	}
	batchID := "cover-backfill-" + startedAt.UTC().Format("20060102T150405.000000000Z")
	result := VideoCoverBackfillResult{
		Status:    "no_change",
		HubUserID: hubUserID,
		BatchID:   batchID,
	}

	events, err := s.repo.ListEvents(ctx, hubUserID)
	if err != nil {
		return result, fmt.Errorf("list events for video cover backfill: %w", err)
	}
	observations, err := s.repo.ListVideoMetricObservations(ctx, hubUserID)
	if err != nil {
		return result, fmt.Errorf("list video observations for cover backfill: %w", err)
	}

	targets := buildVideoCoverBackfillTargets(hubUserID, events, observations)
	if len(query.ObjectIDs) > 0 {
		allowedObjectIDs := make(map[string]struct{}, len(query.ObjectIDs))
		for _, objectID := range query.ObjectIDs {
			if objectID = strings.TrimSpace(objectID); objectID != "" {
				allowedObjectIDs[objectID] = struct{}{}
			}
		}
		filteredTargets := make([]videoCoverBackfillTarget, 0, len(allowedObjectIDs))
		for _, target := range targets {
			if _, allowed := allowedObjectIDs[target.event.ObjectID]; allowed {
				filteredTargets = append(filteredTargets, target)
			}
		}
		targets = filteredTargets
	}
	result.CandidateCount = len(targets)
	if len(targets) == 0 {
		result.Status = "noop"
		return result, nil
	}

	keywords := make([]string, 0, len(targets))
	seenKeywords := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		keyword := strings.TrimSpace(target.keyword)
		if keyword == "" {
			continue
		}
		if _, exists := seenKeywords[keyword]; exists {
			continue
		}
		seenKeywords[keyword] = struct{}{}
		keywords = append(keywords, keyword)
	}
	sort.Strings(keywords)
	result.KeywordCount = len(keywords)
	if len(keywords) == 0 {
		result.Status = "no_keyword"
		return result, nil
	}

	itemsByObjectID := make(map[string]WXFeedListItem)
	for _, keyword := range keywords {
		liveResult, searchErr := s.SearchWXVideos(ctx, SummaryQuery{
			HubUserID:   hubUserID,
			DeviceCount: query.DeviceCount,
		}, "", keyword, "")
		if searchErr != nil {
			result.SearchFailures = append(result.SearchFailures, fmt.Sprintf("%s: %v", keyword, searchErr))
			continue
		}
		for _, item := range liveResult.Items {
			objectID := strings.TrimSpace(item.ID)
			if objectID == "" || strings.TrimSpace(item.CoverURL) == "" {
				continue
			}
			itemsByObjectID[objectID] = item
		}
	}

	// Search results are current and bounded. Probe a small, interaction-prioritized
	// set by exact profile so one maintenance run cannot flood the WeChat client.
	profileAttempts := 0
	for _, target := range targets {
		if profileAttempts >= videoCoverBackfillProfileLimit {
			break
		}
		if _, found := itemsByObjectID[target.event.ObjectID]; found {
			continue
		}
		nonceID := strings.TrimSpace(target.event.ObjectNonceID)
		if nonceID == "" && target.hasObservation {
			nonceID = strings.TrimSpace(target.latestObservation.ObjectNonceID)
		}
		if strings.TrimSpace(target.event.ObjectID) == "" || nonceID == "" {
			continue
		}
		profileAttempts++
		result.ProfileAttempts = profileAttempts
		item, profileErr := s.wxFeedListClient.FetchVideoProfile(ctx, target.event.ObjectID, nonceID)
		if profileErr != nil {
			result.ProfileFailures = append(result.ProfileFailures, fmt.Sprintf("%s: %v", target.event.ObjectID, profileErr))
			continue
		}
		if strings.TrimSpace(item.CoverURL) == "" {
			continue
		}
		itemsByObjectID[target.event.ObjectID] = item
		result.ProfileMatched++
	}

	updatedEvents := append([]Event(nil), events...)
	eventIndexes := make(map[string]int, len(updatedEvents))
	for index, event := range updatedEvents {
		if event.HubUserID == hubUserID && strings.TrimSpace(event.ID) != "" {
			eventIndexes[event.ID] = index
		}
	}

	updatedObservations := make([]VideoMetricObservation, 0, len(targets))
	for _, target := range targets {
		item, found := itemsByObjectID[target.event.ObjectID]
		if !found {
			continue
		}
		result.MatchedCount++
		coverURL := strings.TrimSpace(item.CoverURL)

		if index, exists := eventIndexes[target.event.ID]; exists {
			if strings.TrimSpace(updatedEvents[index].CoverURL) == "" && coverURL != "" {
				updatedEvents[index].CoverURL = coverURL
				result.EventsUpdated++
			}
			if updatedEvents[index].PublishedAt.IsZero() && !item.PublishedAt.IsZero() {
				updatedEvents[index].PublishedAt = item.PublishedAt
				result.EventsUpdated++
			}
		}

		needsObservation := !target.hasObservation || strings.TrimSpace(target.latestObservation.CoverURL) == "" ||
			(target.latestObservation.PublishedAt.IsZero() && !item.PublishedAt.IsZero())
		if !needsObservation {
			continue
		}

		observation := target.latestObservation
		if !target.hasObservation {
			observation = observationFromVideoCoverBackfillEvent(target.event)
		}
		observation.ID = strings.TrimSpace(target.event.ID) + "@" + startedAt.UTC().Format(time.RFC3339Nano)
		observation.HubUserID = hubUserID
		observation.EventID = target.event.ID
		observation.BatchID = batchID
		observation.Source = RunSourceWXVideoSearch
		observation.ObservedAt = startedAt
		if observation.CoverURL == "" {
			observation.CoverURL = coverURL
		}
		if observation.PublishedAt.IsZero() {
			observation.PublishedAt = item.PublishedAt
		}
		if observation.ObjectID == "" {
			observation.ObjectID = target.event.ObjectID
		}
		if observation.ObjectNonceID == "" {
			observation.ObjectNonceID = target.event.ObjectNonceID
		}
		if observation.Keyword == "" {
			observation.Keyword = target.keyword
		}
		updatedObservations = append(updatedObservations, observation)
	}

	if result.EventsUpdated > 0 {
		if err := s.repo.ReplaceEvents(ctx, hubUserID, updatedEvents); err != nil {
			return result, fmt.Errorf("persist video cover events: %w", err)
		}
	}
	if len(updatedObservations) > 0 {
		if err := s.repo.AppendVideoMetricObservations(ctx, hubUserID, updatedObservations); err != nil {
			return result, fmt.Errorf("append video cover observations: %w", err)
		}
	}
	result.ObservationsAppended = len(updatedObservations)
	if result.MatchedCount > 0 && result.EventsUpdated == 0 && result.ObservationsAppended == 0 {
		result.Status = "noop"
		return result, nil
	}

	switch {
	case result.MatchedCount > 0 && (len(result.SearchFailures) > 0 || result.MatchedCount < result.CandidateCount):
		result.Status = "partial"
	case result.MatchedCount > 0:
		result.Status = "success"
	case len(result.SearchFailures) > 0 || len(result.ProfileFailures) > 0:
		result.Status = "failed"
	case result.CandidateCount > 0:
		result.Status = "no_match"
	}
	if result.Status == "failed" {
		failures := append([]string(nil), result.SearchFailures...)
		failures = append(failures, result.ProfileFailures...)
		return result, fmt.Errorf("video cover backfill failed: %s", strings.Join(failures, "; "))
	}
	return result, nil
}

func buildVideoCoverBackfillTargets(hubUserID string, events []Event, observations []VideoMetricObservation) []videoCoverBackfillTarget {
	latestObservations := make(map[string]VideoMetricObservation)
	for _, observation := range observations {
		if observation.HubUserID != hubUserID || canonicalSource(observation.Source) != RunSourceWXVideoSearch {
			continue
		}
		objectID := strings.TrimSpace(observation.ObjectID)
		if objectID == "" {
			continue
		}
		current, exists := latestObservations[objectID]
		if !exists || observation.ObservedAt.After(current.ObservedAt) ||
			(observation.ObservedAt.Equal(current.ObservedAt) && observation.ID > current.ID) {
			latestObservations[objectID] = observation
		}
	}

	latestEvents := make(map[string]Event)
	for _, event := range events {
		if event.HubUserID != hubUserID || event.EventType != "video_snapshot" || canonicalSource(event.Source) != RunSourceWXVideoSearch {
			continue
		}
		objectID := strings.TrimSpace(event.ObjectID)
		if objectID == "" {
			continue
		}
		current, exists := latestEvents[objectID]
		if !exists || event.CollectedAt.After(current.CollectedAt) ||
			(event.CollectedAt.Equal(current.CollectedAt) && event.ID > current.ID) {
			latestEvents[objectID] = event
		}
	}

	result := make([]videoCoverBackfillTarget, 0, len(latestEvents))
	for objectID, event := range latestEvents {
		latestObservation, hasObservation := latestObservations[objectID]
		needsCover := strings.TrimSpace(event.CoverURL) == "" || !hasObservation || strings.TrimSpace(latestObservation.CoverURL) == ""
		needsPublishedAt := event.PublishedAt.IsZero() || (hasObservation && latestObservation.PublishedAt.IsZero())
		if !needsCover && !needsPublishedAt {
			continue
		}
		keyword := strings.TrimSpace(event.Keyword)
		if keyword == "" && hasObservation {
			keyword = strings.TrimSpace(latestObservation.Keyword)
		}
		if keyword == "" {
			continue
		}
		result = append(result, videoCoverBackfillTarget{
			event:             event,
			latestObservation: latestObservation,
			hasObservation:    hasObservation,
			keyword:           keyword,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		leftPriority := videoCoverBackfillPriority(result[i].event)
		rightPriority := videoCoverBackfillPriority(result[j].event)
		if leftPriority != rightPriority {
			return leftPriority > rightPriority
		}
		if !result[i].event.CollectedAt.Equal(result[j].event.CollectedAt) {
			return result[i].event.CollectedAt.After(result[j].event.CollectedAt)
		}
		return result[i].event.ObjectID < result[j].event.ObjectID
	})
	return result
}

func videoCoverBackfillPriority(event Event) int64 {
	return int64(maxInt(0, event.LikeCount)) +
		int64(maxInt(0, event.CommentCount)) +
		int64(maxInt(0, event.FavCount)) +
		int64(maxInt(0, event.ForwardCount))
}

func observationFromVideoCoverBackfillEvent(event Event) VideoMetricObservation {
	return VideoMetricObservation{
		HubUserID:       event.HubUserID,
		EventID:         event.ID,
		Source:          RunSourceWXVideoSearch,
		PublishedAt:     event.PublishedAt,
		ObjectID:        event.ObjectID,
		ObjectNonceID:   event.ObjectNonceID,
		ChannelUsername: event.ChannelUsername,
		Title:           event.Title,
		ChannelName:     event.ChannelName,
		Keyword:         event.Keyword,
		Tags:            append([]string(nil), event.Tags...),
		PlayCount:       event.PlayCount,
		LikeCount:       event.LikeCount,
		CommentCount:    event.CommentCount,
		FavCount:        event.FavCount,
		ForwardCount:    event.ForwardCount,
	}
}
