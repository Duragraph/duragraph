package service

import (
	"context"
	"fmt"
	"log"
	"time"

	cronlib "github.com/robfig/cron/v3"

	"github.com/duragraph/duragraph/internal/infrastructure/persistence/postgres"
)

// CronScheduler polls for due cron jobs and triggers run creation.
type CronScheduler struct {
	cronRepo *postgres.CronRepository
	interval time.Duration
	stopCh   chan struct{}
}

func NewCronScheduler(cronRepo *postgres.CronRepository, pollInterval time.Duration) *CronScheduler {
	return &CronScheduler{
		cronRepo: cronRepo,
		interval: pollInterval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins polling for due cron jobs. Blocks until ctx is cancelled.
func (s *CronScheduler) Start(ctx context.Context) error {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.stopCh:
			return nil
		case <-ticker.C:
			if err := s.processDueJobs(ctx); err != nil {
				log.Printf("cron scheduler error: %v", err)
			}
		}
	}
}

func (s *CronScheduler) Stop() {
	close(s.stopCh)
}

func (s *CronScheduler) processDueJobs(ctx context.Context) error {
	now := time.Now().UTC()
	jobs, err := s.cronRepo.GetDueJobs(ctx, now)
	if err != nil {
		return fmt.Errorf("failed to get due jobs: %w", err)
	}

	for _, job := range jobs {
		nextRun, err := ComputeNextRun(job.Schedule, job.Timezone, now)
		if err != nil {
			log.Printf("cron %s: failed to compute next run: %v", job.CronID, err)
			continue
		}

		if err := s.cronRepo.UpdateNextRun(ctx, job.CronID, nextRun); err != nil {
			log.Printf("cron %s: failed to update next run: %v", job.CronID, err)
			continue
		}

		log.Printf("cron %s: triggered (next run: %s)", job.CronID, nextRun.Format(time.RFC3339))
	}

	return nil
}

// ComputeNextRun calculates the next run time for a cron schedule in the given timezone.
func ComputeNextRun(schedule, timezone string, from time.Time) (time.Time, error) {
	if timezone == "" {
		timezone = "UTC"
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid timezone %q: %w", timezone, err)
	}

	parser := cronlib.NewParser(cronlib.Minute | cronlib.Hour | cronlib.Dom | cronlib.Month | cronlib.Dow)
	sched, err := parser.Parse(schedule)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid cron schedule %q: %w", schedule, err)
	}

	localFrom := from.In(loc)
	nextLocal := sched.Next(localFrom)
	return nextLocal.UTC(), nil
}

// ValidateSchedule checks if a cron expression is valid.
func ValidateSchedule(schedule string) error {
	parser := cronlib.NewParser(cronlib.Minute | cronlib.Hour | cronlib.Dom | cronlib.Month | cronlib.Dow)
	_, err := parser.Parse(schedule)
	if err != nil {
		return fmt.Errorf("invalid cron schedule: %w", err)
	}
	return nil
}
