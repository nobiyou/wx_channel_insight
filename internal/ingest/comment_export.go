package ingest

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
)

type RawCommentExportMetadata struct {
	ExportedAt time.Time `json:"exportedAt"`
	HubUserID  string    `json:"hubUserId"`
	Total      int       `json:"total"`
	Filters    any       `json:"filters,omitempty"`
}

// RawCommentJSONWriter writes a bounded-memory export envelope. Repositories
// can feed it one record at a time while keeping the database as the source.
type RawCommentJSONWriter struct {
	dst    io.Writer
	first  bool
	closed bool
}

func NewRawCommentJSONWriter(dst io.Writer, metadata RawCommentExportMetadata, query RawCommentQuery) (*RawCommentJSONWriter, error) {
	if dst == nil {
		return nil, fmt.Errorf("comment export writer is required")
	}
	if metadata.ExportedAt.IsZero() {
		metadata.ExportedAt = time.Now().UTC()
	}
	metadata.ExportedAt = metadata.ExportedAt.UTC()
	metadata.HubUserID = strings.TrimSpace(metadata.HubUserID)
	metadata.Filters = rawCommentExportFilters(query)
	header, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}
	if len(header) == 0 || header[len(header)-1] != '}' {
		return nil, fmt.Errorf("invalid comment export metadata")
	}
	header = append(header[:len(header)-1], []byte(`,"items":[`)...)
	if _, err := dst.Write(header); err != nil {
		return nil, err
	}
	return &RawCommentJSONWriter{dst: dst, first: true}, nil
}

func (w *RawCommentJSONWriter) Write(item RawCommentRecord) error {
	if w == nil || w.closed {
		return fmt.Errorf("comment export writer is closed")
	}
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	if !w.first {
		if _, err := io.WriteString(w.dst, ","); err != nil {
			return err
		}
	}
	if _, err := w.dst.Write(payload); err != nil {
		return err
	}
	w.first = false
	return nil
}

func (w *RawCommentJSONWriter) Close() error {
	if w == nil || w.closed {
		return nil
	}
	w.closed = true
	_, err := io.WriteString(w.dst, "]}\n")
	return err
}

func rawCommentExportFilters(query RawCommentQuery) map[string]string {
	filters := map[string]string{}
	if value := strings.TrimSpace(query.ObjectID); value != "" {
		filters["objectId"] = value
	}
	if value := strings.TrimSpace(query.AccountID); value != "" {
		filters["accountId"] = value
	}
	if value := strings.TrimSpace(query.AccountKeyword); value != "" {
		filters["account"] = value
	}
	if value := strings.TrimSpace(query.Source); value != "" {
		filters["source"] = value
	}
	if value := strings.TrimSpace(query.Keyword); value != "" {
		filters["q"] = value
	}
	if !query.From.IsZero() {
		filters["from"] = query.From.UTC().Format(time.RFC3339)
	}
	if !query.To.IsZero() {
		filters["to"] = query.To.UTC().Format(time.RFC3339)
	}
	if len(filters) == 0 {
		return nil
	}
	return filters
}
