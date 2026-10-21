package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func RunMigrationsFromDir(db *sql.DB, dir string) error {
	if db == nil {
		return errors.New("postgres db is required")
	}
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return errors.New("migration dir is required")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".sql") {
			files = append(files, filepath.Join(dir, name))
		}
	}
	sort.Strings(files)

	for _, path := range files {
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sqlText := strings.TrimSpace(string(payload))
		if sqlText == "" {
			continue
		}
		if err := ensureSchema(db, sqlText); err != nil {
			return fmt.Errorf("migration %s: %w", filepath.Base(path), err)
		}
	}

	return nil
}
