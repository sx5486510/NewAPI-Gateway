package service

import (
	"fmt"
	"strings"
	"time"

	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
)

// CheckinProvider performs a checkin for a specific provider
func CheckinProvider(provider *model.Provider) error {
	if provider == nil {
		return fmt.Errorf("provider is nil")
	}
	if provider.IsKeyOnly() {
		return fmt.Errorf("provider does not support checkin")
	}
	if !provider.CheckinEnabled {
		return fmt.Errorf("checkin disabled")
	}

	client := NewUpstreamClient(provider.BaseURL, provider.AccessToken, provider.UserID)
	result, err := client.Checkin()
	if err != nil {
		if isCheckinDisabledError(err) {
			provider.CheckinEnabled = false
			provider.UpdateCheckinEnabled(false)
			common.SysLog(fmt.Sprintf("provider %s checkin disabled due to upstream response: %v", provider.Name, err))
		}
		if isCheckinAlreadyDoneError(err) {
			provider.UpdateCheckinTime()
			common.SysLog(fmt.Sprintf("provider %s already checked in today, synced checkin time", provider.Name))
			return nil
		}
		return err
	}
	provider.UpdateCheckinTime()
	common.SysLog(fmt.Sprintf("provider %s checkin success, quota_awarded: %d", provider.Name, result.QuotaAwarded))
	return nil
}

// CheckinAllProviders performs checkin for all providers with checkin enabled
func CheckinAllProviders() {
	providers, err := model.GetCheckinEnabledProviders()
	if err != nil {
		common.SysLog(fmt.Sprintf("get checkin providers failed: %v", err))
		return
	}
	now := time.Now()
	for _, p := range providers {
		if checkedInToday(p.LastCheckinAt, now) {
			continue
		}
		if err := CheckinProvider(p); err != nil {
			common.SysLog(fmt.Sprintf("checkin failed for provider %s: %v", p.Name, err))
		}
	}
}

func checkedInToday(lastCheckinAt int64, now time.Time) bool {
	if lastCheckinAt <= 0 {
		return false
	}
	last := time.Unix(lastCheckinAt, 0).In(now.Location())
	return last.Year() == now.Year() && last.Month() == now.Month() && last.Day() == now.Day()
}

func isCheckinDisabledError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "checkin") && (strings.Contains(msg, "disabled") || strings.Contains(msg, "not enabled") || strings.Contains(msg, "not open") || strings.Contains(msg, "not allowed")) {
		return true
	}
	return strings.Contains(msg, "未开启签到") ||
		strings.Contains(msg, "未启用签到") ||
		strings.Contains(msg, "签到未开启") ||
		strings.Contains(msg, "签到未启用") ||
		strings.Contains(msg, "签到功能未开启") ||
		strings.Contains(msg, "签到功能未启用") ||
		strings.Contains(msg, "cloudflare")
}

func isCheckinAlreadyDoneError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "checkin") && (strings.Contains(msg, "already") || strings.Contains(msg, "today")) {
		return true
	}
	return strings.Contains(msg, "已签到") ||
		strings.Contains(msg, "今日已签到") ||
		strings.Contains(msg, "今天已签到") ||
		strings.Contains(msg, "已经签到")
}
