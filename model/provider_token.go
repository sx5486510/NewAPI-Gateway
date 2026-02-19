package model

import (
	"errors"
	"time"
)

type ProviderToken struct {
	Id              int    `json:"id"`
	ProviderId      int    `json:"provider_id" gorm:"index;not null"`
	UpstreamTokenId int    `json:"upstream_token_id"`
	SkKey           string `json:"sk_key" gorm:"type:varchar(256)"`
	Name            string `json:"name"`
	GroupName       string `json:"group_name" gorm:"type:varchar(64);index"`
	Status          int    `json:"status" gorm:"default:1"`
	Priority        int    `json:"priority" gorm:"default:0"`
	Weight          int    `json:"weight" gorm:"default:10"`
	RemainQuota     int64  `json:"remain_quota"`
	UnlimitedQuota  bool   `json:"unlimited_quota"`
	UsedQuota       int64  `json:"used_quota"`
	ModelLimits     string `json:"model_limits" gorm:"type:varchar(2048)"`
	LastSynced      int64  `json:"last_synced"`
	CreatedAt       int64  `json:"created_at"`
}

func GetProviderTokensByProviderId(providerId int) ([]*ProviderToken, error) {
	var tokens []*ProviderToken
	err := DB.Where("provider_id = ?", providerId).Order("id desc").Find(&tokens).Error
	return tokens, err
}

func GetEnabledProviderTokensByProviderId(providerId int) ([]*ProviderToken, error) {
	var tokens []*ProviderToken
	err := DB.Where("provider_id = ? AND status = 1", providerId).Find(&tokens).Error
	return tokens, err
}

func GetProviderTokenById(id int) (*ProviderToken, error) {
	if id == 0 {
		return nil, errors.New("id 为空")
	}
	var token ProviderToken
	err := DB.First(&token, "id = ?", id).Error
	return &token, err
}

func (pt *ProviderToken) Insert() error {
	pt.CreatedAt = time.Now().Unix()
	return DB.Create(pt).Error
}

func (pt *ProviderToken) Update() error {
	return DB.Model(pt).Updates(pt).Error
}

func (pt *ProviderToken) Delete() error {
	// Clean up related model_routes
	DB.Where("provider_token_id = ?", pt.Id).Delete(&ModelRoute{})
	return DB.Delete(pt).Error
}

// UpsertByUpstreamId creates or updates a provider token based on upstream token id + provider id
func UpsertProviderToken(pt *ProviderToken) error {
	var existing ProviderToken
	result := DB.Where("provider_id = ? AND upstream_token_id = ?", pt.ProviderId, pt.UpstreamTokenId).First(&existing)
	if result.RowsAffected > 0 {
		// Update existing
		pt.Id = existing.Id
		pt.CreatedAt = existing.CreatedAt
		return DB.Model(&existing).Updates(pt).Error
	}
	// Create new
	pt.CreatedAt = time.Now().Unix()
	return DB.Create(pt).Error
}

// DeleteProviderTokensNotInIds deletes tokens for a provider that are NOT in the given upstream token ID list
func DeleteProviderTokensNotInIds(providerId int, upstreamIds []int) error {
	if len(upstreamIds) == 0 {
		return DB.Where("provider_id = ?", providerId).Delete(&ProviderToken{}).Error
	}
	return DB.Where("provider_id = ? AND upstream_token_id NOT IN (?)", providerId, upstreamIds).Delete(&ProviderToken{}).Error
}

// CleanForResponse removes sensitive sk_key before sending to frontend
func (pt *ProviderToken) CleanForResponse() {
	if len(pt.SkKey) > 8 {
		pt.SkKey = pt.SkKey[:4] + "****" + pt.SkKey[len(pt.SkKey)-4:]
	}
}
