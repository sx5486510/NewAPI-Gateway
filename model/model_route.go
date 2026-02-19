package model

import (
	"errors"
	"math/rand"
	"sort"
)

type ModelRoute struct {
	Id              int    `json:"id"`
	ModelName       string `json:"model_name" gorm:"type:varchar(255);index;not null"`
	ProviderTokenId int    `json:"provider_token_id" gorm:"index;not null"`
	ProviderId      int    `json:"provider_id" gorm:"index;not null"`
	Enabled         bool   `json:"enabled" gorm:"default:true"`
	Priority        int    `json:"priority" gorm:"default:0;index"`
	Weight          int    `json:"weight" gorm:"default:10"`
}

// SelectProviderToken selects a provider token for the given model using priority + weight algorithm
// retry is used to fall back to lower priority levels
func SelectProviderToken(modelName string, retry int) (*ProviderToken, *Provider, error) {
	var routes []ModelRoute
	err := DB.Where("model_name = ? AND enabled = ?", modelName, true).
		Order("priority DESC").Find(&routes).Error
	if err != nil {
		return nil, nil, err
	}
	if len(routes) == 0 {
		return nil, nil, errors.New("无可用的模型路由: " + modelName)
	}

	// Get distinct priorities (descending)
	prioritySet := make(map[int]bool)
	var priorities []int
	for _, r := range routes {
		if !prioritySet[r.Priority] {
			prioritySet[r.Priority] = true
			priorities = append(priorities, r.Priority)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(priorities)))

	// Select priority level based on retry count
	idx := retry
	if idx >= len(priorities) {
		idx = len(priorities) - 1
	}
	targetPriority := priorities[idx]

	// Filter candidates by priority
	var candidates []ModelRoute
	for _, r := range routes {
		if r.Priority == targetPriority {
			candidates = append(candidates, r)
		}
	}

	// Weighted random selection (same algorithm as upstream ability.go)
	weightSum := 0
	for _, r := range candidates {
		weightSum += r.Weight + 10
	}
	pick := rand.Intn(weightSum)
	for _, r := range candidates {
		pick -= r.Weight + 10
		if pick <= 0 {
			token, err := GetProviderTokenById(r.ProviderTokenId)
			if err != nil {
				return nil, nil, err
			}
			provider, err := GetProviderById(r.ProviderId)
			if err != nil {
				return nil, nil, err
			}
			return token, provider, nil
		}
	}

	return nil, nil, errors.New("路由选择失败")
}

// GetAllModelRoutes returns all routes with optional model name filter
func GetAllModelRoutes(modelName string, startIdx int, num int) ([]*ModelRoute, error) {
	var routes []*ModelRoute
	query := DB.Order("model_name ASC, priority DESC")
	if modelName != "" {
		query = query.Where("model_name LIKE ?", "%"+modelName+"%")
	}
	err := query.Limit(num).Offset(startIdx).Find(&routes).Error
	return routes, err
}

// GetDistinctModels returns all unique model names available in routes
func GetDistinctModels() ([]string, error) {
	var models []string
	err := DB.Model(&ModelRoute{}).Where("enabled = ?", true).
		Distinct("model_name").Pluck("model_name", &models).Error
	return models, err
}

func (r *ModelRoute) Update() error {
	return DB.Model(r).Updates(r).Error
}

// RebuildRoutesForProvider rebuilds all model routes for a specific provider
func RebuildRoutesForProvider(providerId int, routes []ModelRoute) error {
	tx := DB.Begin()

	// Delete old routes
	if err := tx.Where("provider_id = ?", providerId).Delete(&ModelRoute{}).Error; err != nil {
		tx.Rollback()
		return err
	}

	// Insert new routes in batches
	batchSize := 50
	for i := 0; i < len(routes); i += batchSize {
		end := i + batchSize
		if end > len(routes) {
			end = len(routes)
		}
		if err := tx.Create(routes[i:end]).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit().Error
}

func CountModelRoutes() int64 {
	var count int64
	DB.Model(&ModelRoute{}).Count(&count)
	return count
}
