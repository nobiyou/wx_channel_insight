package insight

import (
	"sort"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/normalize"
)

func applyNormalizedOverlay(payload *Dashboard, sourceSummary ingest.Summary, snapshot normalize.Snapshot) {
	normalizedSummary := buildNormalizedIngestSummary(sourceSummary, snapshot)
	if len(normalizedSummary.RecentEvents) == 0 {
		applyIngestOverlay(payload, sourceSummary)
		return
	}
	applyIngestOverlay(payload, normalizedSummary)
}

func buildNormalizedIngestSummary(sourceSummary ingest.Summary, snapshot normalize.Snapshot) ingest.Summary {
	summary := sourceSummary
	events := make([]ingest.Event, 0, len(snapshot.ContentSnapshots)+len(snapshot.CommentSnapshots)+len(snapshot.TopicObservations))

	for _, item := range snapshot.ContentSnapshots {
		events = append(events, ingest.Event{
			ID:              item.EventID,
			BatchID:         item.BatchID,
			EventType:       "video_snapshot",
			Source:          item.Source,
			CollectedAt:     item.CollectedAt,
			PublishedAt:     item.PublishedAt,
			ObjectID:        item.ObjectID,
			ObjectNonceID:   item.ObjectNonceID,
			ChannelUsername: item.ChannelUsername,
			CoverURL:        item.CoverURL,
			VideoCount:      maxInt(1, item.VideoCount),
			PlayCount:       maxInt(0, item.PlayCount),
			LikeCount:       maxInt(0, item.LikeCount),
			CommentCount:    maxInt(0, item.CommentCount),
			FavCount:        maxInt(0, item.FavCount),
			ForwardCount:    maxInt(0, item.ForwardCount),
			TopicCount:      maxInt(0, item.TopicCount),
			Title:           item.Title,
			ChannelName:     item.ChannelName,
			Keyword:         item.Keyword,
			Signal:          item.Signal,
			PromptAngle:     item.PromptAngle,
			TargetAudience:  item.TargetAudience,
			Tags:            append([]string(nil), item.Tags...),
			Keywords:        append([]string(nil), item.Keywords...),
		})
	}
	for _, item := range snapshot.CommentSnapshots {
		events = append(events, ingest.Event{
			ID:                    item.EventID,
			BatchID:               item.BatchID,
			EventType:             "comment_snapshot",
			Source:                item.Source,
			CollectedAt:           item.CollectedAt,
			ObjectID:              item.ObjectID,
			ObjectNonceID:         item.ObjectNonceID,
			VideoCount:            maxInt(1, item.VideoCount),
			CommentCount:          maxInt(0, item.CommentCount),
			TopicCount:            maxInt(0, item.TopicCount),
			Title:                 item.Title,
			ChannelName:           item.ChannelName,
			Keyword:               item.Keyword,
			Signal:                item.Signal,
			RepresentativeComment: item.RepresentativeComment,
			TargetAudience:        item.TargetAudience,
			Keywords:              append([]string(nil), item.Keywords...),
		})
	}
	for _, item := range snapshot.TopicObservations {
		events = append(events, ingest.Event{
			ID:             item.EventID,
			BatchID:        item.BatchID,
			EventType:      "topic_cluster",
			Source:         item.Source,
			CollectedAt:    item.CollectedAt,
			ObjectID:       item.ObjectID,
			ObjectNonceID:  item.ObjectNonceID,
			VideoCount:     maxInt(1, item.VideoCount),
			CommentCount:   maxInt(0, item.CommentCount),
			TopicCount:     maxInt(0, item.TopicCount),
			Keyword:        item.Keyword,
			Cluster:        item.Cluster,
			Signal:         item.Signal,
			PromptAngle:    item.PromptAngle,
			TargetAudience: item.TargetAudience,
			Keywords:       append([]string(nil), item.Keywords...),
		})
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].CollectedAt.Equal(events[j].CollectedAt) {
			if events[i].BatchID == events[j].BatchID {
				return events[i].ID > events[j].ID
			}
			return events[i].BatchID > events[j].BatchID
		}
		return events[i].CollectedAt.After(events[j].CollectedAt)
	})

	summary.RecentEvents = events
	if snapshot.Summary.LatestBatchID != "" {
		summary.LatestBatchID = snapshot.Summary.LatestBatchID
	}
	if !snapshot.Summary.LatestEventAt.IsZero() {
		summary.LatestEventAt = snapshot.Summary.LatestEventAt
	}
	if summary.TotalEvents == 0 {
		summary.TotalEvents = len(events)
	}
	return summary
}
