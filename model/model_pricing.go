package model

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type ModelPricing struct {
	Id                     int     `json:"id"`
	ModelName              string  `json:"model_name" gorm:"type:varchar(255);index"`
	ProviderId             int     `json:"provider_id" gorm:"index"`
	QuotaType              int     `json:"quota_type"`
	ModelRatio             float64 `json:"model_ratio"`
	CompletionRatio        float64 `json:"completion_ratio"`
	ModelPrice             float64 `json:"model_price"`
	EnableGroups           string  `json:"enable_groups" gorm:"type:text"`
	SupportedEndpointTypes string  `json:"supported_endpoint_types" gorm:"type:text"`
	LastSynced             int64   `json:"last_synced"`
}

func upsertModelPricingWithDB(db *gorm.DB, p *ModelPricing) error {
	var existing ModelPricing
	result := db.Where("model_name = ? AND provider_id = ?", p.ModelName, p.ProviderId).First(&existing)
	if result.RowsAffected > 0 {
		p.Id = existing.Id
		return db.Model(&existing).Updates(p).Error
	}
	p.LastSynced = time.Now().Unix()
	return db.Create(p).Error
}

// UpsertModelPricing creates or updates a pricing record
func UpsertModelPricing(p *ModelPricing) error {
	return upsertModelPricingWithDB(DB, p)
}

// UpsertModelPricingSilent creates or updates a pricing record without logging query warnings.
// Intended for sync flows that delete records first.
func UpsertModelPricingSilent(p *ModelPricing) error {
	silentDB := DB.Session(&gorm.Session{Logger: logger.Discard})
	return upsertModelPricingWithDB(silentDB, p)
}

func GetModelPricingByProvider(providerId int) ([]*ModelPricing, error) {
	var pricing []*ModelPricing
	err := DB.Where("provider_id = ?", providerId).Find(&pricing).Error
	return pricing, err
}

func GetModelPricingByProviderAndModel(providerId int, modelName string) (*ModelPricing, error) {
	var pricing ModelPricing
	err := DB.Where("provider_id = ? AND model_name = ?", providerId, modelName).First(&pricing).Error
	if err != nil {
		return nil, err
	}
	return &pricing, nil
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
