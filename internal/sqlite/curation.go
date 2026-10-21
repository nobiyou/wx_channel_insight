package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/nobiyou/wx_channel_insight/internal/insight"
)

type DataCenterExclusionRepository struct{ c *Client }

func NewDataCenterExclusionRepository(c *Client) (*DataCenterExclusionRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &DataCenterExclusionRepository{c}, nil
}
func (r *DataCenterExclusionRepository) List(ctx context.Context, scope string) ([]insight.DataCenterExclusion, error) {
	items, err := list[insight.DataCenterExclusion](ctx, r.c.db, "exclusions", scope)
	sort.SliceStable(items, func(i, j int) bool { return items[i].DeletedAt.After(items[j].DeletedAt) })
	return items, err
}
func (r *DataCenterExclusionRepository) Upsert(ctx context.Context, v insight.DataCenterExclusion) (saved insight.DataCenterExclusion, err error) {
	err = r.c.update(ctx, func(tx *sql.Tx) error {
		id := compositeKey(string(v.Entity), v.EntityID)
		existing, found, err := get[insight.DataCenterExclusion](ctx, tx, "exclusions", v.WorkspaceID, id)
		if err != nil {
			return err
		}
		if found {
			saved = existing
			return nil
		}
		saved = v
		return put(ctx, tx, "exclusions", v.WorkspaceID, id, v)
	})
	return
}
func (r *DataCenterExclusionRepository) Restore(ctx context.Context, scope string, entity insight.DataCenterEntity, id string) (bool, error) {
	return remove(ctx, r.c.db, "exclusions", scope, compositeKey(string(entity), id))
}

type VideoCurationRepository struct{ c *Client }

func NewVideoCurationRepository(c *Client) (*VideoCurationRepository, error) {
	if err := requireClient(c); err != nil {
		return nil, err
	}
	return &VideoCurationRepository{c}, nil
}
func listCurations(ctx context.Context, q queryer, scope string) ([]insight.VideoCuration, error) {
	items, err := list[insight.VideoCuration](ctx, q, "curations", scope)
	for i := range items {
		items[i].WorkspaceID = scope
	}
	sort.Slice(items, func(i, j int) bool { return items[i].VideoID < items[j].VideoID })
	return items, err
}
func (r *VideoCurationRepository) List(ctx context.Context, scope string) ([]insight.VideoCuration, error) {
	return listCurations(ctx, r.c.db, scope)
}
func saveCurations(ctx context.Context, q queryer, items []insight.VideoCuration) error {
	for _, v := range items {
		if err := put(ctx, q, "curations", v.WorkspaceID, v.VideoID, v); err != nil {
			return err
		}
	}
	return nil
}
func (r *VideoCurationRepository) SaveBatch(ctx context.Context, items []insight.VideoCuration) error {
	return r.c.update(ctx, func(tx *sql.Tx) error { return saveCurations(ctx, tx, items) })
}
func (r *VideoCurationRepository) LoadOperations(ctx context.Context, scope string) (insight.OperationsState, error) {
	v, _, err := get[insight.OperationsState](ctx, r.c.db, "operations", scope, "state")
	return v, err
}
func (r *VideoCurationRepository) MutateOperations(ctx context.Context, scope string, fn insight.OperationsMutation) error {
	return r.mutate(ctx, scope, true, fn)
}
func (r *VideoCurationRepository) MutateCurations(ctx context.Context, scope string, fn func(map[string]insight.VideoCuration) ([]insight.VideoCuration, error)) error {
	return r.mutate(ctx, scope, false, func(_ *insight.OperationsState, items map[string]insight.VideoCuration) ([]insight.VideoCuration, error) {
		return fn(items)
	})
}
func (r *VideoCurationRepository) mutate(ctx context.Context, scope string, persistState bool, fn insight.OperationsMutation) error {
	return r.c.update(ctx, func(tx *sql.Tx) error {
		state, _, err := get[insight.OperationsState](ctx, tx, "operations", scope, "state")
		if err != nil {
			return err
		}
		items, err := listCurations(ctx, tx, scope)
		if err != nil {
			return err
		}
		current := map[string]insight.VideoCuration{}
		for _, v := range items {
			current[v.VideoID] = v
		}
		writes, err := fn(&state, current)
		if err != nil {
			return err
		}
		for _, v := range writes {
			if v.WorkspaceID != scope {
				return errors.New("curation write outside locked workspace")
			}
		}
		if err := saveCurations(ctx, tx, writes); err != nil {
			return err
		}
		if persistState {
			return put(ctx, tx, "operations", scope, "state", state)
		}
		return nil
	})
}

var (
	_ insight.DataCenterExclusionRepository = (*DataCenterExclusionRepository)(nil)
	_ insight.VideoCurationRepository       = (*VideoCurationRepository)(nil)
	_ insight.OperationsRepository          = (*VideoCurationRepository)(nil)
	_ insight.AtomicCurationRepository      = (*VideoCurationRepository)(nil)
)
