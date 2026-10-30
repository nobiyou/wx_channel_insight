package insight

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ExportVideos uses the same assessed, authorized snapshot as the workbench,
// including filters and sort order, without its display pagination.
func (s *DataCenterService) ExportVideos(ctx context.Context, query DataCenterQuery, ids []string) ([]DataCenterItem, error) {
	if len(ids) > 50 {
		return nil, errors.New("video export ids must contain at most 50 videos")
	}
	selected := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !strings.HasPrefix(id, "video:") || strings.TrimSpace(strings.TrimPrefix(id, "video:")) == "" {
			return nil, errors.New("video export ids must be valid video identifiers")
		}
		selected[id] = true
	}
	query.Entity = DataCenterEntityVideos
	query.Page, query.PageSize, query.Refresh, query.allItems = 1, 50, false, true
	page, err := s.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return page.Items, nil
	}
	items := make([]DataCenterItem, 0, len(selected))
	for _, item := range page.Items {
		if selected[item.ID] {
			items = append(items, item)
			delete(selected, item.ID)
		}
	}
	if len(selected) != 0 {
		return nil, fmt.Errorf("%w: selected videos are outside the current result", ErrDataCenterNotFound)
	}
	return items, nil
}
