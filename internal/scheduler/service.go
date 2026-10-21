package scheduler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// CycleRunner is the only business dependency required by the scheduler.
type CycleRunner interface {
	RunCycle(context.Context) error
}

// WaitFunc waits for the next scheduled cycle. It receives the service context
// so cancellation can interrupt the wait without relying on wall-clock sleep.
type WaitFunc func(context.Context, time.Duration) error

type ServiceOption func(*Service)

func WithWait(wait WaitFunc) ServiceOption {
	return func(service *Service) {
		if wait != nil {
			service.wait = wait
		}
	}
}

func WithWaitFunc(wait WaitFunc) ServiceOption {
	return WithWait(wait)
}

func WithLogger(logger *log.Logger) ServiceOption {
	return func(service *Service) {
		if logger != nil {
			service.logger = logger
		}
	}
}

type Service struct {
	runner     CycleRunner
	interval   time.Duration
	runOnStart bool
	wait       WaitFunc
	logger     *log.Logger
}

func NewService(runner CycleRunner, cfg Config, options ...ServiceOption) *Service {
	service := &Service{
		runner:     runner,
		interval:   cfg.Interval,
		runOnStart: cfg.RunOnStart,
		wait:       waitForInterval,
		logger:     log.Default(),
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

func (s *Service) Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("scheduler context is nil")
	}
	if s == nil {
		return errors.New("scheduler service is nil")
	}
	if s.runner == nil {
		return errors.New("scheduler cycle runner is nil")
	}
	if s.interval <= 0 {
		return fmt.Errorf("scheduler interval must be positive, got %s", s.interval)
	}
	if s.wait == nil {
		return errors.New("scheduler wait function is nil")
	}

	if !s.runOnStart {
		if err := s.wait(ctx, s.interval); err != nil {
			return waitError(ctx, err)
		}
	}

	for {
		if ctx.Err() != nil {
			return nil
		}

		s.logger.Printf("scheduler cycle starting")
		runErr := s.runner.RunCycle(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if runErr != nil {
			s.logger.Printf("scheduler cycle failed: %v", runErr)
		} else {
			s.logger.Printf("scheduler cycle succeeded")
		}

		if err := s.wait(ctx, s.interval); err != nil {
			return waitError(ctx, err)
		}
	}
}

func waitForInterval(ctx context.Context, interval time.Duration) error {
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func waitError(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
