package model

import (
	"NewAPI-Gateway/common"
	"errors"
	"strings"
	"time"
)

type Provider struct {
	Id                       int    `json:"id"`
	Name                     string `json:"name" gorm:"index;not null"`
	BaseURL                  string `json:"base_url" gorm:"not null"`
	AccessToken              string `json:"access_token" gorm:"type:text"`
	ApiKey                   string `json:"api_key" gorm:"type:text"`
	ProviderType             string `json:"provider_type" gorm:"type:varchar(32);default:'full'"`
	UserID                   int    `json:"user_id"`
	Status                   int    `json:"status" gorm:"default:1"`
	Priority                 int    `json:"priority" gorm:"default:0"`
	Weight                   int    `json:"weight" gorm:"default:10"`
	CheckinEnabled           bool   `json:"checkin_enabled"`
	LastCheckinAt            int64  `json:"last_checkin_at"`
	LastSyncedAt             int64  `json:"last_synced_at"`
	Balance                  string `json:"balance"`
	BalanceUpdated           int64  `json:"balance_updated"`
	PricingGroupRatio        string `json:"pricing_group_ratio" gorm:"type:text"`
	PricingSupportedEndpoint string `json:"pricing_supported_endpoint" gorm:"type:text"`
	ModelAliasMapping        string `json:"model_alias_mapping" gorm:"type:text"`
	Remark                   string `json:"remark" gorm:"type:text"`
	CreatedAt                int64  `json:"created_at"`
}

const (
	ProviderTypeFull    = "full"
	ProviderTypeKeyOnly = "key_only"
)

func GetAllProviders(startIdx int, num int) ([]*Provider, error) {
	var providers []*Provider
	err := DB.Order("id desc").Limit(num).Offset(startIdx).Find(&providers).Error
	return providers, err
}

func GetProviderById(id int) (*Provider, error) {
	if id == 0 {
		return nil, errors.New("id 为空")
	}
	var provider Provider
	err := DB.First(&provider, "id = ?", id).Error
	return &provider, err
}

func GetEnabledProviders() ([]*Provider, error) {
	var providers []*Provider
	err := DB.Where("status = ?", common.UserStatusEnabled).Find(&providers).Error
	return providers, err
}

func GetCheckinEnabledProviders() ([]*Provider, error) {
	var providers []*Provider
	err := DB.Where("status = ? AND checkin_enabled = ?", common.UserStatusEnabled, true).Find(&providers).Error
	return providers, err
}

func (p *Provider) Insert() error {
	p.CreatedAt = time.Now().Unix()
	p.ProviderType = NormalizeProviderType(p.ProviderType)
	return DB.Create(p).Error
}

func (p *Provider) Update() error {
	updates := map[string]interface{}{
		"name":            p.Name,
		"base_url":        p.BaseURL,
		"user_id":         p.UserID,
		"status":          p.Status,
		"priority":        p.Priority,
		"weight":          p.Weight,
		"checkin_enabled": p.CheckinEnabled,
		"remark":          p.Remark,
		"provider_type":   NormalizeProviderType(p.ProviderType),
	}
	if strings.TrimSpace(p.AccessToken) != "" {
		updates["access_token"] = p.AccessToken
	}
	if strings.TrimSpace(p.ApiKey) != "" {
		updates["api_key"] = p.ApiKey
	}
	return DB.Model(&Provider{}).Where("id = ?", p.Id).Updates(updates).Error
}

func (p *Provider) Delete() error {
	if p.Id == 0 {
		return errors.New("id 为空")
	}
	// Also clean up related provider_tokens and model_routes
	DB.Where("provider_id = ?", p.Id).Delete(&ProviderToken{})
	DB.Where("provider_id = ?", p.Id).Delete(&ModelRoute{})
	DB.Where("provider_id = ?", p.Id).Delete(&ModelPricing{})
	return DB.Delete(p).Error
}

func (p *Provider) UpdateBalance(balance string) {
	DB.Model(p).Updates(map[string]interface{}{
		"balance":         balance,
		"balance_updated": time.Now().Unix(),
	})
}

func (p *Provider) UpdatePricingGroupRatio(groupRatio string) {
	DB.Model(p).Update("pricing_group_ratio", groupRatio)
}

func (p *Provider) UpdatePricingSupportedEndpoint(supportedEndpoint string) {
	DB.Model(p).Update("pricing_supported_endpoint", supportedEndpoint)
}

func (p *Provider) UpdateModelAliasMapping(modelAliasMapping string) {
	DB.Model(p).Update("model_alias_mapping", modelAliasMapping)
}

func (p *Provider) UpdateCheckinTime() {
	DB.Model(p).Update("last_checkin_at", time.Now().Unix())
}

func (p *Provider) UpdateLastSyncedTime() {
	DB.Model(p).Update("last_synced_at", time.Now().Unix())
}

func (p *Provider) UpdateCheckinEnabled(enabled bool) {
	DB.Model(p).Update("checkin_enabled", enabled)
}

// CleanForResponse removes sensitive fields before sending to frontend
func (p *Provider) CleanForResponse() {
	p.AccessToken = ""
	p.ApiKey = ""
}

func NormalizeProviderType(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case ProviderTypeKeyOnly:
		return ProviderTypeKeyOnly
	case ProviderTypeFull, "":
		return ProviderTypeFull
	default:
		return ProviderTypeFull
	}
}

func (p *Provider) IsKeyOnly() bool {
	return NormalizeProviderType(p.ProviderType) == ProviderTypeKeyOnly
}

func CountProviders() int64 {
	var count int64
	DB.Model(&Provider{}).Count(&count)
	return count
}
