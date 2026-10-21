package ingest

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestRawCommentJSONWriterWritesFilteredExportEnvelope(t *testing.T) {
	var buffer bytes.Buffer
	writer, err := NewRawCommentJSONWriter(&buffer, RawCommentExportMetadata{
		HubUserID: "hub:test", Total: 1, ExportedAt: time.Date(2026, 8, 23, 1, 2, 3, 0, time.UTC),
	}, RawCommentQuery{ObjectID: "video-1", Keyword: "问题"})
	if err != nil {
		t.Fatalf("create writer: %v", err)
	}
	if err := writer.Write(RawCommentRecord{VideoObjectID: "video-1", CommentID: "c1", Content: "问题内容"}); err != nil {
		t.Fatalf("write record: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	var payload struct {
		HubUserID string             `json:"hubUserId"`
		Total     int                `json:"total"`
		Filters   map[string]string  `json:"filters"`
		Items     []RawCommentRecord `json:"items"`
	}
	if err := json.Unmarshal(buffer.Bytes(), &payload); err != nil {
		t.Fatalf("decode export: %v body=%s", err, buffer.String())
	}
	if payload.HubUserID != "hub:test" || payload.Total != 1 || len(payload.Items) != 1 {
		t.Fatalf("unexpected export envelope: %+v", payload)
	}
	if payload.Filters["objectId"] != "video-1" || payload.Filters["q"] != "问题" {
		t.Fatalf("unexpected filters: %+v", payload.Filters)
	}
}
