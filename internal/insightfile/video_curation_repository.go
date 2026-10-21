package insightfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

// An empty path is an explicit in-memory store for demo/test mode.
type VideoCurationRepository struct {
	mu    sync.Mutex
	path  string
	items map[string]insight.VideoCuration
}
type persistedVideoCurations struct {
	Items []storedVideoCuration `json:"items"`
}
type storedVideoCuration struct {
	WorkspaceID string `json:"workspaceId"`
	insight.VideoCuration
}

func NewVideoCurationRepository(path string) (*VideoCurationRepository, error) {
	r := &VideoCurationRepository{path: path, items: map[string]insight.VideoCuration{}}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func curationKey(item insight.VideoCuration) string { return item.WorkspaceID + "\x00" + item.VideoID }
func cloneCuration(item insight.VideoCuration) insight.VideoCuration {
	item.Tags = append([]string{}, item.Tags...)
	return item
}

func (r *VideoCurationRepository) load() error {
	if r.path == "" {
		return nil
	}
	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var payload persistedVideoCurations
	if err = json.Unmarshal(data, &payload); err != nil {
		return err
	}
	items := map[string]insight.VideoCuration{}
	for _, row := range payload.Items {
		item := row.VideoCuration
		item.WorkspaceID = row.WorkspaceID
		items[curationKey(item)] = item
	}
	r.items = items
	return nil
}

func (r *VideoCurationRepository) List(ctx context.Context, workspace string) ([]insight.VideoCuration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.load(); err != nil {
		return nil, err
	}
	items := []insight.VideoCuration{}
	for _, item := range r.items {
		if item.WorkspaceID == workspace {
			items = append(items, cloneCuration(item))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].VideoID < items[j].VideoID })
	return items, nil
}

func (r *VideoCurationRepository) SaveBatch(ctx context.Context, items []insight.VideoCuration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.load(); err != nil {
		return err
	}
	next := map[string]insight.VideoCuration{}
	for key, row := range r.items {
		next[key] = row
	}
	for _, row := range items {
		next[curationKey(row)] = cloneCuration(row)
	}
	if r.path != "" {
		payload := persistedVideoCurations{Items: []storedVideoCuration{}}
		for _, row := range next {
			payload.Items = append(payload.Items, storedVideoCuration{WorkspaceID: row.WorkspaceID, VideoCuration: row})
		}
		sort.Slice(payload.Items, func(i, j int) bool {
			return curationKey(payload.Items[i].VideoCuration) < curationKey(payload.Items[j].VideoCuration)
		})
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(r.path), 0755); err != nil {
			return err
		}
		file, err := os.CreateTemp(filepath.Dir(r.path), ".video-curations-*")
		if err != nil {
			return err
		}
		temp := file.Name()
		defer os.Remove(temp)
		if _, err = file.Write(data); err != nil {
			file.Close()
			return err
		}
		if err = file.Close(); err != nil {
			return err
		}
		if err = os.Rename(temp, r.path); err != nil {
			return err
		}
	}
	r.items = next
	return nil
}
