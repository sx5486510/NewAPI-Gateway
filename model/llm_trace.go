package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

type LLMTrace struct {
	Id                int64  `json:"id" gorm:"primaryKey;autoIncrement"`
	RequestId         string `json:"request_id" gorm:"type:varchar(64);index"`
	UserId            int    `json:"user_id" gorm:"index"`
	AggregatedTokenId int    `json:"aggregated_token_id"`
	ProviderId        int    `json:"provider_id" gorm:"index"`
	ProviderName      string `json:"provider_name" gorm:"type:varchar(128);index"`
	ProviderTokenId   int    `json:"provider_token_id"`
	TokenGroupName    string `json:"token_group_name" gorm:"type:varchar(64);index"`
	ModelName         string `json:"model_name" gorm:"type:varchar(255);index"`
	Method            string `json:"method" gorm:"type:varchar(16)"`
	Path              string `json:"path" gorm:"type:varchar(512)"`
	StatusCode        int    `json:"status_code" gorm:"index"`
	RequestedStream   bool   `json:"requested_stream"`
	ResponseIsStream  bool   `json:"response_is_stream"`
	RequestBody       string `json:"request_body" gorm:"type:text"`
	ResponseBody      string `json:"response_body" gorm:"type:text"`
	ErrorMessage      string `json:"error_message" gorm:"type:text"`
	ClientIp          string `json:"client_ip" gorm:"type:varchar(64)"`
	UserAgent         string `json:"user_agent" gorm:"type:varchar(512)"`
	CreatedAt         int64  `json:"created_at" gorm:"index"`
	UpdatedAt         int64  `json:"updated_at" gorm:"index"`
	// InProgress marks a trace created when the request arrived and not yet
	// finalized. It is cleared once the response (or failure) is recorded.
	InProgress bool `json:"in_progress" gorm:"index;default:false"`
	// Security audit fields
	RiskLevel    string `json:"risk_level" gorm:"type:varchar(32);index;default:'unknown'"` // safe, low, medium, high, critical, unknown
	RiskTags     string `json:"risk_tags" gorm:"type:text"`                                 // JSON array of detected risk tags
	AutoReviewed bool   `json:"auto_reviewed" gorm:"index;default:false"`                   // Whether auto security scan completed
}

type LLMTraceQuery struct {
	Offset       int
	Limit        int
	Keyword      string
	ProviderName string
	ModelName    string
	Status       string
	RiskLevel    string // Filter by risk level: safe, low, medium, high, critical
}

func (t *LLMTrace) Insert() error {
	now := time.Now().Unix()
	if t.CreatedAt == 0 {
		t.CreatedAt = now
	}
	if t.UpdatedAt == 0 {
		t.UpdatedAt = t.CreatedAt
	}
	return DB.Create(t).Error
}

// UpdateLLMTrace patches an existing trace row with the given columns.
func UpdateLLMTrace(id int64, values map[string]interface{}) error {
	if id <= 0 || len(values) == 0 {
		return nil
	}
	values["updated_at"] = time.Now().Unix()
	return DB.Model(&LLMTrace{}).Where("id = ?", id).Updates(values).Error
}

// MarkStaleLLMTracesFailed finalizes traces that were still in progress after
// cutoff — the process most likely died before the response was recorded.
func MarkStaleLLMTracesFailed(cutoff int64, message string) (int64, error) {
	result := DB.Model(&LLMTrace{}).
		Where("in_progress = ? AND COALESCE(NULLIF(updated_at, 0), created_at) < ?", true, cutoff).
		Updates(map[string]interface{}{
			"in_progress":   false,
			"error_message": message,
			"updated_at":    time.Now().Unix(),
		})
	return result.RowsAffected, result.Error
}

func applyLLMTraceFilters(db *gorm.DB, query LLMTraceQuery) *gorm.DB {
	if providerName := strings.TrimSpace(query.ProviderName); providerName != "" {
		db = db.Where("provider_name = ?", providerName)
	}
	if modelName := strings.TrimSpace(query.ModelName); modelName != "" {
		db = db.Where("model_name = ?", modelName)
	}
	if riskLevel := strings.TrimSpace(query.RiskLevel); riskLevel != "" && riskLevel != "all" {
		db = db.Where("risk_level = ?", riskLevel)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		like := "%" + keyword + "%"
		db = db.Where(
			"(request_id LIKE ? OR model_name LIKE ? OR provider_name LIKE ? OR error_message LIKE ? OR client_ip LIKE ? OR user_agent LIKE ?)",
			like, like, like, like, like, like,
		)
	}

	isErrorCondition := "(status_code >= 400 OR (error_message IS NOT NULL AND TRIM(error_message) <> ''))"
	isSuccessCondition := "(status_code >= 200 AND status_code < 400 AND (error_message IS NULL OR TRIM(error_message) = ''))"
	switch query.Status {
	case "success":
		db = db.Where(isSuccessCondition).Where("in_progress = ?", false)
	case "error":
		db = db.Where(isErrorCondition)
	case "pending":
		db = db.Where("in_progress = ?", true)
	}
	return db
}

func QueryLLMTraces(query LLMTraceQuery) ([]*LLMTrace, int64, error) {
	if query.Limit <= 0 {
		query.Limit = 15
	}
	if query.Offset < 0 {
		query.Offset = 0
	}

	baseQuery := applyLLMTraceFilters(DB.Model(&LLMTrace{}), query)

	var total int64
	if err := baseQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var traces []*LLMTrace
	err := baseQuery.
		Select("id", "request_id", "user_id", "aggregated_token_id", "provider_id", "provider_name", "provider_token_id", "token_group_name", "model_name", "method", "path", "status_code", "requested_stream", "response_is_stream", "error_message", "client_ip", "user_agent", "created_at", "updated_at", "risk_level", "risk_tags", "auto_reviewed", "in_progress").
		Order("id desc").
		Limit(query.Limit).
		Offset(query.Offset).
		Find(&traces).Error
	return traces, total, err
}

func GetLLMTraceByID(id int64) (*LLMTrace, error) {
	if id <= 0 {
		return nil, errors.New("invalid llm trace id")
	}

	var trace LLMTrace
	if err := DB.First(&trace, id).Error; err != nil {
		return nil, err
	}
	return &trace, nil
}

func DeleteAllLLMTraces() (int64, error) {
	result := DB.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&LLMTrace{})
	return result.RowsAffected, result.Error
}
