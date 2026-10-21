package insightapi_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightapi"
)

func TestAnalyticalDashboardDoesNotExposeArchivedProduction(t *testing.T) {
	var snapshot insight.Dashboard
	if err := json.Unmarshal([]byte(`{"hotVideos":[{"id":"current-video"}],"teamFollowUps":[{"workflow":{"production":{"task":{"assets":[{"uri":"private/asset.mp4"}]}}}}],"topicIdeas":[{"id":"old"}]}`), &snapshot); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(insightapi.NewDashboardResponse(snapshot))
	if err != nil {
		t.Fatal(err)
	}
	for _, retired := range []string{"teamFollowUps", "topicIdeas", "production", "private/asset.mp4"} {
		if strings.Contains(string(encoded), retired) {
			t.Fatalf("retired data exposed in analytical dashboard: %s", encoded)
		}
	}
}
