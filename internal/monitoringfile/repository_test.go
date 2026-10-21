package monitoringfile

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nobiyou/wx_channel_insight/internal/monitoring"
)

func TestRepositoryRecoversCorruptTaskFileAndPreservesEvidence(t *testing.T) {
	for _, corrupt := range [][]byte{bytes.Repeat([]byte{0}, 64), {}, []byte(`{"tasks":`), []byte(`null`)} {
		t.Run(fmt.Sprintf("bytes-%d", len(corrupt)), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "monitoring-tasks.json")
			good := []byte(`{"tasks":{},"runs":{}}`)
			if err := os.WriteFile(path+".bak", good, 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, corrupt, 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewRepository(path); err != nil {
				t.Fatalf("recover: %v", err)
			}
			restored, _ := os.ReadFile(path)
			if !bytes.Equal(restored, good) {
				t.Fatalf("unexpected restored data: %s", restored)
			}
			archives, _ := filepath.Glob(path + ".corrupt-*")
			if len(archives) != 1 {
				t.Fatalf("missing corruption evidence: %v", archives)
			}
			evidence, _ := os.ReadFile(archives[0])
			if !bytes.Equal(evidence, corrupt) {
				t.Fatal("corrupt original was changed")
			}
		})
	}
}

func TestRepositoryRejectsCorruptionWithoutValidBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	corrupt := []byte{0, 0, 0}
	if err := os.WriteFile(path, corrupt, 0600); err != nil {
		t.Fatal(err)
	}
	_, err := NewRepository(path)
	if err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("expected contextual error, got %v", err)
	}
	data, _ := os.ReadFile(path)
	if !bytes.Equal(data, corrupt) {
		t.Fatal("invalid file must remain intact")
	}
}

func TestRepositoryWritesValidatedBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tasks.json")
	repo, err := NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	task := monitoring.Task{ID: "backup", HubUserID: "owner", Name: "test", Status: monitoring.StatusPaused, ScopeMode: monitoring.ScopeAllSavedBindings, Interval: time.Hour}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	backup, err := os.ReadFile(path + ".bak")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{0}, 0600); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewRepository(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.GetTask(context.Background(), "owner", "backup"); err != nil {
		t.Fatal(err)
	}
	restored, _ := os.ReadFile(path)
	if !bytes.Equal(restored, backup) {
		t.Fatal("backup was not restored")
	}
}

func TestRepositoryPersistsTaskAndRunHistory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "monitoring-tasks.json")
	repo, err := NewRepository(path)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	now := time.Date(2026, 8, 9, 13, 0, 0, 0, time.UTC)
	task := monitoring.Task{
		ID:        "task-file-1",
		HubUserID: "operator-file",
		Name:      "saved feeds",
		Status:    monitoring.StatusEnabled,
		ScopeMode: monitoring.ScopeSelected,
		Interval:  5 * time.Minute,
		NextRunAt: now.Add(-time.Minute),
		Targets: []monitoring.TaskTarget{{BindingRef: monitoring.BindingRef{
			Source: "wx_feed_list", TargetKey: "account-1",
		}}},
	}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := repo.ClaimDueTask(context.Background(), task.HubUserID, "run-file-1", now, now.Add(time.Minute)); err != nil {
		t.Fatalf("claim task: %v", err)
	}
	renewedUntil := now.Add(2 * time.Minute)
	if err := repo.RenewTaskRunLease(context.Background(), task.HubUserID, "run-file-1", renewedUntil); err != nil {
		t.Fatalf("renew task lease: %v", err)
	}
	renewedRepo, err := NewRepository(path)
	if err != nil {
		t.Fatalf("reopen renewed repository: %v", err)
	}
	renewedRuns, err := renewedRepo.ListTaskRuns(context.Background(), task.HubUserID, task.ID, 10)
	if err != nil {
		t.Fatalf("load renewed history: %v", err)
	}
	if len(renewedRuns) != 1 || !renewedRuns[0].LeaseUntil.Equal(renewedUntil) {
		t.Fatalf("renewed lease was not persisted: %#v", renewedRuns)
	}
	if err := repo.CompleteTaskRun(context.Background(), task.HubUserID, "run-file-1", monitoring.TaskRunUpdate{
		Status:     monitoring.RunStatusPartial,
		FinishedAt: now.Add(time.Second),
		Message:    "one target skipped",
		Counts:     monitoring.TaskRunCounts{Attempted: 1, Succeeded: 0, Skipped: 1},
		NextRunAt:  now.Add(5 * time.Minute),
	}); err != nil {
		t.Fatalf("complete task: %v", err)
	}

	reopened, err := NewRepository(path)
	if err != nil {
		t.Fatalf("reopen repository: %v", err)
	}
	loaded, err := reopened.GetTask(context.Background(), task.HubUserID, task.ID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if loaded.LastStatus != monitoring.RunStatusPartial || len(loaded.Targets) != 1 {
		t.Fatalf("unexpected persisted task: %#v", loaded)
	}
	runs, err := reopened.ListTaskRuns(context.Background(), task.HubUserID, task.ID, 10)
	if err != nil {
		t.Fatalf("load history: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != monitoring.RunStatusPartial || runs[0].Counts.Skipped != 1 {
		t.Fatalf("unexpected persisted history: %#v", runs)
	}
}
