package monitoring

import (
	"context"
	"testing"
	"time"
)

func TestDefaultTaskExcludesManagedTargetsEvenWhenPaused(t *testing.T) {
	now := time.Date(2026, 10, 12, 0, 0, 0, 0, time.UTC)
	a := TaskTarget{BindingRef: BindingRef{Source: "wx_video_search", TargetKey: "AI"}}
	b := TaskTarget{BindingRef: BindingRef{Source: "wx_video_search", TargetKey: "运营"}}
	resolver := func(_ context.Context, _ string, refs []BindingRef) ([]TaskTarget, error) {
		if len(refs) == 0 {
			return []TaskTarget{a, b}, nil
		}
		resolved := make([]TaskTarget, 0, len(refs))
		for _, ref := range refs {
			resolved = append(resolved, TaskTarget{BindingRef: ref})
		}
		return resolved, nil
	}
	service, err := NewService(NewMemoryRepository(), ServiceConfig{Now: func() time.Time { return now }, BindingResolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	runner := &testOperatorRunner{results: []OperatorRunResult{{Status: RunStatusSuccess, FinalStatus: RunStatusSuccess, Attempted: 1, Succeeded: 1}}}
	service.SetOperatorRunner(runner)
	_, err = service.CreateTask(context.Background(), "owner", Task{ID: "object", Name: "AI", Status: StatusPaused, ScopeMode: ScopeSelected, Interval: time.Hour, Targets: []TaskTarget{a}})
	if err != nil {
		t.Fatal(err)
	}
	def, _, err := service.EnsureDefaultTask(context.Background(), "owner")
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.RunManualTask(context.Background(), "owner", def.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.options) != 1 || len(runner.options[0].TargetRefs) != 1 || runner.options[0].TargetRefs[0] != b.BindingRef {
		t.Fatalf("default must only collect unmanaged objects: %+v", runner.options)
	}
	paused, err := service.GetTask(context.Background(), "owner", "object")
	if err != nil {
		t.Fatal(err)
	}
	paused.Targets = []TaskTarget{a, b}
	_, err = service.UpdateTask(context.Background(), "owner", paused)
	if err != nil {
		t.Fatal(err)
	}
	// The test resolver supplies both managed refs for this update.
	run, err := service.RunManualTask(context.Background(), "owner", def.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runner.options) != 1 || run.Status != RunStatusSkipped {
		t.Fatalf("empty default scope must skip instead of collecting every binding: run=%+v calls=%+v", run, runner.options)
	}
}
