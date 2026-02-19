package service

import (
	"fmt"
	"gin-template/common"
	"gin-template/model"
)

// CheckinProvider performs a checkin for a specific provider
func CheckinProvider(provider *model.Provider) error {
	client := NewUpstreamClient(provider.BaseURL, provider.AccessToken, provider.UserID)
	result, err := client.Checkin()
	if err != nil {
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
	for _, p := range providers {
		if err := CheckinProvider(p); err != nil {
			common.SysLog(fmt.Sprintf("checkin failed for provider %s: %v", p.Name, err))
		}
	}
}
