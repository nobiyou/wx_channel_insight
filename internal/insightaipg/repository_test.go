package insightaipg_test

import (
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/insightaipg"
	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/testpostgres"
)

func TestRepositoryInitializesWhenPostgresIsAvailable(t *testing.T) {
	db, _ := testpostgres.Open(t)
	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatalf("new postgres client: %v", err)
	}
	if _, err := insightaipg.NewRepository(client); err != nil {
		t.Fatalf("new AI settings repository: %v", err)
	}
}
