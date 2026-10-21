package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"github.com/nobiyou/wx_channel_insight/internal/ingest"
)

type IngestRepository struct{ c *Client }

func NewIngestRepository(c *Client) (*IngestRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &IngestRepository{c}, nil
}

func (r *IngestRepository) ListEvents(ctx context.Context, owner string) ([]ingest.Event, error) {
	return list[ingest.Event](ctx, r.c.db, "events", owner)
}
func appendEvents(ctx context.Context, q queryer, owner string, items []ingest.Event) error {
	for _, v := range items {
		if err := appendDocument(ctx, q, "events", owner, v); err != nil {
			return err
		}
	}
	return nil
}
func (r *IngestRepository) AppendEvents(ctx context.Context, owner string, items []ingest.Event) error {
	return r.c.update(ctx, func(tx *sql.Tx) error { return appendEvents(ctx, tx, owner, items) })
}
func (r *IngestRepository) ReplaceEvents(ctx context.Context, owner string, items []ingest.Event) error {
	return r.c.update(ctx, func(tx *sql.Tx) error {
		if err := clear(ctx, tx, "events", owner); err != nil {
			return err
		}
		return appendEvents(ctx, tx, owner, items)
	})
}
func (r *IngestRepository) ListVideoMetricObservations(ctx context.Context, owner string) ([]ingest.VideoMetricObservation, error) {
	return list[ingest.VideoMetricObservation](ctx, r.c.db, "metrics", owner)
}
func (r *IngestRepository) AppendVideoMetricObservations(ctx context.Context, owner string, items []ingest.VideoMetricObservation) error {
	return r.c.update(ctx, func(tx *sql.Tx) error {
		for _, v := range items {
			if v.ID == "" {
				return errors.New("video metric observation id is required")
			}
			if v.HubUserID == "" {
				v.HubUserID = owner
			}
			if v.HubUserID != owner {
				return errors.New("video metric observation hub user id mismatch")
			}
			if _, found, err := get[ingest.VideoMetricObservation](ctx, tx, "metrics", owner, v.ID); err != nil {
				return err
			} else if found {
				continue
			}
			if err := put(ctx, tx, "metrics", owner, v.ID, v); err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *IngestRepository) ListRuns(ctx context.Context, owner string) ([]ingest.Run, error) {
	return list[ingest.Run](ctx, r.c.db, "ingest_runs", owner)
}
func (r *IngestRepository) AppendRun(ctx context.Context, owner string, run ingest.Run) error {
	return appendDocument(ctx, r.c.db, "ingest_runs", owner, run)
}
func (r *IngestRepository) ListOperatorRuns(ctx context.Context, owner string) ([]ingest.OperatorRun, error) {
	return list[ingest.OperatorRun](ctx, r.c.db, "operator_runs", owner)
}
func (r *IngestRepository) AppendOperatorRun(ctx context.Context, owner string, run ingest.OperatorRun) error {
	return appendDocument(ctx, r.c.db, "operator_runs", owner, run)
}
func (r *IngestRepository) ListBindings(ctx context.Context, owner string) ([]ingest.SourceBinding, error) {
	return list[ingest.SourceBinding](ctx, r.c.db, "bindings", owner)
}
func compositeKey(parts ...string) string { data, _ := json.Marshal(parts); return string(data) }
func (r *IngestRepository) UpsertBinding(ctx context.Context, owner string, binding ingest.SourceBinding) error {
	id := compositeKey(binding.Source, ingest.BindingTargetKey(binding))
	return r.c.update(ctx, func(tx *sql.Tx) error {
		existing, found, err := get[ingest.SourceBinding](ctx, tx, "bindings", owner, id)
		if err != nil {
			return err
		}
		if found {
			binding = ingest.NormalizeSourceBindingForUpsert(binding, existing)
		} else {
			binding = ingest.NormalizeSourceBinding(binding)
		}
		return put(ctx, tx, "bindings", owner, id, binding)
	})
}
func (r *IngestRepository) DeleteBinding(ctx context.Context, owner, source, target string) error {
	found, err := remove(ctx, r.c.db, "bindings", owner, compositeKey(source, target))
	if err == nil && !found {
		return ingest.ErrSourceBindingNotFound
	}
	return err
}
func (r *IngestRepository) Reset(ctx context.Context, owner string) error {
	return r.c.update(ctx, func(tx *sql.Tx) error {
		for _, bucket := range []string{"events", "metrics", "ingest_runs", "operator_runs", "bindings"} {
			if err := clear(ctx, tx, bucket, owner); err != nil {
				return err
			}
		}
		return nil
	})
}

type CommentRepository struct{ c *Client }

func NewCommentRepository(c *Client) (*CommentRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &CommentRepository{c}, nil
}
func (r *CommentRepository) Upsert(ctx context.Context, owner string, items []ingest.RawCommentRecord) error {
	return r.c.update(ctx, func(tx *sql.Tx) error {
		for _, item := range items {
			if strings.TrimSpace(item.VideoObjectID) == "" || strings.TrimSpace(item.CommentID) == "" {
				continue
			}
			item.HubUserID = owner
			if err := put(ctx, tx, "comments", owner, compositeKey(item.VideoObjectID, item.CommentID), item); err != nil {
				return err
			}
		}
		return nil
	})
}
func (r *CommentRepository) List(ctx context.Context, owner string, query ingest.RawCommentQuery) (ingest.RawCommentPage, error) {
	items, err := list[ingest.RawCommentRecord](ctx, r.c.db, "comments", owner)
	if err != nil {
		return ingest.RawCommentPage{}, err
	}
	return ingest.PageRawComments(items, query), nil
}
func (r *CommentRepository) GetState(ctx context.Context, owner, account, video string) (ingest.VideoCommentSyncState, bool, error) {
	return get[ingest.VideoCommentSyncState](ctx, r.c.db, "comment_sync", owner, compositeKey(account, video))
}
func (r *CommentRepository) UpsertState(ctx context.Context, state ingest.VideoCommentSyncState) error {
	return put(ctx, r.c.db, "comment_sync", state.HubUserID, compositeKey(state.AccountID, state.VideoObjectID), state)
}

var (
	_ ingest.Repository                 = (*IngestRepository)(nil)
	_ ingest.RawCommentRepository       = (*CommentRepository)(nil)
	_ ingest.VideoCommentSyncRepository = (*CommentRepository)(nil)
)
