package service

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"time"
)

var syncTicker *time.Ticker
var checkinTimer *time.Timer
var refreshTicker *time.Ticker
var reapTicker *time.Ticker
var stopCron chan bool

const (
	dailyCheckinHour   = 0
	dailyCheckinMinute = 5
)

// pendingReapInterval / pendingReapAfter control how abandoned in-flight
// records (process restart, hard disconnect) get finalized.
const (
	pendingReapInterval = 10 * time.Minute
	pendingReapAfter    = 15 * time.Minute
)

// StartCronJobs starts background sync and checkin tasks
func StartCronJobs() {
	stopCron = make(chan bool)

	// Sync every 5 minutes
	syncTicker = time.NewTicker(5 * time.Minute)
	// Checkin at fixed local calendar time every day
	checkinTimer = time.NewTimer(durationUntilNextCheckin(time.Now()))
	// Refresh expiring tokens every 2 minutes
	refreshTicker = time.NewTicker(2 * time.Minute)
	// Finalize abandoned in-flight log/audit rows
	reapTicker = time.NewTicker(pendingReapInterval)

	// Catch up one run on startup
	go CheckinAllProviders()
	go RefreshExpiringTokens()
	// A restart means no request can still be in flight.
	go ReapAllPendingRecords()

	go func() {
		for {
			select {
			case <-syncTicker.C:
				syncAllProviders()
			case <-checkinTimer.C:
				CheckinAllProviders()
				checkinTimer.Reset(durationUntilNextCheckin(time.Now()))
			case <-refreshTicker.C:
				RefreshExpiringTokens()
			case <-reapTicker.C:
				reapStalePendingRecords(time.Now().Add(-pendingReapAfter).Unix())
			case <-stopCron:
				syncTicker.Stop()
				refreshTicker.Stop()
				reapTicker.Stop()
				if !checkinTimer.Stop() {
					select {
					case <-checkinTimer.C:
					default:
					}
				}
				return
			}
		}
	}()

	common.SysLog("cron jobs started: sync every 5m, checkin daily at 00:05 local time, refresh tokens every 2m, reap pending records every 10m")
}

// StopCronJobs stops background tasks
func StopCronJobs() {
	if stopCron != nil {
		stopCron <- true
	}
}

func syncAllProviders() {
	providers, err := model.GetEnabledProviders()
	if err != nil {
		common.SysLog("failed to get enabled providers for sync: " + err.Error())
		return
	}
	for _, p := range providers {
		if err := SyncProvider(p); err != nil {
			common.SysLog("sync failed for provider " + p.Name + ": " + err.Error())
		}
	}
}

func durationUntilNextCheckin(now time.Time) time.Duration {
	next := time.Date(
		now.Year(),
		now.Month(),
		now.Day(),
		dailyCheckinHour,
		dailyCheckinMinute,
		0,
		0,
		now.Location(),
	)
	if !now.Before(next) {
		next = next.AddDate(0, 0, 1)
	}
	return next.Sub(now)
}
