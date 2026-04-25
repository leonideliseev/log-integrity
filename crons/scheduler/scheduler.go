// Package scheduler provides a simple interval-based background scheduler.
package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Job is a scheduled function that receives the application context.
type Job func(ctx context.Context)

// Scheduler stores registered jobs and controls their lifecycle.
type Scheduler struct {
	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	jobs    []scheduledJob
}

type scheduledJob struct {
	name     string
	interval time.Duration
	job      Job
}

// New creates an empty in-process job scheduler.
func New() *Scheduler {
	return &Scheduler{}
}

// AddFunc registers a new periodic job before the scheduler is started.
func (s *Scheduler) AddFunc(name string, interval time.Duration, job Job) error {
	if interval <= 0 {
		return fmt.Errorf("scheduler: interval for %q must be positive", name)
	}
	if job == nil {
		return fmt.Errorf("scheduler: job for %q is nil", name)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("scheduler: cannot add job %q after start", name)
	}

	s.jobs = append(s.jobs, scheduledJob{name: name, interval: interval, job: job})
	return nil
}

// Start begins background execution for all registered jobs.
func (s *Scheduler) Start(parent context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return
	}

	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	s.started = true

	for _, item := range s.jobs {
		s.wg.Add(1)
		go s.runJob(ctx, item)
	}
}

// Stop cancels all running jobs and waits for them to finish.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	cancel := s.cancel
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	s.wg.Wait()

	s.mu.Lock()
	s.started = false
	s.cancel = nil
	s.mu.Unlock()
}

// Started reports whether the scheduler lifecycle has been started and not stopped yet.
func (s *Scheduler) Started() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started
}

// runJob executes one job immediately and then on every ticker interval.
func (s *Scheduler) runJob(ctx context.Context, item scheduledJob) {
	defer s.wg.Done()

	s.runJobSafely(ctx, item)

	ticker := time.NewTicker(item.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runJobSafely(ctx, item)
		}
	}
}

// runJobSafely prevents one panicking job from stopping its scheduler goroutine.
func (s *Scheduler) runJobSafely(ctx context.Context, item scheduledJob) {
	defer func() {
		if recovered := recover(); recovered != nil {
			slog.Default().Error("scheduled job panicked", "job", item.name, "panic", recovered)
		}
	}()
	item.job(ctx)
}

// ParseInterval converts a limited cron-like expression into a Go duration.
func ParseInterval(spec string) (time.Duration, error) {
	normalized := strings.TrimSpace(spec)
	if normalized == "" {
		return 0, fmt.Errorf("scheduler: empty schedule")
	}

	if strings.HasPrefix(normalized, "@every ") {
		return time.ParseDuration(strings.TrimSpace(strings.TrimPrefix(normalized, "@every ")))
	}

	switch normalized {
	case "*/5 * * * *":
		return 5 * time.Minute, nil
	case "0 * * * *":
		return time.Hour, nil
	case "0 */6 * * *":
		return 6 * time.Hour, nil
	case "0 0 * * *":
		return 24 * time.Hour, nil
	}

	fields := strings.Fields(normalized)
	if len(fields) != 5 {
		return 0, fmt.Errorf("scheduler: unsupported cron expression %q", spec)
	}

	if strings.HasPrefix(fields[0], "*/") && fields[1] == "*" && fields[2] == "*" && fields[3] == "*" && fields[4] == "*" {
		value, err := time.ParseDuration(strings.TrimPrefix(fields[0], "*/") + "m")
		if err != nil {
			return 0, fmt.Errorf("scheduler: parse minutes from %q: %w", spec, err)
		}
		return value, nil
	}

	if fields[0] == "0" && strings.HasPrefix(fields[1], "*/") && fields[2] == "*" && fields[3] == "*" && fields[4] == "*" {
		value, err := time.ParseDuration(strings.TrimPrefix(fields[1], "*/") + "h")
		if err != nil {
			return 0, fmt.Errorf("scheduler: parse hours from %q: %w", spec, err)
		}
		return value, nil
	}

	return 0, fmt.Errorf("scheduler: unsupported cron expression %q", spec)
}
