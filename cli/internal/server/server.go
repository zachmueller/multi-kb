package server

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/zmueller/multi-kb/internal/config"
	"github.com/zmueller/multi-kb/internal/lock"
)

// RunServer starts the server-mode main loop. It blocks until SIGTERM/SIGINT.
func RunServer(ctx context.Context, cfg *config.Config) error {
	tickInterval, err := time.ParseDuration(cfg.TickInterval)
	if err != nil {
		return fmt.Errorf("server: invalid tick_interval: %w", err)
	}
	dreamInterval, err := time.ParseDuration(cfg.DreamCycle.Interval)
	if err != nil {
		return fmt.Errorf("server: invalid dream_cycle.interval: %w", err)
	}

	ctx, cancel := signal.NotifyContext(ctx, syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	lk, err := lock.Acquire(lockPath(), "server")
	if err != nil {
		return fmt.Errorf("server: cannot acquire lock: %w", err)
	}
	defer lk.Release()

	slog.Info("server started", "tick_interval", tickInterval, "dream_cycle_interval", dreamInterval)

	var busy atomic.Bool
	var lastDreamCycle time.Time
	var lastRecallLog time.Time

	recallSchedule := "02:00"
	if cfg.RecallLog != nil && cfg.RecallLog.Schedule != "" {
		recallSchedule = cfg.RecallLog.Schedule
	}

	ticker := time.NewTicker(tickInterval)
	defer ticker.Stop()

	// Run first tick immediately
	dispatchTick(ctx, cfg, &busy, &lastDreamCycle, &lastRecallLog, dreamInterval, recallSchedule)

	for {
		select {
		case <-ctx.Done():
			slog.Info("server shutting down")
			return nil
		case <-ticker.C:
			dispatchTick(ctx, cfg, &busy, &lastDreamCycle, &lastRecallLog, dreamInterval, recallSchedule)
		}
	}
}

func dispatchTick(ctx context.Context, cfg *config.Config, busy *atomic.Bool,
	lastDreamCycle, lastRecallLog *time.Time, dreamInterval time.Duration, recallSchedule string) {
	if !busy.CompareAndSwap(false, true) {
		slog.Debug("tick skipped: previous tick still running")
		return
	}
	defer busy.Store(false)

	now := time.Now().UTC()

	if now.Sub(*lastDreamCycle) >= dreamInterval {
		slog.Info("dream cycle due", "last", lastDreamCycle, "interval", dreamInterval)
		if err := RunDreamCycle(ctx, cfg); err != nil {
			slog.Error("dream cycle failed", "error", err)
		} else {
			*lastDreamCycle = now
		}
		return
	}

	// SQS ingestion
	if err := RunIngestion(ctx, cfg); err != nil {
		slog.Error("ingestion failed", "error", err)
	}

	// Recall log processing — once per day
	if shouldProcessRecallLogs(now, *lastRecallLog, recallSchedule) {
		slog.Info("recall log processing due")
		if err := RunRecallLogProcessing(ctx, cfg); err != nil {
			slog.Error("recall log processing failed", "error", err)
		} else {
			*lastRecallLog = now
		}
	}
}

func shouldProcessRecallLogs(now, lastRun time.Time, schedule string) bool {
	if now.Sub(lastRun) < 20*time.Hour {
		return false
	}
	hour, min := parseSchedule(schedule)
	nowMinutes := now.Hour()*60 + now.Minute()
	schedMinutes := hour*60 + min
	return nowMinutes >= schedMinutes
}

func parseSchedule(s string) (hour, min int) {
	fmt.Sscanf(s, "%d:%d", &hour, &min)
	return
}

func lockPath() string {
	if dir := os.Getenv("MULTI_KB_DIR"); dir != "" {
		return dir + "/lock"
	}
	return lock.DefaultLockPath()
}
