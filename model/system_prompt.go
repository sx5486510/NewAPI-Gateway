package model

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidSystemPrompt       = errors.New("invalid system prompt")
	ErrDuplicateSystemPrompt     = errors.New("duplicate system prompt")
	ErrSystemPromptInUse         = errors.New("system prompt is in use")
	ErrSystemPromptModelMismatch = errors.New("system prompt model does not match bound route model")
)

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
	if prompt.Name == "" || prompt.ModelName == "" || strings.TrimSpace(prompt.Content) == "" {
		return fmt.Errorf("%w: name, model name, and content are required", ErrInvalidSystemPrompt)
	}
	return nil
}

func CreateSystemPrompt(prompt *SystemPrompt) error {
	if prompt == nil {
		return fmt.Errorf("%w: system prompt is required", ErrInvalidSystemPrompt)
	}
	if err := normalizeSystemPrompt(prompt); err != nil {
		return err
	}
	now := time.Now().Unix()
	prompt.CreatedAt = now
	prompt.UpdatedAt = now
	duplicate, err := systemPromptNameExists(prompt.ModelName, prompt.Name, 0)
	if err != nil {
		return err
	}
	if duplicate {
		return ErrDuplicateSystemPrompt
	}
	if err := DB.Create(prompt).Error; err != nil {
		if duplicate, checkErr := systemPromptNameExists(prompt.ModelName, prompt.Name, 0); checkErr == nil && duplicate {
			return ErrDuplicateSystemPrompt
		}
		return err
	}
	return nil
}

func UpdateSystemPrompt(prompt *SystemPrompt) error {
	if prompt == nil || prompt.Id <= 0 {
		return fmt.Errorf("%w: valid system prompt is required", ErrInvalidSystemPrompt)
	}
	if err := normalizeSystemPrompt(prompt); err != nil {
		return err
	}
	prompt.UpdatedAt = time.Now().Unix()
	return DB.Transaction(func(tx *gorm.DB) error {
		existing, err := lockSystemPromptByID(tx, prompt.Id)
		if err != nil {
			return err
		}
		duplicate, err := systemPromptNameExistsWithDB(tx, prompt.ModelName, prompt.Name, prompt.Id)
		if err != nil {
			return err
		}
		if duplicate {
			return ErrDuplicateSystemPrompt
		}
		if existing.ModelName != prompt.ModelName {
			var bound int64
			if err := tx.Model(&ModelRoute{}).
				Where("system_prompt_id = ?", prompt.Id).
				Count(&bound).Error; err != nil {
				return err
			}
			if bound > 0 {
				return ErrSystemPromptModelMismatch
			}
		}
		result := tx.Model(&SystemPrompt{}).Where("id = ?", prompt.Id).Updates(map[string]interface{}{
			"name": prompt.Name, "model_name": prompt.ModelName, "content": prompt.Content, "updated_at": prompt.UpdatedAt,
		})
		if result.Error != nil {
			if duplicate, checkErr := systemPromptNameExistsWithDB(tx, prompt.ModelName, prompt.Name, prompt.Id); checkErr == nil && duplicate {
				return ErrDuplicateSystemPrompt
			}
			return result.Error
		}
		return nil
	})
}

func ListSystemPrompts(modelName, keyword string) ([]*SystemPrompt, error) {
	query := DB.Model(&SystemPrompt{})
	if modelName = strings.TrimSpace(modelName); modelName != "" {
		query = query.Where("system_prompts.model_name = ?", modelName)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("system_prompts.name LIKE ?", like)
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
	var resultCount int64
	err := DB.Transaction(func(tx *gorm.DB) error {
		if _, err := lockSystemPromptByID(tx, id); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&ModelRoute{}).Where("system_prompt_id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 && !unbind {
			resultCount = count
			return ErrSystemPromptInUse
		}
		if count > 0 {
			result := tx.Model(&ModelRoute{}).Where("system_prompt_id = ?", id).Update("system_prompt_id", nil)
			if result.Error != nil {
				return result.Error
			}
			resultCount = result.RowsAffected
		}
		result := tx.Delete(&SystemPrompt{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	return resultCount, err
}

// All prompt-binding mutations acquire this row lock before reading or changing route bindings.
func lockSystemPromptByID(db *gorm.DB, id int) (*SystemPrompt, error) {
	var prompt SystemPrompt
	if err := db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&prompt, id).Error; err != nil {
		return nil, err
	}
	return &prompt, nil
}

func systemPromptNameExists(modelName, name string, excludeID int) (bool, error) {
	return systemPromptNameExistsWithDB(DB, modelName, name, excludeID)
}

func systemPromptNameExistsWithDB(db *gorm.DB, modelName, name string, excludeID int) (bool, error) {
	query := db.Model(&SystemPrompt{}).Where("model_name = ? AND name = ?", modelName, name)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
