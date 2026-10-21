package insight

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrVideoCurationStore = errors.New("video curation store is not configured")

// Operator decisions live separately from collected facts. Collection never
// overwrites a selection, and exclusion never deletes source observations.
type VideoCuration struct {
	WorkspaceID string    `json:"-"`
	VideoID     string    `json:"videoId"`
	Status      string    `json:"status"`
	Note        string    `json:"note"`
	Tags        []string  `json:"tags"`
	UpdatedAt   time.Time `json:"updatedAt"`
	UpdatedBy   string    `json:"updatedBy"`
}

type VideoCurationRepository interface {
	List(context.Context, string) ([]VideoCuration, error)
	SaveBatch(context.Context, []VideoCuration) error
}

type VideoCurationPatch struct {
	IDs    []string  `json:"ids"`
	Status *string   `json:"status,omitempty"`
	Note   *string   `json:"note,omitempty"`
	Tags   *[]string `json:"tags,omitempty"`
}

type VideoManagementSummary struct {
	Total      int `json:"total"`
	Unreviewed int `json:"unreviewed"`
	Selected   int `json:"selected"`
	Watching   int `json:"watching"`
	Excluded   int `json:"excluded"`
}

func validVideoCurationStatus(value string) bool {
	return value == "unreviewed" || value == "selected" || value == "watching" || value == "excluded"
}

func (s *DataCenterService) CurateVideos(ctx context.Context, query DataCenterQuery, patch VideoCurationPatch, actor string) ([]VideoCuration, error) {
	s.curationMu.Lock()
	defer s.curationMu.Unlock()
	if len(patch.IDs) == 0 || len(patch.IDs) > 50 {
		return nil, errors.New("video curation ids must contain 1 to 50 videos")
	}
	if patch.Status == nil && patch.Note == nil && patch.Tags == nil {
		return nil, errors.New("video curation patch must contain status, note or tags")
	}
	if patch.Status != nil && !validVideoCurationStatus(*patch.Status) {
		return nil, errors.New("unsupported video curation status")
	}
	if patch.Note != nil && utf8.RuneCountInString(*patch.Note) > 2000 {
		return nil, errors.New("video curation note must not exceed 2000 characters")
	}
	if patch.Tags != nil {
		if len(*patch.Tags) > 12 {
			return nil, errors.New("video curation tags must not exceed 12")
		}
		for _, tag := range *patch.Tags {
			if utf8.RuneCountInString(tag) > 30 {
				return nil, errors.New("video curation tag must not exceed 30 characters")
			}
		}
	}
	if s.curations == nil {
		return nil, ErrVideoCurationStore
	}
	workspaceID, hubUserID, err := s.resolveContext(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return nil, err
	}
	// Fetch only video evidence, never the unbounded raw comment library.
	known, err := s.videoEvidenceIDs(ctx, workspaceID, hubUserID)
	if err != nil {
		return nil, err
	}
	for _, id := range patch.IDs {
		if !known[id] {
			return nil, fmt.Errorf("%w: video %s", ErrDataCenterNotFound, id)
		}
	}
	items := make([]VideoCuration, 0, len(patch.IDs))
	mutate := func(existing map[string]VideoCuration) ([]VideoCuration, error) {
		now := s.now().UTC().Truncate(time.Microsecond)
		seen := map[string]bool{}
		for _, id := range patch.IDs {
			if seen[id] {
				continue
			}
			seen[id] = true
			item := existing[id]
			item.WorkspaceID, item.VideoID = workspaceID, id
			if item.Status == "" {
				item.Status = "unreviewed"
			}
			if patch.Status != nil {
				item.Status = *patch.Status
			}
			if patch.Note != nil {
				item.Note = strings.TrimSpace(*patch.Note)
			}
			if patch.Tags != nil {
				item.Tags = exactStrings(*patch.Tags)
			}
			if item.Tags == nil {
				item.Tags = []string{}
			}
			item.UpdatedAt, item.UpdatedBy = now, actor
			items = append(items, item)
		}
		return items, nil
	}
	if atomic, ok := s.curations.(AtomicCurationRepository); ok {
		err = atomic.MutateCurations(ctx, workspaceID, mutate)
	} else {
		var existing map[string]VideoCuration
		existing, err = s.curationMap(ctx, workspaceID)
		if err == nil {
			_, err = mutate(existing)
		}
		if err == nil {
			err = s.curations.SaveBatch(ctx, items)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("save video decisions: %w", err)
	}
	return items, nil
}

func (s *DataCenterService) GetVideoCuration(ctx context.Context, query DataCenterQuery, videoID string) (VideoCuration, error) {
	if s.curations == nil {
		return VideoCuration{}, ErrVideoCurationStore
	}
	workspaceID, _, err := s.resolveContext(ctx, query.HubUserID, query.DeviceCount, query.ContextSource)
	if err != nil {
		return VideoCuration{}, err
	}
	items, err := s.curationMap(ctx, workspaceID)
	if err != nil {
		return VideoCuration{}, err
	}
	if item, ok := items[videoID]; ok {
		return item, nil
	}
	return VideoCuration{VideoID: videoID, Status: "unreviewed", Tags: []string{}}, nil
}

func (s *DataCenterService) curationMap(ctx context.Context, workspaceID string) (map[string]VideoCuration, error) {
	result := map[string]VideoCuration{}
	if s.curations == nil {
		return result, nil
	}
	items, err := s.curations.List(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load video decisions: %w", err)
	}
	for _, item := range items {
		result[item.VideoID] = item
	}
	return result, nil
}

func filterVideoWorkbench(rows []DataCenterItem, query DataCenterQuery) ([]DataCenterItem, *VideoManagementSummary) {
	summary := &VideoManagementSummary{Total: len(rows)}
	filtered := make([]DataCenterItem, 0, len(rows))
	for _, row := range rows {
		switch row.Curation.Status {
		case "selected":
			summary.Selected++
		case "watching":
			summary.Watching++
		case "excluded":
			summary.Excluded++
		default:
			summary.Unreviewed++
		}
		if query.Curation == "" || query.Curation == "active" {
			if row.Curation.Status == "excluded" {
				continue
			}
		} else if query.Curation != "all" && query.Curation != row.Curation.Status {
			continue
		}
		if query.Performance != "" && query.Performance != row.Performance.Tier {
			continue
		}
		if query.MinInteractions != nil && (row.Performance.Interactions == nil || *row.Performance.Interactions < *query.MinInteractions) {
			continue
		}
		if query.MinLikes != nil && (row.Likes == nil || *row.Likes < *query.MinLikes) {
			continue
		}
		if query.FreshOnly && row.Performance.Freshness != "fresh" {
			continue
		}
		if query.RecentOnly && (row.Performance.RecentHeat.Score == nil || *row.Performance.RecentHeat.Score <= 0) {
			continue
		}
		if query.MomentumOnly && (row.Performance.GrowthPerHour == nil || *row.Performance.GrowthPerHour <= 0 || row.Performance.Freshness != "fresh") {
			continue
		}
		if query.Tag != "" && !containsExact(row.Curation.Tags, query.Tag) {
			continue
		}
		filtered = append(filtered, row)
	}
	return filtered, summary
}
