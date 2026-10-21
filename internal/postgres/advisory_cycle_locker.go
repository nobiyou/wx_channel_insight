package postgres

import (
	"context"
	"errors"
	"sync"
)

const OperatorCycleLockNamespace = "insight-operator-cycle"

type AdvisoryCycleLocker struct {
	client *Client
}

func NewAdvisoryCycleLocker(client *Client) (*AdvisoryCycleLocker, error) {
	if client == nil || client.DB() == nil {
		return nil, errors.New("postgres client is required for advisory cycle locker")
	}
	return &AdvisoryCycleLocker{client: client}, nil
}

func (l *AdvisoryCycleLocker) TryAcquire(ctx context.Context, key string) (func(), bool, error) {
	if l == nil || l.client == nil || l.client.DB() == nil {
		return nil, false, errors.New("postgres client is required for advisory cycle locker")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	tx, err := l.client.DB().BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	acquired, err := TryAcquireWorkspaceAdvisoryLock(ctx, tx, OperatorCycleLockNamespace, key)
	if err != nil {
		_ = tx.Rollback()
		return nil, false, err
	}
	if !acquired {
		_ = tx.Rollback()
		return nil, false, nil
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			_ = tx.Rollback()
		})
	}, true, nil
}
