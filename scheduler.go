package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

// Scheduler manages scheduled sync operations
type Scheduler struct {
	schedule string
	worker   *SyncWorker
	logger   *slog.Logger
	cron     *cron.Cron
}

// newScheduler creates a new scheduler
func newScheduler(schedule string, worker *SyncWorker, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		schedule: schedule,
		worker:   worker,
		logger:   logger,
		cron:     cron.New(),
	}
}

// Run starts the scheduler and blocks until context is cancelled
func (s *Scheduler) Run(ctx context.Context) error {
	s.logger.Info("Setting up scheduler", "schedule", s.schedule)

	// Add sync job to cron
	_, err := s.cron.AddFunc(s.schedule, func() {
		s.logger.Info("Scheduled sync triggered")

		// Create context with timeout for sync operation
		syncCtx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		if err := s.worker.SyncAll(syncCtx); err != nil {
			s.logger.Error("Scheduled sync failed", "error", err)
		} else {
			s.logger.Info("Scheduled sync completed successfully")
		}
	})
	if err != nil {
		return err
	}

	// Start cron scheduler
	s.cron.Start()
	s.logger.Info("Scheduler started")

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	s.logger.Info("Stopping scheduler")
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()

	return nil
}