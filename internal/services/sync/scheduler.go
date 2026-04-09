package sync

import (
	"context"
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron         *cron.Cron
	orchestrator *Orchestrator
	opts         SyncOptions
	alert        Alert
	logger       *slog.Logger
}

func NewScheduler(orchestrator *Orchestrator, opts SyncOptions, alert Alert, logger *slog.Logger) *Scheduler {
	return &Scheduler{
		cron:         cron.New(),
		orchestrator: orchestrator,
		opts:         opts,
		alert:        alert,
		logger:       logger,
	}
}

func (s *Scheduler) Start(schedule string) error {
	_, err := s.cron.AddFunc(schedule, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Hour)
		defer cancel()

		if s.logger != nil {
			s.logger.Info("scheduled sync started")
		}

		result, err := s.orchestrator.Run(ctx, s.opts)
		if err != nil {
			if s.alert != nil {
				s.alert.Send("Sync Failed", err.Error())
			}
			if s.logger != nil {
				s.logger.Error("scheduled sync failed", slog.String("error", err.Error()))
			}
			return
		}

		if s.logger != nil {
			s.logger.Info("scheduled sync completed",
				slog.Int("processed", result.Processed),
				slog.Int("failed", result.Failed),
			)
		}
	})
	if err != nil {
		return err
	}

	s.cron.Start()
	return nil
}

func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
}
