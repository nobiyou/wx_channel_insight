package ingest

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var errWXCommentExportNoUsableFile = errors.New("no usable wx comment export file")

type wxCommentExportPathResolution struct {
	ResolvedPath string
	PersistPath  string
}

type wxCommentExportCandidate struct {
	Path      string
	ObjectID  string
	Timestamp time.Time
}

func resolveWXCommentExportPath(path string) (wxCommentExportPathResolution, error) {
	normalizedPath := normalizeBindingPath(path)
	if normalizedPath == "" {
		return wxCommentExportPathResolution{}, os.ErrNotExist
	}

	info, err := os.Stat(normalizedPath)
	switch {
	case err == nil && info.IsDir():
		candidate, findErr := findLatestWXCommentExportCandidate(normalizedPath, "")
		if findErr != nil {
			return wxCommentExportPathResolution{}, findErr
		}
		return wxCommentExportPathResolution{
			ResolvedPath: candidate.Path,
			PersistPath:  normalizedPath,
		}, nil
	case err == nil:
		root := findWXCommentExportRoot(normalizedPath)
		if root == "" {
			return wxCommentExportPathResolution{
				ResolvedPath: normalizedPath,
				PersistPath:  normalizedPath,
			}, nil
		}

		current, currentErr := loadWXCommentExportCandidate(normalizedPath)
		if currentErr == nil {
			newer, newerErr := findLatestWXCommentExportCandidate(root, current.ObjectID)
			if newerErr == nil && newer.Path != "" && newer.Timestamp.After(current.Timestamp) {
				return wxCommentExportPathResolution{
					ResolvedPath: newer.Path,
					PersistPath:  newer.Path,
				}, nil
			}
			return wxCommentExportPathResolution{
				ResolvedPath: normalizedPath,
				PersistPath:  normalizedPath,
			}, nil
		}

		fallback, fallbackErr := findLatestWXCommentExportCandidate(root, "")
		if fallbackErr != nil {
			return wxCommentExportPathResolution{}, currentErr
		}
		return wxCommentExportPathResolution{
			ResolvedPath: fallback.Path,
			PersistPath:  root,
		}, nil
	case errors.Is(err, os.ErrNotExist):
		root := findWXCommentExportRoot(normalizedPath)
		if root == "" {
			return wxCommentExportPathResolution{}, err
		}
		candidate, findErr := findLatestWXCommentExportCandidate(root, "")
		if findErr != nil {
			return wxCommentExportPathResolution{}, err
		}
		return wxCommentExportPathResolution{
			ResolvedPath: candidate.Path,
			PersistPath:  root,
		}, nil
	default:
		return wxCommentExportPathResolution{}, err
	}
}

func resolveWXCommentExportPathWithRoot(path, dataRoot string) (wxCommentExportPathResolution, error) {
	resolution, err := resolveWXCommentExportPath(path)
	if err == nil || strings.TrimSpace(dataRoot) == "" {
		return resolution, err
	}

	mappedPath := mapWXCommentExportPathToRoot(path, dataRoot)
	if mappedPath == "" || strings.EqualFold(mappedPath, normalizeBindingPath(path)) {
		return resolution, err
	}
	mappedResolution, mappedErr := resolveWXCommentExportPath(mappedPath)
	if mappedErr != nil {
		return resolution, err
	}
	if mappedResolution.PersistPath == "" {
		mappedResolution.PersistPath = normalizeBindingPath(mappedPath)
	}
	return mappedResolution, nil
}

func mapWXCommentExportPathToRoot(path, dataRoot string) string {
	root := strings.TrimSpace(dataRoot)
	if root == "" {
		return ""
	}
	root = normalizeBindingPath(root)
	normalized := strings.ReplaceAll(strings.TrimSpace(path), "\\", "/")
	lower := strings.ToLower(normalized)
	const marker = "comment_data/"
	if index := strings.Index(lower, marker); index >= 0 {
		suffix := strings.TrimPrefix(normalized[index+len(marker):], "/")
		if suffix == "" {
			return root
		}
		return filepath.Join(root, filepath.FromSlash(suffix))
	}
	if strings.HasSuffix(lower, "comment_data") {
		return root
	}
	return ""
}

func findLatestWXCommentExportCandidate(root, preferredObjectID string) (wxCommentExportCandidate, error) {
	candidates := make([]wxCommentExportCandidate, 0, 8)
	var firstErr error
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if firstErr == nil {
				firstErr = walkErr
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		name := strings.ToLower(entry.Name())
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".partial.json") {
			return nil
		}

		candidate, candidateErr := loadWXCommentExportCandidate(path)
		if candidateErr != nil {
			if firstErr == nil {
				firstErr = candidateErr
			}
			return nil
		}
		candidates = append(candidates, candidate)
		return nil
	})
	if err != nil && firstErr == nil {
		firstErr = err
	}
	if len(candidates) == 0 {
		if firstErr != nil {
			return wxCommentExportCandidate{}, firstErr
		}
		return wxCommentExportCandidate{}, errWXCommentExportNoUsableFile
	}

	bestOverall := candidates[0]
	var bestPreferred wxCommentExportCandidate
	bestPreferredFound := false
	for _, candidate := range candidates[1:] {
		if wxCommentExportCandidateAfter(candidate, bestOverall) {
			bestOverall = candidate
		}
	}
	if strings.TrimSpace(preferredObjectID) != "" {
		for _, candidate := range candidates {
			if candidate.ObjectID != preferredObjectID {
				continue
			}
			if !bestPreferredFound || wxCommentExportCandidateAfter(candidate, bestPreferred) {
				bestPreferred = candidate
				bestPreferredFound = true
			}
		}
		if bestPreferredFound {
			return bestPreferred, nil
		}
	}
	return bestOverall, nil
}

func loadWXCommentExportCandidate(path string) (wxCommentExportCandidate, error) {
	info, err := os.Stat(path)
	if err != nil {
		return wxCommentExportCandidate{}, err
	}
	if info.IsDir() {
		return wxCommentExportCandidate{}, errWXCommentExportNoUsableFile
	}

	payload, err := LoadWXCommentExportFile(path)
	if err != nil {
		return wxCommentExportCandidate{}, err
	}

	timestamp := info.ModTime()
	if parsed, ok := parseWXCommentExportTime(payload.SavedAt); ok {
		timestamp = parsed
	}
	return wxCommentExportCandidate{
		Path:      normalizeBindingPath(path),
		ObjectID:  strings.TrimSpace(payload.ObjectID),
		Timestamp: timestamp,
	}, nil
}

func wxCommentExportCandidateAfter(left, right wxCommentExportCandidate) bool {
	if left.Timestamp.Equal(right.Timestamp) {
		return left.Path > right.Path
	}
	return left.Timestamp.After(right.Timestamp)
}

func findWXCommentExportRoot(path string) string {
	path = normalizeBindingPath(path)
	if path == "" {
		return ""
	}

	current := path
	if filepath.Ext(current) != "" {
		current = filepath.Dir(current)
	}
	for {
		if strings.EqualFold(filepath.Base(current), "comment_data") {
			return current
		}
		next := filepath.Dir(current)
		if next == current {
			return ""
		}
		current = next
	}
}
