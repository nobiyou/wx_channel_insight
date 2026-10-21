package operator

import (
	"context"
	"errors"
)

var ErrCycleAlreadyRunning = errors.New("operator cycle is already running")

// CycleLocker coordinates a cycle across process boundaries when the runtime
// has a shared lock store. acquired=false is a non-blocking contention result.
type CycleLocker interface {
	TryAcquire(context.Context, string) (release func(), acquired bool, err error)
}
