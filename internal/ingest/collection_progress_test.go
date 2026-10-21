package ingest_test

import (
	"context"
	"fmt"
	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestfile"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestAccountPaginationResumesAfterRepositoryReopen(t *testing.T) {
	calls := []string{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		marker := r.URL.Query().Get("next_marker")
		calls = append(calls, marker)
		switch marker {
		case "":
			fmt.Fprint(w, `{"code":0,"data":{"objectList":[{"id":"first"}],"lastBuffer":"second"}}`)
		case "second":
			fmt.Fprint(w, `{"code":0,"data":{"objectList":[],"lastBuffer":""}}`)
		default:
			t.Error("unexpected cursor")
		}
	}))
	defer upstream.Close()
	path := filepath.Join(t.TempDir(), "ingest.json")
	now := time.Now()
	config := ingest.ServiceConfig{DefaultHubUserID: "owner", WXFeedListClient: ingest.WXFeedListClient{BaseURL: upstream.URL, Client: upstream.Client(), MaxPages: 1}, Now: func() time.Time { now = now.Add(time.Second); return now }}
	repo, err := ingestfile.NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	service := ingest.NewService(repo, config)
	first, _, err := service.ImportWXFeedListForAccount(context.Background(), ingest.SummaryQuery{HubUserID: "owner"}, "account", "")
	if err != nil || first.Complete || first.NextMarker != "second" {
		t.Fatalf("first result: %+v %v", first, err)
	}
	repo, err = ingestfile.NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	service = ingest.NewService(repo, config)
	last, _, err := service.ImportWXFeedListForAccount(context.Background(), ingest.SummaryQuery{HubUserID: "owner"}, "account", "")
	if err != nil || !last.Complete {
		t.Fatalf("resume: %+v %v", last, err)
	}
	progress, err := service.ListCollectionProgress(context.Background(), ingest.SummaryQuery{HubUserID: "owner"})
	if err != nil || len(progress) != 1 || !progress[0].Complete || progress[0].PageCount != 2 || progress[0].VideoCount != 1 {
		t.Fatalf("saved coverage: %+v %v", progress, err)
	}
	if len(calls) != 3 || calls[0] != "" || calls[1] != "" || calls[2] != "second" {
		t.Fatalf("wrong pagination: %v", calls)
	}
}

type cancellationCheckingRepository struct{ ingest.Repository }

func (r cancellationCheckingRepository) AppendEvents(ctx context.Context, owner string, events []ingest.Event) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("cannot save collected pages: %w", err)
	}
	return r.Repository.AppendEvents(ctx, owner, events)
}

func TestCanceledContinuationStillSavesCollectedPages(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("next_marker") != "" {
			cancel()
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{"objectList":[{"id":"saved"}],"lastBuffer":"resume"}}`)
	}))
	defer upstream.Close()
	repo, err := ingestfile.NewRepository(filepath.Join(t.TempDir(), "ingest.json"))
	if err != nil {
		t.Fatal(err)
	}
	service := ingest.NewService(cancellationCheckingRepository{repo}, ingest.ServiceConfig{DefaultHubUserID: "owner", WXFeedListClient: ingest.WXFeedListClient{BaseURL: upstream.URL, Client: upstream.Client()}})
	result, _, err := service.ImportWXFeedListForAccount(ctx, ingest.SummaryQuery{HubUserID: "owner"}, "account", "")
	if err != nil || result.Complete {
		t.Fatalf("partial save failed: %+v %v", result, err)
	}
	progress, err := service.ListCollectionProgress(context.Background(), ingest.SummaryQuery{HubUserID: "owner"})
	if err != nil || len(progress) != 1 || progress[0].NextMarker != "resume" || progress[0].VideoCount != 1 {
		t.Fatalf("progress lost: %+v %v", progress, err)
	}
}
