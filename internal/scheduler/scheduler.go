package scheduler

import (
	"fmt"
	"time"

	"github.com/lemefy/lemefy-bacen/internal/config"
	"github.com/lemefy/lemefy-bacen/internal/scraper"
	"github.com/lemefy/lemefy-bacen/internal/storage"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
)

// Scheduler represents the job scheduler for automatic updates
type Scheduler struct {
	cron       *cron.Cron
	config     *config.Config
	storage    *storage.Database
	logger     *logrus.Logger
	scraper    *scraper.Scraper
	isRunning  bool
	lastUpdate time.Time
}

// NewScheduler creates a new scheduler instance
func NewScheduler(cfg *config.Config, db *storage.Database, sc *scraper.Scraper) *Scheduler {
	logger := config.GetLogger()

	// Create cron scheduler
	c := cron.New(
		cron.WithLocation(time.UTC),
		cron.WithLogger(&CronLogger{logger: logger}),
	)

	return &Scheduler{
		cron:     c,
		config:   cfg,
		storage:  db,
		logger:   logger,
		scraper:  sc,
		isRunning: false,
	}
}

// CronLogger implements cron.Logger interface
type CronLogger struct {
	logger *logrus.Logger
}

func (cl *CronLogger) Info(msg string, keysAndValues ...interface{}) {
	cl.logger.WithFields(cl.kvToFields(keysAndValues...)).Info(msg)
}

func (cl *CronLogger) Error(err error, msg string, keysAndValues ...interface{}) {
	cl.logger.WithFields(cl.kvToFields(keysAndValues...)).WithError(err).Error(msg)
}

func (cl *CronLogger) kvToFields(keysAndValues ...interface{}) logrus.Fields {
	fields := make(logrus.Fields)
	for i := 0; i < len(keysAndValues); i += 2 {
		if i+1 < len(keysAndValues) {
			key := keysAndValues[i]
			value := keysAndValues[i+1]
			if keyStr, ok := key.(string); ok {
				fields[keyStr] = value
			}
		}
	}
	return fields
}

// Start starts the scheduler
func (s *Scheduler) Start() error {
	if s.isRunning {
		return fmt.Errorf("scheduler is already running")
	}

	s.logger.Info("Starting scheduler...")

	// Add jobs
	s.addJobs()

	// Start cron
	s.cron.Start()
	s.isRunning = true

	s.logger.Info("Scheduler started successfully")
	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() error {
	if !s.isRunning {
		return fmt.Errorf("scheduler is not running")
	}

	s.logger.Info("Stopping scheduler...")
	
	// Stop cron
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.isRunning = false

	s.logger.Info("Scheduler stopped")
	return nil
}

// addJobs adds the scheduled jobs
func (s *Scheduler) addJobs() {
	// Daily update job
	if s.config.Scheduler.Enabled && s.config.Scheduler.UpdateCron != "" {
		_, err := s.cron.AddFunc(s.config.Scheduler.UpdateCron, func() {
			s.runDailyUpdate()
		})
		if err != nil {
			s.logger.WithError(err).Error("Failed to add daily update job")
		}
	}

	// Weekly cleanup job
	if s.config.Scheduler.Enabled && s.config.Scheduler.CleanupCron != "" {
		_, err := s.cron.AddFunc(s.config.Scheduler.CleanupCron, func() {
			s.runCleanup()
		})
		if err != nil {
			s.logger.WithError(err).Error("Failed to add cleanup job")
		}
	}
}

// runDailyUpdate runs the daily update job
func (s *Scheduler) runDailyUpdate() {
	s.lastUpdate = time.Now().UTC()
	s.logger.Info("Running daily update...")

	start := time.Now()

	// Run scraper
	err := s.scraper.Run()
	if err != nil {
		s.logger.WithError(err).Error("Daily update failed")
		s.saveUpdateStatus("failed", err.Error())
		return
	}

	duration := time.Since(start)
	s.logger.WithField("duration", duration).Info("Daily update completed")

	s.saveUpdateStatus("completed", "")
}

// runCleanup runs the cleanup job
func (s *Scheduler) runCleanup() {
	s.logger.Info("Running cleanup...")

	// Delete old revoked norms
	deleted, err := s.storage.DeleteOldNormas(s.config.Scheduler.CleanupDays)
	if err != nil {
		s.logger.WithError(err).Error("Cleanup failed")
		return
	}

	s.logger.WithField("deleted_count", deleted).Info("Cleanup completed")
}

// saveUpdateStatus saves the update status to database
func (s *Scheduler) saveUpdateStatus(status, errorMsg string) {
	stats := s.scraper.GetStats()
	duration := int(s.lastUpdate.Sub(stats.StartTime).Milliseconds())

	err := s.storage.SaveScrapeHistory(
		stats.NormasFound,
		stats.NormasAdded,
		stats.NormasUpdated,
		duration,
		status,
		errorMsg,
	)

	if err != nil {
		s.logger.WithError(err).Error("Failed to save update status")
	}
}

// RunNow triggers an immediate update
func (s *Scheduler) RunNow() error {
	s.logger.Info("Running immediate update...")

	// Run scraper directly
	err := s.scraper.Run()
	if err != nil {
		return fmt.Errorf("immediate update failed: %w", err)
	}

	return nil
}

// GetLastUpdate returns the time of the last update
func (s *Scheduler) GetLastUpdate() time.Time {
	return s.lastUpdate
}

// GetNextUpdate returns the time of the next scheduled update
func (s *Scheduler) GetNextUpdate() (time.Time, error) {
	if !s.config.Scheduler.Enabled {
		return time.Time{}, fmt.Errorf("scheduler is disabled")
	}

	// Parse cron expression
	cronParser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.DowOptional | cron.Descriptor)
	schedule, err := cronParser.Parse(s.config.Scheduler.UpdateCron)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse cron expression: %w", err)
	}

	// Get next execution time
	next := schedule.Next(s.lastUpdate)
	return next, nil
}

// IsRunning returns whether the scheduler is running
func (s *Scheduler) IsRunning() bool {
	return s.isRunning
}

// GetScheduleInfo returns information about the current schedule
type ScheduleInfo struct {
	Enabled        bool      `json:"enabled"`
	UpdateCron     string    `json:"update_cron"`
	CleanupCron    string    `json:"cleanup_cron"`
	CleanupDays    int       `json:"cleanup_days"`
	LastUpdate     time.Time `json:"last_update"`
	NextUpdate     time.Time `json:"next_update,omitempty"`
	IsRunning      bool      `json:"is_running"`
	UpdateInterval string    `json:"update_interval"`
}

// GetScheduleInfo returns information about the current schedule
func (s *Scheduler) GetScheduleInfo() (*ScheduleInfo, error) {
	info := &ScheduleInfo{
		Enabled:     s.config.Scheduler.Enabled,
		UpdateCron:  s.config.Scheduler.UpdateCron,
		CleanupCron: s.config.Scheduler.CleanupCron,
		CleanupDays: s.config.Scheduler.CleanupDays,
		LastUpdate:  s.lastUpdate,
		IsRunning:   s.isRunning,
	}

	// Get next update time
	next, err := s.GetNextUpdate()
	if err == nil {
		info.NextUpdate = next
		info.UpdateInterval = next.Sub(s.lastUpdate).String()
	} else {
		info.UpdateInterval = s.config.GetUpdateInterval().String()
	}

	return info, nil
}

// UpdateCronExpression updates the cron expression for daily updates
func (s *Scheduler) UpdateCronExpression(expression string) error {
	// Validate cron expression
	cronParser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.DowOptional | cron.Descriptor)
	_, err := cronParser.Parse(expression)
	if err != nil {
		return fmt.Errorf("invalid cron expression: %w", err)
	}

	// Update config
	s.config.Scheduler.UpdateCron = expression

	// Remove existing job and add new one
	s.cron.Stop()
	s.cron = cron.New(
		cron.WithLocation(time.UTC),
		cron.WithLogger(&CronLogger{logger: s.logger}),
	)

	s.addJobs()
	s.cron.Start()

	s.logger.WithField("cron_expression", expression).Info("Updated cron expression")
	return nil
}

// Enable enables the scheduler
func (s *Scheduler) Enable() error {
	if s.config.Scheduler.Enabled {
		return nil
	}

	s.config.Scheduler.Enabled = true

	// Add jobs and start
	s.addJobs()
	s.cron.Start()
	s.isRunning = true

	s.logger.Info("Scheduler enabled")
	return nil
}

// Disable disables the scheduler
func (s *Scheduler) Disable() error {
	if !s.config.Scheduler.Enabled {
		return nil
	}

	s.config.Scheduler.Enabled = false

	// Stop cron
	ctx := s.cron.Stop()
	<-ctx.Done()
	s.isRunning = false

	s.logger.Info("Scheduler disabled")
	return nil
}
