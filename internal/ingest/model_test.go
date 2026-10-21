package ingest_test

import (
	"testing"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

func TestNormalizeSourceBindingDefaultsOriginAndTargetKey(t *testing.T) {
	got := ingest.NormalizeSourceBinding(ingest.SourceBinding{
		Source:   ingest.RunSourceWXFeedList,
		Username: " account-one ",
	})

	if got.Origin != ingest.BindingOriginManual {
		t.Fatalf("expected missing origin to default to manual, got %q", got.Origin)
	}
	if got.TargetKey != "account-one" {
		t.Fatalf("expected target key from username, got %q", got.TargetKey)
	}
}

func TestNormalizeSourceBindingForUpsertPreservesExistingOrigin(t *testing.T) {
	got := ingest.NormalizeSourceBindingForUpsert(
		ingest.SourceBinding{Source: ingest.RunSourceWXFeedList, Username: "account-one"},
		ingest.SourceBinding{Source: ingest.RunSourceWXFeedList, Username: "account-one", Origin: ingest.BindingOriginMonitoring},
	)

	if got.Origin != ingest.BindingOriginMonitoring {
		t.Fatalf("expected refresh to preserve monitoring origin, got %q", got.Origin)
	}
}
