package service

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"time"
)

var syncTicker *time.Ticker
var checkinTicker *time.Ticker
var stopCron chan bool

// StartCronJobs starts background sync and checkin tasks
func StartCronJobs() {
	stopCron = make(chan bool)

	// Sync every 5 minutes
	syncTicker = time.NewTicker(5 * time.Minute)
	// Checkin every 24 hours
	checkinTicker = time.NewTicker(24 * time.Hour)

	go func() {
		for {
			select {
			case <-syncTicker.C:
				syncAllProviders()
			case <-checkinTicker.C:
				CheckinAllProviders()
			case <-stopCron:
				syncTicker.Stop()
				checkinTicker.Stop()
				return
			}
		}
	}()

	common.SysLog("cron jobs started: sync every 5m, checkin every 24h")
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
