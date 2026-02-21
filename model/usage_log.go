package model

import (
	"math"
	"strings"
	"time"

	"gorm.io/gorm"
)

type UsageLog struct {
	Id                    int64   `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId                int     `json:"user_id" gorm:"index"`
	AggregatedTokenId     int     `json:"aggregated_token_id"`
	ProviderId            int     `json:"provider_id" gorm:"index"`
	ProviderName          string  `json:"provider_name" gorm:"type:varchar(128)"`
	ProviderTokenId       int     `json:"provider_token_id"`
	ModelName             string  `json:"model_name" gorm:"type:varchar(255);index"`
	PromptTokens          int     `json:"prompt_tokens"`
	CompletionTokens      int     `json:"completion_tokens"`
	CacheTokens           int     `json:"cache_tokens"`
	CacheCreationTokens   int     `json:"cache_creation_tokens"`
	CacheCreation5mTokens int     `json:"cache_creation_5m_tokens"`
	CacheCreation1hTokens int     `json:"cache_creation_1h_tokens"`
	ResponseTimeMs        int     `json:"response_time_ms"`
	FirstTokenMs          int     `json:"first_token_ms"`
	IsStream              bool    `json:"is_stream"`
	CostUSD               float64 `json:"cost_usd"`
	Status                int     `json:"status"`
	ErrorMessage          string  `json:"error_message" gorm:"type:text"`
	ClientIp              string  `json:"client_ip" gorm:"type:varchar(64)"`
	RequestId             string  `json:"request_id" gorm:"type:varchar(64);index"`
	CreatedAt             int64   `json:"created_at" gorm:"index"`
}

func (l *UsageLog) Insert() error {
	l.CreatedAt = time.Now().Unix()
	return DB.Model(&UsageLog{}).Create(map[string]interface{}{
		"user_id":                 l.UserId,
		"aggregated_token_id":     l.AggregatedTokenId,
		"provider_id":             l.ProviderId,
		"provider_name":           l.ProviderName,
		"provider_token_id":       l.ProviderTokenId,
		"model_name":              l.ModelName,
		"prompt_tokens":           l.PromptTokens,
		"completion_tokens":       l.CompletionTokens,
		"cache_tokens":            l.CacheTokens,
		"cache_creation_tokens":   l.CacheCreationTokens,
		"cache_creation5m_tokens": l.CacheCreation5mTokens,
		"cache_creation1h_tokens": l.CacheCreation1hTokens,
		"response_time_ms":        l.ResponseTimeMs,
		"first_token_ms":          l.FirstTokenMs,
		"is_stream":               l.IsStream,
		"cost_usd":                l.CostUSD,
		"status":                  l.Status,
		"error_message":           l.ErrorMessage,
		"client_ip":               l.ClientIp,
		"request_id":              l.RequestId,
		"created_at":              l.CreatedAt,
	}).Error
}

type UsageLogQuery struct {
	UserID       *int
	Offset       int
	Limit        int
	Keyword      string
	ProviderName string
	Status       string
	ViewTab      string
}

type UsageLogSummary struct {
	Total        int64   `json:"total"`
	SuccessCount int64   `json:"success_count"`
	ErrorCount   int64   `json:"error_count"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CacheTokens  int64   `json:"cache_tokens"`
	TotalCost    float64 `json:"total_cost"`
	AvgLatency   int64   `json:"avg_latency"`
}

func applyUsageLogFilters(db *gorm.DB, query UsageLogQuery) *gorm.DB {
	if query.UserID != nil {
		db = db.Where("user_id = ?", *query.UserID)
	}
	if providerName := strings.TrimSpace(query.ProviderName); providerName != "" {
		db = db.Where("provider_name = ?", providerName)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where(
			"(model_name LIKE ? OR provider_name LIKE ? OR request_id LIKE ? OR error_message LIKE ? OR client_ip LIKE ?)",
			like, like, like, like, like,
		)
	}

	isErrorCondition := "(status <> 1 OR (error_message IS NOT NULL AND TRIM(error_message) <> ''))"
	isSuccessCondition := "(status = 1 AND (error_message IS NULL OR TRIM(error_message) = ''))"
	if query.ViewTab == "error" {
		db = db.Where(isErrorCondition)
	}
	switch query.Status {
	case "success":
		db = db.Where(isSuccessCondition)
	case "error":
		db = db.Where(isErrorCondition)
	}
	return db
}

func QueryUsageLogs(query UsageLogQuery) ([]*UsageLog, int64, error) {
	if query.Limit <= 0 {
		query.Limit = 15
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	baseQuery := applyUsageLogFilters(DB.Model(&UsageLog{}), query)

	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var logs []*UsageLog
	err := baseQuery.Order("id desc").Limit(query.Limit).Offset(query.Offset).Find(&logs).Error
	return logs, total, err
}

func QueryUsageLogProviders(query UsageLogQuery) ([]string, error) {
	// Provider options should not collapse to the currently selected provider.
	// Keep other filters, but ignore provider_name itself.
	providerQuery := query
	providerQuery.ProviderName = ""

	var providers []string
	err := applyUsageLogFilters(DB.Model(&UsageLog{}), providerQuery).
		Where("provider_name IS NOT NULL AND TRIM(provider_name) <> ''").
		Distinct("provider_name").
		Order("provider_name asc").
		Pluck("provider_name", &providers).Error
	return providers, err
}

func QueryUsageLogSummary(query UsageLogQuery) (UsageLogSummary, error) {
	isErrorCondition := "(status <> 1 OR (error_message IS NOT NULL AND TRIM(error_message) <> ''))"
	isSuccessCondition := "(status = 1 AND (error_message IS NULL OR TRIM(error_message) = ''))"

	type usageLogSummaryRaw struct {
		Total        int64
		SuccessCount int64
		ErrorCount   int64
		InputTokens  int64
		OutputTokens int64
		CacheTokens  int64
		TotalCost    float64
		AvgLatency   float64
	}

	var raw usageLogSummaryRaw
	err := applyUsageLogFilters(DB.Model(&UsageLog{}), query).
		Select(
			"COUNT(*) AS total",
			"SUM(CASE WHEN "+isSuccessCondition+" THEN 1 ELSE 0 END) AS success_count",
			"SUM(CASE WHEN "+isErrorCondition+" THEN 1 ELSE 0 END) AS error_count",
			"COALESCE(SUM(prompt_tokens), 0) AS input_tokens",
			"COALESCE(SUM(completion_tokens), 0) AS output_tokens",
			"COALESCE(SUM(cache_tokens), 0) AS cache_tokens",
			"COALESCE(SUM(cost_usd), 0) AS total_cost",
			"COALESCE(AVG(response_time_ms), 0) AS avg_latency",
		).
		Scan(&raw).Error
	if err != nil {
		return UsageLogSummary{}, err
	}

	return UsageLogSummary{
		Total:        raw.Total,
		SuccessCount: raw.SuccessCount,
		ErrorCount:   raw.ErrorCount,
		InputTokens:  raw.InputTokens,
		OutputTokens: raw.OutputTokens,
		CacheTokens:  raw.CacheTokens,
		TotalCost:    raw.TotalCost,
		AvgLatency:   int64(math.Round(raw.AvgLatency)),
	}, nil
}

// GetUserLogs returns logs for a specific user
func GetUserLogs(userId int, startIdx int, num int) ([]*UsageLog, error) {
	logQuery := UsageLogQuery{
		UserID: &userId,
		Offset: startIdx,
		Limit:  num,
	}
	logs, _, err := QueryUsageLogs(logQuery)
	return logs, err
}

// GetAllLogs returns all logs (admin)
func GetAllLogs(startIdx int, num int) ([]*UsageLog, error) {
	logQuery := UsageLogQuery{
		Offset: startIdx,
		Limit:  num,
	}
	logs, _, err := QueryUsageLogs(logQuery)
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
	TotalRequests    int64               `json:"total_requests"`
	SuccessRequests  int64               `json:"success_requests"`
	FailedRequests   int64               `json:"failed_requests"`
	TotalProviders   int64               `json:"total_providers"`
	TotalModels      int64               `json:"total_models"`
	TotalRoutes      int64               `json:"total_routes"`
	ByProvider       []ProviderStat      `json:"by_provider"`
	ByModel          []ModelStat         `json:"by_model"`
	RecentRequests   []DailyRequestCount `json:"recent_requests"`
	RecentMetrics    []DailyTrendStat    `json:"recent_metrics"`
	RecentModelStats []DailyModelStat    `json:"recent_model_stats"`
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

type DailyTrendStat struct {
	Date         string  `json:"date"`
	RequestCount int64   `json:"request_count"`
	CostUSD      float64 `json:"cost_usd"`
	TokenCount   int64   `json:"token_count"`
}

type DailyModelStat struct {
	Date       string `json:"date"`
	ModelName  string `json:"model_name"`
	TokenCount int64  `json:"token_count"`
}

// GetDashboardStats returns aggregated stats for the admin dashboard
func GetDashboardStats() (*DashboardStats, error) {
	stats := &DashboardStats{}

	// Total counts
	DB.Model(&UsageLog{}).Count(&stats.TotalRequests)
	DB.Model(&UsageLog{}).Where("status = 1 AND (error_message = '' OR error_message IS NULL)").Count(&stats.SuccessRequests)
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

	// Recent trends (last 7 days, including today)
	type dailyTrendRow struct {
		Date         string  `json:"date"`
		RequestCount int64   `json:"request_count"`
		CostUSD      float64 `json:"cost_usd"`
		TokenCount   int64   `json:"token_count"`
	}
	now := time.Now()
	startDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -6)
	startUnix := startDay.Unix()

	var dateExpr string
	switch DB.Dialector.Name() {
	case "mysql":
		dateExpr = "DATE_FORMAT(FROM_UNIXTIME(created_at), '%Y-%m-%d')"
	case "postgres":
		dateExpr = "TO_CHAR(TO_TIMESTAMP(created_at), 'YYYY-MM-DD')"
	default:
		dateExpr = "strftime('%Y-%m-%d', datetime(created_at, 'unixepoch', 'localtime'))"
	}

	var recentRows []dailyTrendRow
	if err := DB.Model(&UsageLog{}).
		Select(dateExpr+" AS date, COUNT(*) AS request_count, COALESCE(SUM(cost_usd), 0) AS cost_usd, COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS token_count").
		Where("created_at >= ?", startUnix).
		Group(dateExpr).
		Order(dateExpr + " ASC").
		Scan(&recentRows).Error; err != nil {
		return nil, err
	}

	trendMap := make(map[string]DailyTrendStat, len(recentRows))
	for _, row := range recentRows {
		trendMap[row.Date] = DailyTrendStat{
			Date:         row.Date,
			RequestCount: row.RequestCount,
			CostUSD:      row.CostUSD,
			TokenCount:   row.TokenCount,
		}
	}

	stats.RecentRequests = make([]DailyRequestCount, 0, 7)
	stats.RecentMetrics = make([]DailyTrendStat, 0, 7)
	for i := 0; i < 7; i++ {
		day := startDay.AddDate(0, 0, i)
		dayStr := day.Format("2006-01-02")
		trend := trendMap[dayStr]
		trend.Date = dayStr

		stats.RecentMetrics = append(stats.RecentMetrics, trend)
		stats.RecentRequests = append(stats.RecentRequests, DailyRequestCount{
			Date:         dayStr,
			RequestCount: trend.RequestCount,
		})
	}

	type modelTokenTotalRow struct {
		ModelName  string `json:"model_name"`
		TokenCount int64  `json:"token_count"`
	}
	type modelDailyRow struct {
		Date       string `json:"date"`
		ModelName  string `json:"model_name"`
		TokenCount int64  `json:"token_count"`
	}

	var topModelRows []modelTokenTotalRow
	if err := DB.Model(&UsageLog{}).
		Select("model_name, COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS token_count").
		Where("created_at >= ? AND model_name IS NOT NULL AND TRIM(model_name) <> ''", startUnix).
		Group("model_name").
		Order("token_count DESC").
		Limit(8).
		Scan(&topModelRows).Error; err != nil {
		return nil, err
	}

	if len(topModelRows) > 0 {
		modelNames := make([]string, 0, len(topModelRows))
		for _, row := range topModelRows {
			modelNames = append(modelNames, row.ModelName)
		}

		var modelRows []modelDailyRow
		if err := DB.Model(&UsageLog{}).
			Select(dateExpr+" AS date, model_name, COALESCE(SUM(prompt_tokens + completion_tokens), 0) AS token_count").
			Where("created_at >= ? AND model_name IN ?", startUnix, modelNames).
			Group(dateExpr + ", model_name").
			Order(dateExpr + " ASC").
			Scan(&modelRows).Error; err != nil {
			return nil, err
		}
		stats.RecentModelStats = make([]DailyModelStat, 0, len(modelRows))
		for _, row := range modelRows {
			stats.RecentModelStats = append(stats.RecentModelStats, DailyModelStat{
				Date:       row.Date,
				ModelName:  row.ModelName,
				TokenCount: row.TokenCount,
			})
		}
	} else {
		stats.RecentModelStats = []DailyModelStat{}
	}

	return stats, nil
}
