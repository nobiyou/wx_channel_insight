package insight_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
	"github.com/nobiyou/wx_channel_insight/internal/insightdemo"
)

func TestDashboardServiceRejectsSparseSnapshot(t *testing.T) {
	service := insight.NewDashboardService(insightdemo.NewDashboardRepository(), insight.DashboardServiceConfig{})

	err := service.ReplaceDashboard(context.Background(), insight.Dashboard{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !errors.Is(err, insight.ErrInvalidDashboardSnapshot) {
		t.Fatalf("expected invalid dashboard snapshot error, got %v", err)
	}
}
