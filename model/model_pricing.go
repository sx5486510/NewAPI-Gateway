package model

import "time"

type ModelPricing struct {
	Id              int     `json:"id"`
	ModelName       string  `json:"model_name" gorm:"type:varchar(255);index"`
	ProviderId      int     `json:"provider_id" gorm:"index"`
	QuotaType       int     `json:"quota_type"`
	ModelRatio      float64 `json:"model_ratio"`
	CompletionRatio float64 `json:"completion_ratio"`
	ModelPrice      float64 `json:"model_price"`
	EnableGroups    string  `json:"enable_groups" gorm:"type:text"`
	LastSynced      int64   `json:"last_synced"`
}

// UpsertModelPricing creates or updates a pricing record
func UpsertModelPricing(p *ModelPricing) error {
	var existing ModelPricing
	result := DB.Where("model_name = ? AND provider_id = ?", p.ModelName, p.ProviderId).First(&existing)
	if result.RowsAffected > 0 {
		p.Id = existing.Id
		return DB.Model(&existing).Updates(p).Error
	}
	p.LastSynced = time.Now().Unix()
	return DB.Create(p).Error
}

func GetModelPricingByProvider(providerId int) ([]*ModelPricing, error) {
	var pricing []*ModelPricing
	err := DB.Where("provider_id = ?", providerId).Find(&pricing).Error
	return pricing, err
}

func GetAllModelPricing() ([]*ModelPricing, error) {
	var pricing []*ModelPricing
	err := DB.Find(&pricing).Error
	return pricing, err
}

// DeletePricingForProvider removes all pricing records for a provider
func DeletePricingForProvider(providerId int) error {
	return DB.Where("provider_id = ?", providerId).Delete(&ModelPricing{}).Error
}
