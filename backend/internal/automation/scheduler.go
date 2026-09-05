package automation

import (
	"context"
	"log"
	"time"

	"specpowers/backend/internal/cronexpr"
	"specpowers/backend/internal/domain"
)

// Scheduler fires cron-triggered autopilots when their next_run_at is due.
type Scheduler struct {
	autopilots autopilotStore
	svc        *Service
	now        func() time.Time
}

// autopilotStore is the narrow store surface the scheduler needs.
type autopilotStore interface {
	DueCronAutopilots(ctx context.Context, now time.Time) ([]domain.Autopilot, error)
	UpdateAutopilot(ctx context.Context, a *domain.Autopilot) (*domain.Autopilot, error)
}

func NewScheduler(autopilots autopilotStore, svc *Service) *Scheduler {
	return &Scheduler{autopilots: autopilots, svc: svc, now: time.Now}
}

// WithNow overrides the clock (tests).
func (s *Scheduler) WithNow(now func() time.Time) *Scheduler {
	s.now = now
	return s
}

// Tick fires every due cron autopilot once. A failing autopilot is logged
// and skipped; its next_run_at still advances so a permanently broken
// action cannot hot-loop the scheduler. A parse failure (should not happen
// — specs are validated at create/update) falls back to a one-hour delay.
func (s *Scheduler) Tick(ctx context.Context) error {
	now := s.now()
	due, err := s.autopilots.DueCronAutopilots(ctx, now)
	if err != nil {
		return err
	}
	for i := range due {
		a := &due[i]
		if s.svc != nil {
			if err := s.svc.executeAction(ctx, a, EventPayload{}); err != nil {
				log.Printf("automation: autopilot %s (%s) action failed: %v", a.ID, a.Name, err)
			} else {
				firedAt := now
				a.LastFiredAt = &firedAt
			}
		}
		next := nextCronTime(a.CronSpec, now)
		a.NextRunAt = &next
		if _, err := s.autopilots.UpdateAutopilot(ctx, a); err != nil {
			log.Printf("automation: autopilot %s reschedule failed: %v", a.ID, err)
		}
	}
	return nil
}

func nextCronTime(spec string, now time.Time) time.Time {
	sched, err := cronexpr.Parse(spec)
	if err != nil {
		log.Printf("automation: invalid cron spec %q: %v", spec, err)
		return now.Add(time.Hour)
	}
	return sched.Next(now)
}

// Loop runs Tick on a ticker until ctx is cancelled; tick errors are
// logged, never fatal.
func (s *Scheduler) Loop(ctx context.Context, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.Tick(ctx); err != nil {
				log.Printf("automation: scheduler tick failed: %v", err)
			}
		}
	}
}
