package promptpg_test

import (
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/postgres"
	"github.com/nobiyou/wx_channel_insight/internal/promptpg"
	"github.com/nobiyou/wx_channel_insight/internal/testpostgres"
)

func TestRepositoryRoundTripsWhenPostgresIsAvailable(t *testing.T) {
	db, _ := testpostgres.Open(t)
	client, err := postgres.NewClientFromDB(db, "postgres://test")
	if err != nil {
		t.Fatalf("new postgres client: %v", err)
	}
	if _, err := promptpg.NewRepository(client); err != nil {
		t.Fatalf("new prompt repository: %v", err)
	}
}
