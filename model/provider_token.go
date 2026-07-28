package model

import (
	"errors"
	"strings"
	"time"
)

type ProviderToken struct {
	Id              int    `json:"id"`
	ProviderId      int    `json:"provider_id" gorm:"index;not null"`
	UpstreamTokenId int    `json:"upstream_token_id"`
	SkKey           string `json:"sk_key" gorm:"type:varchar(256)"`
	RefreshToken    string `json:"refresh_token" gorm:"type:varchar(512)"`
	ExpiresAt       int64  `json:"expires_at" gorm:"index"`
	Name            string `json:"name"`
	GroupName       string `json:"group_name" gorm:"type:varchar(64);index"`
	Status          int    `json:"status" gorm:"default:1"`
	Priority        int    `json:"priority" gorm:"default:0"`
	Weight          int    `json:"weight" gorm:"default:10"`
	RemainQuota     int64  `json:"remain_quota"`
	UnlimitedQuota  bool   `json:"unlimited_quota"`
	UsedQuota       int64  `json:"used_quota"`
	ModelLimits     string `json:"model_limits" gorm:"type:varchar(2048)"`
	AllowCodex      bool   `json:"allow_codex" gorm:"default:false"`
	AllowCC         bool   `json:"allow_cc" gorm:"default:false"`
	BlockClients    bool   `json:"block_clients" gorm:"default:false"`
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
	pt.NormalizeClientRestrictions()
	return DB.Create(pt).Error
}

func (pt *ProviderToken) Update() error {
	pt.NormalizeClientRestrictions()
	return DB.Model(pt).Updates(map[string]interface{}{
		"provider_id":       pt.ProviderId,
		"upstream_token_id": pt.UpstreamTokenId,
		"sk_key":            pt.SkKey,
		"refresh_token":     pt.RefreshToken,
		"expires_at":        pt.ExpiresAt,
		"name":              pt.Name,
		"group_name":        pt.GroupName,
		"status":            pt.Status,
		"priority":          pt.Priority,
		"weight":            pt.Weight,
		"remain_quota":      pt.RemainQuota,
		"unlimited_quota":   pt.UnlimitedQuota,
		"used_quota":        pt.UsedQuota,
		"model_limits":      pt.ModelLimits,
		"allow_codex":       pt.AllowCodex,
		"allow_cc":          pt.AllowCC,
		"block_clients":     pt.BlockClients,
		"last_synced":       pt.LastSynced,
	}).Error
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
		pt.Id = existing.Id
		pt.CreatedAt = existing.CreatedAt
		pt.AllowCodex = existing.AllowCodex
		pt.AllowCC = existing.AllowCC
		pt.BlockClients = existing.BlockClients
		pt.NormalizeClientRestrictions()
		return DB.Model(&existing).Updates(map[string]interface{}{
			"sk_key":            pt.SkKey,
			"refresh_token":     pt.RefreshToken,
			"expires_at":        pt.ExpiresAt,
			"name":              pt.Name,
			"group_name":        pt.GroupName,
			"status":            pt.Status,
			"priority":          pt.Priority,
			"weight":            pt.Weight,
			"remain_quota":      pt.RemainQuota,
			"unlimited_quota":   pt.UnlimitedQuota,
			"used_quota":        pt.UsedQuota,
			"model_limits":      pt.ModelLimits,
			"allow_codex":       pt.AllowCodex,
			"allow_cc":          pt.AllowCC,
			"block_clients":     pt.BlockClients,
			"last_synced":       pt.LastSynced,
			"upstream_token_id": pt.UpstreamTokenId,
		}).Error
	}
	pt.CreatedAt = time.Now().Unix()
	pt.NormalizeClientRestrictions()
	return DB.Create(pt).Error
}

// DeleteProviderTokensNotInIds deletes tokens for a provider that are NOT in the given upstream token ID list
func DeleteProviderTokensNotInIds(providerId int, upstreamIds []int) error {
	if len(upstreamIds) == 0 {
		return DB.Where("provider_id = ?", providerId).Delete(&ProviderToken{}).Error
	}
	return DB.Where("provider_id = ? AND upstream_token_id NOT IN (?)", providerId, upstreamIds).Delete(&ProviderToken{}).Error
}

// IsMaskedKey returns true when the upstream token key is still masked.
func IsMaskedKey(key string) bool {
	return strings.Contains(key, "**")
}

// GetProviderTokenByUpstream retrieves an existing token by provider_id + upstream_token_id.
func GetProviderTokenByUpstream(providerId int, upstreamTokenId int) (*ProviderToken, error) {
	var token ProviderToken
	result := DB.Where("provider_id = ? AND upstream_token_id = ?", providerId, upstreamTokenId).First(&token)
	if result.RowsAffected == 0 {
		return nil, nil
	}
	return &token, result.Error
}

// CleanForResponse removes sensitive sk_key before sending to frontend
func (pt *ProviderToken) CleanForResponse() {
	if len(pt.SkKey) > 8 {
		pt.SkKey = pt.SkKey[:4] + "****" + pt.SkKey[len(pt.SkKey)-4:]
	}
}

func (pt *ProviderToken) NormalizeClientRestrictions() {
	if pt.BlockClients {
		pt.AllowCodex = false
		pt.AllowCC = false
	}
	if pt.AllowCodex || pt.AllowCC {
		pt.BlockClients = false
	}
}

// IsClientAllowed checks if a client type is allowed to use this token
// Returns true if:
// - AllowCodex, AllowCC, and BlockClients are false (no restriction)
// - clientType is empty (unrestricted client)
// - clientType matches one of the allowed types
func (pt *ProviderToken) IsClientAllowed(clientType string) bool {
	pt.NormalizeClientRestrictions()
	// No restriction set - allow all
	if !pt.AllowCodex && !pt.AllowCC && !pt.BlockClients {
		return true
	}
	// Unrestricted client - allow all
	if clientType == "" {
		return true
	}
	if pt.BlockClients && (clientType == "codex" || clientType == "cc") {
		return false
	}
	// Check specific restrictions
	if clientType == "codex" && pt.AllowCodex {
		return true
	}
	if clientType == "cc" && pt.AllowCC {
		return true
	}
	return false
}

// GetExpiringSoonTokens retrieves tokens that will expire within the given duration
func GetExpiringSoonTokens(withinSeconds int64) ([]*ProviderToken, error) {
	if withinSeconds <= 0 {
		withinSeconds = 3600 // default 1 hour
	}
	threshold := time.Now().Unix() + withinSeconds
	var tokens []*ProviderToken
	err := DB.Where("status = 1 AND expires_at > 0 AND expires_at <= ? AND refresh_token != ''", threshold).Find(&tokens).Error
	return tokens, err
}
