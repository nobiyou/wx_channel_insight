package ingest

import (
	"context"
	"io"
)

type RawCommentRepository interface {
	Upsert(ctx context.Context, hubUserID string, items []RawCommentRecord) error
	List(ctx context.Context, hubUserID string, query RawCommentQuery) (RawCommentPage, error)
}

// RawCommentExporter is an optional capability for repositories that can
// stream an explicit JSON export without materializing the full comment set.
type RawCommentExporter interface {
	ExportJSON(ctx context.Context, hubUserID string, query RawCommentQuery, dst io.Writer) error
}

type VideoCommentSyncRepository interface {
	GetState(ctx context.Context, hubUserID, accountID, videoObjectID string) (VideoCommentSyncState, bool, error)
	UpsertState(ctx context.Context, state VideoCommentSyncState) error
}
