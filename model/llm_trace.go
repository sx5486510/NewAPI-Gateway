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
}

type LLMTraceQuery struct {
	Offset       int
	Limit        int
	Keyword      string
	ProviderName string
	ModelName    string
	Status       string
}

func (t *LLMTrace) Insert() error {
	if t.CreatedAt == 0 {
		t.CreatedAt = time.Now().Unix()
	}
	return DB.Create(t).Error
}

func applyLLMTraceFilters(db *gorm.DB, query LLMTraceQuery) *gorm.DB {
	if providerName := strings.TrimSpace(query.ProviderName); providerName != "" {
		db = db.Where("provider_name = ?", providerName)
	}
	if modelName := strings.TrimSpace(query.ModelName); modelName != "" {
		db = db.Where("model_name = ?", modelName)
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
		db = db.Where(isSuccessCondition)
	case "error":
		db = db.Where(isErrorCondition)
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
		Select("id", "request_id", "user_id", "aggregated_token_id", "provider_id", "provider_name", "provider_token_id", "model_name", "method", "path", "status_code", "requested_stream", "response_is_stream", "error_message", "client_ip", "user_agent", "created_at").
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
