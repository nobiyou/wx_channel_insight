package ingestfile

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

type Repository struct {
	mu            sync.RWMutex
	path          string
	items         map[string][]ingest.Event
	metricHistory map[string][]ingest.VideoMetricObservation
	runs          map[string][]ingest.Run
	operatorRuns  map[string][]ingest.OperatorRun
	bindings      map[string][]ingest.SourceBinding
}

type fileState struct {
	Items         map[string][]ingest.Event                  `json:"items"`
	MetricHistory map[string][]ingest.VideoMetricObservation `json:"metricHistory,omitempty"`
	Runs          map[string][]ingest.Run                    `json:"runs,omitempty"`
	OperatorRuns  map[string][]ingest.OperatorRun            `json:"operatorRuns,omitempty"`
	Bindings      map[string][]ingest.SourceBinding          `json:"bindings,omitempty"`
}

func NewRepository(path string) (*Repository, error) {
	if path == "" {
		return nil, errors.New("ingest repository path is required")
	}

	repo := &Repository{
		path:          path,
		items:         map[string][]ingest.Event{},
		metricHistory: map[string][]ingest.VideoMetricObservation{},
		runs:          map[string][]ingest.Run{},
		operatorRuns:  map[string][]ingest.OperatorRun{},
		bindings:      map[string][]ingest.SourceBinding{},
	}
	if err := repo.load(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *Repository) ListEvents(_ context.Context, hubUserID string) ([]ingest.Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	return append([]ingest.Event(nil), r.items[hubUserID]...), nil
}

func (r *Repository) AppendEvents(_ context.Context, hubUserID string, events []ingest.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.items[hubUserID] = append(r.items[hubUserID], events...)
	return r.persist()
}

func (r *Repository) ReplaceEvents(_ context.Context, hubUserID string, events []ingest.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.items[hubUserID] = append([]ingest.Event(nil), events...)
	return r.persist()
}

func (r *Repository) ListVideoMetricObservations(_ context.Context, hubUserID string) ([]ingest.VideoMetricObservation, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	return append([]ingest.VideoMetricObservation(nil), r.metricHistory[hubUserID]...), nil
}

func (r *Repository) AppendVideoMetricObservations(_ context.Context, hubUserID string, observations []ingest.VideoMetricObservation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
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
	return r.persist()
}

func (r *Repository) ListRuns(_ context.Context, hubUserID string) ([]ingest.Run, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	return append([]ingest.Run(nil), r.runs[hubUserID]...), nil
}

func (r *Repository) AppendRun(_ context.Context, hubUserID string, run ingest.Run) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.runs[hubUserID] = append(r.runs[hubUserID], run)
	return r.persist()
}

func (r *Repository) ListOperatorRuns(_ context.Context, hubUserID string) ([]ingest.OperatorRun, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	return append([]ingest.OperatorRun(nil), r.operatorRuns[hubUserID]...), nil
}

func (r *Repository) AppendOperatorRun(_ context.Context, hubUserID string, run ingest.OperatorRun) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	r.operatorRuns[hubUserID] = append(r.operatorRuns[hubUserID], run)
	return r.persist()
}

func (r *Repository) ListBindings(_ context.Context, hubUserID string) ([]ingest.SourceBinding, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return nil, err
	}
	items := append([]ingest.SourceBinding(nil), r.bindings[hubUserID]...)
	for index := range items {
		items[index] = ingest.NormalizeSourceBinding(items[index])
	}
	return items, nil
}

func (r *Repository) UpsertBinding(_ context.Context, hubUserID string, binding ingest.SourceBinding) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	targetKey := ingest.BindingTargetKey(binding)
	items := append([]ingest.SourceBinding(nil), r.bindings[hubUserID]...)
	for index, item := range items {
		if item.Source == binding.Source && ingest.BindingTargetKey(item) == targetKey {
			items[index] = ingest.NormalizeSourceBindingForUpsert(binding, item)
			r.bindings[hubUserID] = items
			return r.persist()
		}
		if item.Source == binding.Source && ingest.BindingTargetKey(item) == "" && targetKey == "" {
			items[index] = ingest.NormalizeSourceBindingForUpsert(binding, item)
			r.bindings[hubUserID] = items
			return r.persist()
		}
	}
	binding = ingest.NormalizeSourceBinding(binding)
	items = append(items, binding)
	r.bindings[hubUserID] = items
	return r.persist()
}

func (r *Repository) DeleteBinding(_ context.Context, hubUserID, source, targetKey string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
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
	return r.persist()
}

func (r *Repository) Reset(_ context.Context, hubUserID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.loadLocked(); err != nil {
		return err
	}
	delete(r.items, hubUserID)
	delete(r.metricHistory, hubUserID)
	delete(r.runs, hubUserID)
	delete(r.operatorRuns, hubUserID)
	delete(r.bindings, hubUserID)
	return r.persist()
}

func (r *Repository) load() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.loadLocked()
}

func (r *Repository) loadLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o755); err != nil {
		return err
	}

	data, err := os.ReadFile(r.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}

	var state fileState
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.Items != nil {
		r.items = state.Items
	} else {
		r.items = map[string][]ingest.Event{}
	}
	if state.MetricHistory != nil {
		r.metricHistory = state.MetricHistory
	} else {
		r.metricHistory = map[string][]ingest.VideoMetricObservation{}
	}
	if state.Runs != nil {
		r.runs = state.Runs
	} else {
		r.runs = map[string][]ingest.Run{}
	}
	if state.OperatorRuns != nil {
		r.operatorRuns = state.OperatorRuns
	} else {
		r.operatorRuns = map[string][]ingest.OperatorRun{}
	}
	if state.Bindings != nil {
		r.bindings = state.Bindings
	} else {
		r.bindings = map[string][]ingest.SourceBinding{}
	}
	return nil
}

func (r *Repository) persist() error {
	state := fileState{
		Items:         r.items,
		MetricHistory: r.metricHistory,
		Runs:          r.runs,
		OperatorRuns:  r.operatorRuns,
		Bindings:      r.bindings,
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := r.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpPath, r.path)
}
