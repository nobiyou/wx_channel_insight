package postgres

import (
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRunMigrationsFromDirRejectsMissingDB(t *testing.T) {
	err := RunMigrationsFromDir(nil, "migrations/postgres")
	if err == nil {
		t.Fatalf("expected missing db error")
	}
}

func TestRunMigrationsFromDirRejectsMissingDir(t *testing.T) {
	err := RunMigrationsFromDir(&sql.DB{}, "")
	if err == nil {
		t.Fatalf("expected missing dir error")
	}
}

func TestRunMigrationsFromDirFailsWhenDirDoesNotExist(t *testing.T) {
	err := RunMigrationsFromDir(&sql.DB{}, filepath.Join(os.TempDir(), "missing-wx-channel-insight-migrations"))
	if err == nil {
		t.Fatalf("expected missing directory error")
	}
}

func TestAIAnalysisMigrationIsPresentForDirectoryDiscovery(t *testing.T) {
	_, sourcePath, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("could not resolve migrations test source path")
	}
	path := filepath.Join(filepath.Dir(sourcePath), "..", "..", "migrations", "postgres", "0028_ai_analysis_runs.sql")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected AI analysis migration to be discoverable at %q: %v", path, err)
	}
}
