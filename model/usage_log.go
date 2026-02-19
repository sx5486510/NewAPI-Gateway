package model

import "time"

type UsageLog struct {
	Id                int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId            int    `json:"user_id" gorm:"index"`
	AggregatedTokenId int    `json:"aggregated_token_id"`
	ProviderId        int    `json:"provider_id" gorm:"index"`
	ProviderName      string `json:"provider_name" gorm:"type:varchar(128)"`
	ProviderTokenId   int    `json:"provider_token_id"`
	ModelName         string `json:"model_name" gorm:"type:varchar(255);index"`
	PromptTokens      int    `json:"prompt_tokens"`
	CompletionTokens  int    `json:"completion_tokens"`
	ResponseTimeMs    int    `json:"response_time_ms"`
	Status            int    `json:"status" gorm:"default:1"`
	ErrorMessage      string `json:"error_message" gorm:"type:text"`
	ClientIp          string `json:"client_ip" gorm:"type:varchar(64)"`
	RequestId         string `json:"request_id" gorm:"type:varchar(64);index"`
	CreatedAt         int64  `json:"created_at" gorm:"index"`
}

func (l *UsageLog) Insert() error {
	l.CreatedAt = time.Now().Unix()
	return DB.Create(l).Error
}

// GetUserLogs returns logs for a specific user
func GetUserLogs(userId int, startIdx int, num int) ([]*UsageLog, error) {
	var logs []*UsageLog
	err := DB.Where("user_id = ?", userId).Order("id desc").
		Limit(num).Offset(startIdx).Find(&logs).Error
	return logs, err
}

// GetAllLogs returns all logs (admin)
func GetAllLogs(startIdx int, num int) ([]*UsageLog, error) {
	var logs []*UsageLog
	err := DB.Order("id desc").Limit(num).Offset(startIdx).Find(&logs).Error
	return logs, err
}

// CountUserLogs counts total logs for a user
func CountUserLogs(userId int) int64 {
	var count int64
	DB.Model(&UsageLog{}).Where("user_id = ?", userId).Count(&count)
	return count
}

// CountAllLogs counts total logs
func CountAllLogs() int64 {
	var count int64
	DB.Model(&UsageLog{}).Count(&count)
	return count
}

// DashboardStats holds aggregated statistics
type DashboardStats struct {
	TotalRequests   int64               `json:"total_requests"`
	SuccessRequests int64               `json:"success_requests"`
	FailedRequests  int64               `json:"failed_requests"`
	TotalProviders  int64               `json:"total_providers"`
	TotalModels     int64               `json:"total_models"`
	TotalRoutes     int64               `json:"total_routes"`
	ByProvider      []ProviderStat      `json:"by_provider"`
	ByModel         []ModelStat         `json:"by_model"`
	RecentRequests  []DailyRequestCount `json:"recent_requests"`
}

type ProviderStat struct {
	ProviderId   int    `json:"provider_id"`
	ProviderName string `json:"provider_name"`
	RequestCount int64  `json:"request_count"`
}

type ModelStat struct {
	ModelName    string `json:"model_name"`
	RequestCount int64  `json:"request_count"`
}

type DailyRequestCount struct {
	Date         string `json:"date"`
	RequestCount int64  `json:"request_count"`
}

// GetDashboardStats returns aggregated stats for the admin dashboard
func GetDashboardStats() (*DashboardStats, error) {
	stats := &DashboardStats{}

	// Total counts
	DB.Model(&UsageLog{}).Count(&stats.TotalRequests)
	DB.Model(&UsageLog{}).Where("status = 1").Count(&stats.SuccessRequests)
	stats.FailedRequests = stats.TotalRequests - stats.SuccessRequests
	stats.TotalProviders = CountProviders()
	stats.TotalRoutes = CountModelRoutes()

	models, _ := GetDistinctModels()
	stats.TotalModels = int64(len(models))

	// By provider
	DB.Model(&UsageLog{}).Select("provider_id, provider_name, count(*) as request_count").
		Group("provider_id, provider_name").Order("request_count desc").
		Limit(10).Scan(&stats.ByProvider)

	// By model
	DB.Model(&UsageLog{}).Select("model_name, count(*) as request_count").
		Group("model_name").Order("request_count desc").
		Limit(10).Scan(&stats.ByModel)

	return stats, nil
}
