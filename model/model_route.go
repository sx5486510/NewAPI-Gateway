package model

import (
	"NewAPI-Gateway/common"
	"encoding/json"
	"errors"
	"math/rand"
	"sort"
	"strconv"
	"strings"
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

type ModelRoutePatch struct {
	Id       int   `json:"id"`
	Priority *int  `json:"priority,omitempty"`
	Weight   *int  `json:"weight,omitempty"`
	Enabled  *bool `json:"enabled,omitempty"`
}

func (p *ModelRoutePatch) ToUpdates() map[string]interface{} {
	updates := make(map[string]interface{})
	if p.Priority != nil {
		updates["priority"] = *p.Priority
	}
	if p.Weight != nil {
		updates["weight"] = *p.Weight
	}
	if p.Enabled != nil {
		updates["enabled"] = *p.Enabled
	}
	return updates
}

type ModelRouteOverviewItem struct {
	Id                    int      `json:"id"`
	DisplayModelName      string   `json:"display_model_name"`
	ModelName             string   `json:"model_name"`
	ProviderId            int      `json:"provider_id"`
	ProviderName          string   `json:"provider_name"`
	ProviderStatus        int      `json:"provider_status"`
	ProviderTokenId       int      `json:"provider_token_id"`
	TokenName             string   `json:"token_name"`
	TokenGroupName        string   `json:"token_group_name"`
	TokenStatus           int      `json:"token_status"`
	Enabled               bool     `json:"enabled"`
	Priority              int      `json:"priority"`
	Weight                int      `json:"weight"`
	BillingType           string   `json:"billing_type"`
	GroupRatio            float64  `json:"group_ratio"`
	PromptPricePer1M      *float64 `json:"prompt_price_per_1m"`
	CompletionPricePer1M  *float64 `json:"completion_price_per_1m"`
	PerCallPrice          *float64 `json:"per_call_price"`
	EffectiveSharePercent *float64 `json:"effective_share_percent"`
}

type modelRouteOverviewRow struct {
	Id                int     `gorm:"column:id"`
	ModelName         string  `gorm:"column:model_name"`
	ProviderId        int     `gorm:"column:provider_id"`
	ProviderName      string  `gorm:"column:provider_name"`
	ProviderStatus    int     `gorm:"column:provider_status"`
	ProviderTokenId   int     `gorm:"column:provider_token_id"`
	TokenName         string  `gorm:"column:token_name"`
	TokenGroupName    string  `gorm:"column:token_group_name"`
	TokenStatus       int     `gorm:"column:token_status"`
	Enabled           bool    `gorm:"column:enabled"`
	Priority          int     `gorm:"column:priority"`
	Weight            int     `gorm:"column:weight"`
	PricingGroupRatio string  `gorm:"column:pricing_group_ratio"`
	QuotaType         int     `gorm:"column:quota_type"`
	ModelRatio        float64 `gorm:"column:model_ratio"`
	CompletionRatio   float64 `gorm:"column:completion_ratio"`
	ModelPrice        float64 `gorm:"column:model_price"`
}

// SelectProviderToken selects a provider token for the given model using priority + weight algorithm
// retry is used to fall back to lower priority levels
func SelectProviderToken(modelName string, retry int) (*ProviderToken, *Provider, string, error) {
	requestedModel := strings.TrimSpace(modelName)
	if requestedModel == "" {
		return nil, nil, "", errors.New("无效的模型名称")
	}

	candidateRoutes, err := getCandidateRoutesByModel(requestedModel)
	if err != nil {
		return nil, nil, "", err
	}
	if len(candidateRoutes) == 0 {
		return nil, nil, "", errors.New("无可用的模型路由: " + requestedModel)
	}

	token, provider, selectedRoute, err := pickProviderByRoutes(candidateRoutes, retry)
	if err != nil {
		return nil, nil, "", err
	}
	return token, provider, selectedRoute.ModelName, nil
}

func getCandidateRoutesByModel(requestedModel string) ([]ModelRoute, error) {
	var allRoutes []ModelRoute
	if err := DB.Where("enabled = ?", true).
		Order("priority DESC, id ASC").Find(&allRoutes).Error; err != nil {
		return nil, err
	}
	if len(allRoutes) == 0 {
		return nil, nil
	}

	providerAliasLookupMap, err := loadProviderAliasLookups(allRoutes)
	if err != nil {
		return nil, err
	}

	requestedNormalized := common.NormalizeModelName(requestedModel)
	requestedVersionKey := common.ToVersionAgnosticKey(requestedNormalized)

	candidates := make([]ModelRoute, 0, len(allRoutes))
	for _, route := range allRoutes {
		lookup := providerAliasLookupMap[route.ProviderId]
		if routeMatchesRequestedModel(route.ModelName, requestedModel, requestedNormalized, requestedVersionKey, lookup) {
			candidates = append(candidates, route)
		}
	}
	return candidates, nil
}

func loadProviderAliasLookups(routes []ModelRoute) (map[int]providerModelAliasLookup, error) {
	providerIds := make([]int, 0)
	providerSet := make(map[int]bool)
	for _, route := range routes {
		if providerSet[route.ProviderId] {
			continue
		}
		providerSet[route.ProviderId] = true
		providerIds = append(providerIds, route.ProviderId)
	}

	lookups := make(map[int]providerModelAliasLookup)
	if len(providerIds) == 0 {
		return lookups, nil
	}

	var providers []Provider
	if err := DB.Select("id", "model_alias_mapping").Where("id IN ?", providerIds).Find(&providers).Error; err != nil {
		return nil, err
	}
	for _, p := range providers {
		mapping := ParseProviderAliasMapping(p.ModelAliasMapping)
		lookups[p.Id] = buildProviderModelAliasLookup(mapping)
	}
	return lookups, nil
}

func routeMatchesRequestedModel(routeModelName string, requestedModel string, requestedNormalized string,
	requestedVersionKey string, lookup providerModelAliasLookup) bool {
	routeName := strings.TrimSpace(routeModelName)
	if routeName == "" {
		return false
	}

	if strings.EqualFold(routeName, requestedModel) {
		return true
	}

	routeNormalized := common.NormalizeModelName(routeName)
	if requestedNormalized != "" && routeNormalized != "" && routeNormalized == requestedNormalized {
		return true
	}
	if requestedVersionKey != "" && routeNormalized != "" && common.ToVersionAgnosticKey(routeNormalized) == requestedVersionKey {
		return true
	}

	if mappedModel, ok := lookup.Resolve(requestedModel); ok {
		if strings.EqualFold(routeName, mappedModel) {
			return true
		}
		mappedNormalized := common.NormalizeModelName(mappedModel)
		if mappedNormalized != "" && routeNormalized != "" && mappedNormalized == routeNormalized {
			return true
		}
	}
	return false
}

func pickProviderByRoutes(routes []ModelRoute, retry int) (*ProviderToken, *Provider, ModelRoute, error) {
	if len(routes) == 0 {
		return nil, nil, ModelRoute{}, errors.New("路由选择失败")
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
	if len(candidates) == 0 {
		return nil, nil, ModelRoute{}, errors.New("路由选择失败")
	}

	// Weighted random selection (same algorithm as upstream ability.go)
	weightSum := 0
	for _, r := range candidates {
		weight := r.Weight + 10
		if weight < 0 {
			weight = 0
		}
		weightSum += weight
	}

	if weightSum > 0 {
		pick := rand.Intn(weightSum)
		for _, r := range candidates {
			weight := r.Weight + 10
			if weight < 0 {
				weight = 0
			}
			pick -= weight
			if pick <= 0 {
				token, err := GetProviderTokenById(r.ProviderTokenId)
				if err != nil {
					return nil, nil, ModelRoute{}, err
				}
				provider, err := GetProviderById(r.ProviderId)
				if err != nil {
					return nil, nil, ModelRoute{}, err
				}
				return token, provider, r, nil
			}
		}
	}

	// Fallback when all candidate weights are non-positive.
	chosen := candidates[rand.Intn(len(candidates))]
	token, err := GetProviderTokenById(chosen.ProviderTokenId)
	if err != nil {
		return nil, nil, ModelRoute{}, err
	}
	provider, err := GetProviderById(chosen.ProviderId)
	if err != nil {
		return nil, nil, ModelRoute{}, err
	}
	return token, provider, chosen, nil
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

func UpdateModelRouteFields(id int, updates map[string]interface{}) error {
	if id <= 0 {
		return errors.New("无效的路由 ID")
	}
	if len(updates) == 0 {
		return errors.New("没有可更新的字段")
	}
	return DB.Model(&ModelRoute{}).Where("id = ?", id).Updates(updates).Error
}

func BatchUpdateModelRoutes(patches []ModelRoutePatch) error {
	if len(patches) == 0 {
		return errors.New("更新列表为空")
	}
	tx := DB.Begin()
	for _, patch := range patches {
		if patch.Id <= 0 {
			tx.Rollback()
			return errors.New("存在无效的路由 ID")
		}
		updates := patch.ToUpdates()
		if len(updates) == 0 {
			continue
		}
		if err := tx.Model(&ModelRoute{}).Where("id = ?", patch.Id).Updates(updates).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

func GetModelRouteOverview(modelName string, providerId int, enabledOnly bool) ([]*ModelRouteOverviewItem, error) {
	query := DB.Table("model_routes AS mr").
		Select(strings.Join([]string{
			"mr.id",
			"mr.model_name",
			"mr.provider_id",
			"COALESCE(p.name, '') AS provider_name",
			"COALESCE(p.status, 0) AS provider_status",
			"mr.provider_token_id",
			"COALESCE(pt.name, '') AS token_name",
			"COALESCE(pt.group_name, '') AS token_group_name",
			"COALESCE(pt.status, 0) AS token_status",
			"mr.enabled",
			"mr.priority",
			"mr.weight",
			"COALESCE(p.pricing_group_ratio, '') AS pricing_group_ratio",
			"COALESCE(mp.quota_type, 0) AS quota_type",
			"COALESCE(mp.model_ratio, 0) AS model_ratio",
			"COALESCE(mp.completion_ratio, 0) AS completion_ratio",
			"COALESCE(mp.model_price, 0) AS model_price",
		}, ", ")).
		Joins("LEFT JOIN providers AS p ON p.id = mr.provider_id").
		Joins("LEFT JOIN provider_tokens AS pt ON pt.id = mr.provider_token_id").
		Joins("LEFT JOIN model_pricings AS mp ON mp.provider_id = mr.provider_id AND mp.model_name = mr.model_name")

	if modelName != "" {
		query = query.Where("mr.model_name LIKE ?", "%"+modelName+"%")
	}
	if providerId > 0 {
		query = query.Where("mr.provider_id = ?", providerId)
	}
	if enabledOnly {
		query = query.Where("mr.enabled = ?", true)
	}

	var rows []modelRouteOverviewRow
	err := query.Order("mr.model_name ASC, mr.priority DESC, mr.provider_id ASC, mr.provider_token_id ASC, mr.id ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	ratioCache := make(map[int]map[string]float64)
	aliasDisplayLookupCache := make(map[int]providerModelAliasReverseLookup)

	providerIds := make([]int, 0)
	providerSeen := make(map[int]bool)
	for _, row := range rows {
		if providerSeen[row.ProviderId] {
			continue
		}
		providerSeen[row.ProviderId] = true
		providerIds = append(providerIds, row.ProviderId)
	}
	if len(providerIds) > 0 {
		var providers []Provider
		if err := DB.Select("id", "model_alias_mapping").Where("id IN ?", providerIds).Find(&providers).Error; err != nil {
			return nil, err
		}
		for _, p := range providers {
			aliasDisplayLookupCache[p.Id] = buildProviderModelAliasReverseLookup(ParseProviderAliasMapping(p.ModelAliasMapping))
		}
	}

	items := make([]*ModelRouteOverviewItem, 0, len(rows))
	for _, row := range rows {
		groupRatioMap, ok := ratioCache[row.ProviderId]
		if !ok {
			groupRatioMap = parseGroupRatioMap(row.PricingGroupRatio)
			ratioCache[row.ProviderId] = groupRatioMap
		}
		groupRatio := getGroupRatio(row.TokenGroupName, groupRatioMap)
		item := &ModelRouteOverviewItem{
			Id:               row.Id,
			DisplayModelName: "",
			ModelName:        row.ModelName,
			ProviderId:       row.ProviderId,
			ProviderName:     row.ProviderName,
			ProviderStatus:   row.ProviderStatus,
			ProviderTokenId:  row.ProviderTokenId,
			TokenName:        row.TokenName,
			TokenGroupName:   row.TokenGroupName,
			TokenStatus:      row.TokenStatus,
			Enabled:          row.Enabled,
			Priority:         row.Priority,
			Weight:           row.Weight,
			GroupRatio:       groupRatio,
		}
		if reverseLookup, ok := aliasDisplayLookupCache[row.ProviderId]; ok {
			if sourceModelName, matched := reverseLookup.ResolveByTarget(row.ModelName); matched {
				item.DisplayModelName = sourceModelName
			}
		}

		isPerCallBilling := row.ModelPrice > 0 || row.QuotaType == 1
		if isPerCallBilling {
			item.BillingType = "per_call"
			perCallPrice := row.ModelPrice * groupRatio
			item.PerCallPrice = &perCallPrice
		} else {
			item.BillingType = "per_token"
			promptPrice := row.ModelRatio * 2 * groupRatio
			completionRatio := row.CompletionRatio
			if completionRatio <= 0 {
				completionRatio = 1
			}
			completionPrice := promptPrice * completionRatio
			item.PromptPricePer1M = &promptPrice
			item.CompletionPricePer1M = &completionPrice
		}

		items = append(items, item)
	}

	shareSum := make(map[string]float64)
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		contribution := float64(item.Weight + 10)
		if contribution < 0 {
			contribution = 0
		}
		key := item.ModelName + "#" + strconv.Itoa(item.Priority)
		shareSum[key] += contribution
	}

	for _, item := range items {
		if !item.Enabled {
			item.EffectiveSharePercent = nil
			continue
		}
		contribution := float64(item.Weight + 10)
		if contribution < 0 {
			contribution = 0
		}
		key := item.ModelName + "#" + strconv.Itoa(item.Priority)
		total := shareSum[key]
		if total <= 0 || contribution <= 0 {
			item.EffectiveSharePercent = nil
			continue
		}
		percent := contribution / total * 100
		item.EffectiveSharePercent = &percent
	}

	return items, nil
}

func parseGroupRatioMap(raw string) map[string]float64 {
	result := make(map[string]float64)
	if strings.TrimSpace(raw) == "" {
		return result
	}
	_ = json.Unmarshal([]byte(raw), &result)
	return result
}

func getGroupRatio(groupName string, groupRatioMap map[string]float64) float64 {
	if strings.TrimSpace(groupName) == "" {
		return 1
	}
	ratio, ok := groupRatioMap[groupName]
	if !ok || ratio <= 0 {
		return 1
	}
	return ratio
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
