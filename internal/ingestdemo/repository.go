package ingestdemo

import (
	"context"
	"errors"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

type Repository struct {
	mu            sync.RWMutex
	items         map[string][]ingest.Event
	metricHistory map[string][]ingest.VideoMetricObservation
	runs          map[string][]ingest.Run
	operatorRuns  map[string][]ingest.OperatorRun
	bindings      map[string][]ingest.SourceBinding
}

func NewRepository() *Repository {
	return &Repository{
		items:         map[string][]ingest.Event{},
		metricHistory: map[string][]ingest.VideoMetricObservation{},
		runs:          map[string][]ingest.Run{},
		operatorRuns:  map[string][]ingest.OperatorRun{},
		bindings:      map[string][]ingest.SourceBinding{},
	}
}

func (r *Repository) ListEvents(_ context.Context, hubUserID string) ([]ingest.Event, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return append([]ingest.Event(nil), r.items[hubUserID]...), nil
}

func (r *Repository) AppendEvents(_ context.Context, hubUserID string, events []ingest.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[hubUserID] = append(r.items[hubUserID], events...)
	return nil
}

func (r *Repository) ReplaceEvents(_ context.Context, hubUserID string, events []ingest.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.items[hubUserID] = append([]ingest.Event(nil), events...)
	return nil
}

func (r *Repository) ListVideoMetricObservations(_ context.Context, hubUserID string) ([]ingest.VideoMetricObservation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return append([]ingest.VideoMetricObservation(nil), r.metricHistory[hubUserID]...), nil
}

func (r *Repository) AppendVideoMetricObservations(_ context.Context, hubUserID string, observations []ingest.VideoMetricObservation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing := make(map[string]struct{}, len(r.metricHistory[hubUserID]))
	for _, item := range r.metricHistory[hubUserID] {
		existing[item.ID] = struct{}{}
	}
	for _, item := range observations {
		if item.HubUserID == "" {
			item.HubUserID = hubUserID
		}
		if item.ID == "" {
			return errors.New("video metric observation id is required")
		}
		if item.HubUserID != hubUserID {
			return errors.New("video metric observation hub user id mismatch")
		}
		if _, ok := existing[item.ID]; ok {
			continue
		}
		r.metricHistory[hubUserID] = append(r.metricHistory[hubUserID], item)
		existing[item.ID] = struct{}{}
	}
	return nil
}

func (r *Repository) ListRuns(_ context.Context, hubUserID string) ([]ingest.Run, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return append([]ingest.Run(nil), r.runs[hubUserID]...), nil
}

func (r *Repository) AppendRun(_ context.Context, hubUserID string, run ingest.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.runs[hubUserID] = append(r.runs[hubUserID], run)
	return nil
}

func (r *Repository) ListOperatorRuns(_ context.Context, hubUserID string) ([]ingest.OperatorRun, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return append([]ingest.OperatorRun(nil), r.operatorRuns[hubUserID]...), nil
}

func (r *Repository) AppendOperatorRun(_ context.Context, hubUserID string, run ingest.OperatorRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.operatorRuns[hubUserID] = append(r.operatorRuns[hubUserID], run)
	return nil
}

func (r *Repository) ListBindings(_ context.Context, hubUserID string) ([]ingest.SourceBinding, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	items := append([]ingest.SourceBinding(nil), r.bindings[hubUserID]...)
	for index := range items {
		items[index] = ingest.NormalizeSourceBinding(items[index])
	}
	return items, nil
}

func (r *Repository) UpsertBinding(_ context.Context, hubUserID string, binding ingest.SourceBinding) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	targetKey := ingest.BindingTargetKey(binding)
	items := append([]ingest.SourceBinding(nil), r.bindings[hubUserID]...)
	for index, item := range items {
		if item.Source == binding.Source && ingest.BindingTargetKey(item) == targetKey {
			items[index] = ingest.NormalizeSourceBindingForUpsert(binding, item)
			r.bindings[hubUserID] = items
			return nil
		}
	}
	binding = ingest.NormalizeSourceBinding(binding)
	items = append(items, binding)
	r.bindings[hubUserID] = items
	return nil
}

func (r *Repository) DeleteBinding(_ context.Context, hubUserID, source, targetKey string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	items := append([]ingest.SourceBinding(nil), r.bindings[hubUserID]...)
	filtered := items[:0]
	deleted := false
	for _, item := range items {
		normalized := ingest.NormalizeSourceBinding(item)
		if normalized.Source == source && ingest.BindingTargetKey(normalized) == targetKey {
			deleted = true
			continue
		}
		filtered = append(filtered, item)
	}
	if !deleted {
		return ingest.ErrSourceBindingNotFound
	}
	r.bindings[hubUserID] = filtered
	return nil
}

func (r *Repository) Reset(_ context.Context, hubUserID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.items, hubUserID)
	delete(r.metricHistory, hubUserID)
	delete(r.runs, hubUserID)
	delete(r.operatorRuns, hubUserID)
	delete(r.bindings, hubUserID)
	return nil
}
