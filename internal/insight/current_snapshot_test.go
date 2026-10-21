package insight

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestDashboardSchemaContainsOnlyCurrentAnalysis(t *testing.T) {
	var snapshot Dashboard
	if err := json.Unmarshal([]byte(`{"hotVideos":[{"id":"current-video"}],"teamFollowUps":[{"id":"old","workflow":{"production":{"task":{}}}}],"topicIdeas":[{"id":"old-idea"}]}`), &snapshot); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(cloneDashboard(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"teamFollowUps", "topicIdeas", "production"} {
		if strings.Contains(string(encoded), field) {
			t.Fatalf("current snapshot still carries removed field %s: %s", field, encoded)
		}
	}
	if len(snapshot.HotVideos) != 1 || snapshot.HotVideos[0].ID != "current-video" {
		t.Fatal("current video facts were lost")
	}
}

type currentSnapshotRepository struct {
	snapshot Dashboard
}

func (*currentSnapshotRepository) GetDashboard(context.Context, string) (Dashboard, error) {
	return Dashboard{}, errors.New("refresh must not read a previous snapshot to merge retired fields")
}

func (r *currentSnapshotRepository) ReplaceDashboard(_ context.Context, _ string, snapshot Dashboard) error {
	r.snapshot = snapshot
	return nil
}

func (*currentSnapshotRepository) ResetDashboard(context.Context, string) error { return nil }

func TestAnalyticalRefreshWritesCurrentSnapshotDirectly(t *testing.T) {
	repo := &currentSnapshotRepository{}
	_, err := persistDashboardSnapshot(context.Background(), "ws-1", Dashboard{HotVideos: []HotVideo{{ID: "current-video"}}}, repo, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.snapshot.HotVideos) != 1 || repo.snapshot.HotVideos[0].ID != "current-video" {
		t.Fatal("refresh did not persist current analysis")
	}
}
