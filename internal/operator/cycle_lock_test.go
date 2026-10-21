package operator_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
	"github.com/nobiyou/wx_channel_insight/internal/ingestdemo"
	operatorflow "github.com/nobiyou/wx_channel_insight/internal/operator"
)

type blockingBindingRepository struct {
	*ingestdemo.Repository
	entered chan struct{}
	release chan struct{}
}

func (r *blockingBindingRepository) ListBindings(ctx context.Context, hubUserID string) ([]ingest.SourceBinding, error) {
	select {
	case r.entered <- struct{}{}:
	default:
	}
	select {
	case <-r.release:
		return r.Repository.ListBindings(ctx, hubUserID)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestRunCycleRejectsConcurrentCallAndReleasesGateAfterError(t *testing.T) {
	repo := &blockingBindingRepository{
		Repository: ingestdemo.NewRepository(),
		entered:    make(chan struct{}, 1),
		release:    make(chan struct{}),
	}
	ingestService := ingest.NewService(repo, ingest.ServiceConfig{
		DefaultHubUserID: "insight:operator:local",
		Now: func() time.Time {
			return time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
		},
	})
	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service failed: %v", err)
	}
	options := operatorflow.Options{HubUserID: "hub:user:alice", DeviceCount: 1}

	firstDone := make(chan error, 1)
	go func() {
		_, runErr := service.RunCycle(context.Background(), options)
		firstDone <- runErr
	}()
	select {
	case <-repo.entered:
	case <-time.After(time.Second):
		t.Fatal("first cycle did not reach the blocking binding repository")
	}

	if _, err := service.RunCycle(context.Background(), options); !errors.Is(err, operatorflow.ErrCycleAlreadyRunning) {
		t.Fatalf("expected concurrent cycle sentinel, got %v", err)
	}

	close(repo.release)
	select {
	case err := <-firstDone:
		if err == nil {
			t.Fatal("expected first cycle to fail because no bindings exist")
		}
	case <-time.After(time.Second):
		t.Fatal("first cycle did not finish after releasing the repository")
	}

	if _, err := service.RunCycle(context.Background(), options); errors.Is(err, operatorflow.ErrCycleAlreadyRunning) {
		t.Fatalf("expected gate release after first cycle error, got %v", err)
	}
}

func TestRunCycleReturnsCycleSentinelBeforeAppendingRunWhenExternalLockerContends(t *testing.T) {
	repo := ingestdemo.NewRepository()
	ingestService := ingest.NewService(repo, ingest.ServiceConfig{DefaultHubUserID: "insight:operator:local"})
	service, err := operatorflow.NewService(ingestService, stubReadModelRefresher{})
	if err != nil {
		t.Fatalf("new operator service failed: %v", err)
	}
	service.SetCycleLocker(contendedCycleLocker{})

	_, err = service.RunCycle(context.Background(), operatorflow.Options{HubUserID: "hub:user:alice"})
	if !errors.Is(err, operatorflow.ErrCycleAlreadyRunning) {
		t.Fatalf("expected external contention sentinel, got %v", err)
	}
	runs, err := ingestService.ListOperatorRuns(context.Background(), ingest.SummaryQuery{HubUserID: "hub:user:alice"})
	if err != nil {
		t.Fatalf("list operator runs failed: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no fake operator run for a contended cycle, got %+v", runs)
	}
}

type contendedCycleLocker struct{}

func (contendedCycleLocker) TryAcquire(context.Context, string) (func(), bool, error) {
	return nil, false, nil
}

var _ operatorflow.CycleLocker = contendedCycleLocker{}
