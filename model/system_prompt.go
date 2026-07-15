package model

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrSystemPromptInUse = errors.New("system prompt is in use")

type SystemPrompt struct {
	Id         int    `json:"id"`
	Name       string `json:"name" gorm:"type:varchar(255);not null;uniqueIndex:idx_system_prompts_model_name_name"`
	ModelName  string `json:"model_name" gorm:"type:varchar(255);not null;index;uniqueIndex:idx_system_prompts_model_name_name"`
	Content    string `json:"content" gorm:"type:text;not null"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
	RouteCount int64  `json:"route_count" gorm:"-"`
}

func normalizeSystemPrompt(prompt *SystemPrompt) error {
	prompt.Name = strings.TrimSpace(prompt.Name)
	prompt.ModelName = strings.TrimSpace(prompt.ModelName)
	prompt.Content = strings.TrimSpace(prompt.Content)
	if prompt.Name == "" || prompt.ModelName == "" || prompt.Content == "" {
		return errors.New("system prompt name, model name, and content are required")
	}
	return nil
}

func CreateSystemPrompt(prompt *SystemPrompt) error {
	if prompt == nil {
		return errors.New("system prompt is required")
	}
	if err := normalizeSystemPrompt(prompt); err != nil {
		return err
	}
	now := time.Now().Unix()
	prompt.CreatedAt = now
	prompt.UpdatedAt = now
	return DB.Create(prompt).Error
}

func UpdateSystemPrompt(prompt *SystemPrompt) error {
	if prompt == nil || prompt.Id <= 0 {
		return errors.New("valid system prompt is required")
	}
	if err := normalizeSystemPrompt(prompt); err != nil {
		return err
	}
	prompt.UpdatedAt = time.Now().Unix()
	return DB.Model(&SystemPrompt{}).Where("id = ?", prompt.Id).Updates(map[string]interface{}{
		"name": prompt.Name, "model_name": prompt.ModelName, "content": prompt.Content, "updated_at": prompt.UpdatedAt,
	}).Error
}

func ListSystemPrompts(modelName, keyword string) ([]*SystemPrompt, error) {
	query := DB.Model(&SystemPrompt{})
	if modelName = strings.TrimSpace(modelName); modelName != "" {
		query = query.Where("system_prompts.model_name = ?", modelName)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("system_prompts.name LIKE ? OR system_prompts.content LIKE ?", like, like)
	}
	var prompts []*SystemPrompt
	if err := query.Order("system_prompts.id ASC").Find(&prompts).Error; err != nil || len(prompts) == 0 {
		return prompts, err
	}
	type routeCountRow struct {
		SystemPromptId int
		RouteCount     int64
	}
	ids := make([]int, 0, len(prompts))
	for _, prompt := range prompts {
		ids = append(ids, prompt.Id)
	}
	var counts []routeCountRow
	if err := DB.Model(&ModelRoute{}).
		Select("system_prompt_id, COUNT(*) AS route_count").
		Where("system_prompt_id IN ?", ids).
		Group("system_prompt_id").Scan(&counts).Error; err != nil {
		return nil, err
	}
	countByID := make(map[int]int64, len(counts))
	for _, count := range counts {
		countByID[count.SystemPromptId] = count.RouteCount
	}
	for _, prompt := range prompts {
		prompt.RouteCount = countByID[prompt.Id]
	}
	return prompts, nil
}

func GetSystemPromptByID(id int) (*SystemPrompt, error) {
	var prompt SystemPrompt
	err := DB.First(&prompt, id).Error
	return &prompt, err
}

func DeleteSystemPrompt(id int, unbind bool) (int64, error) {
	var unbound int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&ModelRoute{}).Where("system_prompt_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 && !unbind {
			return ErrSystemPromptInUse
		}
		if count > 0 {
			result := tx.Model(&ModelRoute{}).Where("system_prompt_id = ?", id).Update("system_prompt_id", nil)
			if result.Error != nil {
				return result.Error
			}
			unbound = result.RowsAffected
		}
		return tx.Delete(&SystemPrompt{}, id).Error
	})
	return unbound, err
}
