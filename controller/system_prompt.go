package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type systemPromptInput struct {
	Name      string `json:"name"`
	ModelName string `json:"model_name"`
	Content   string `json:"content"`
}

func GetSystemPrompts(c *gin.Context) {
	prompts, err := model.ListSystemPrompts(c.Query("model"), c.Query("keyword"))
	if err != nil {
		respondSystemPromptError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": prompts})
}

func CreateSystemPrompt(c *gin.Context) {
	var input systemPromptInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid system prompt parameters"})
		return
	}
	prompt := &model.SystemPrompt{Name: input.Name, ModelName: input.ModelName, Content: input.Content}
	if err := model.CreateSystemPrompt(prompt); err != nil {
		respondSystemPromptError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": prompt})
}

func UpdateSystemPrompt(c *gin.Context) {
	id, ok := parsePositiveSystemPromptID(c)
	if !ok {
		return
	}
	var input systemPromptInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid system prompt parameters"})
		return
	}
	prompt := &model.SystemPrompt{Id: id, Name: input.Name, ModelName: input.ModelName, Content: input.Content}
	if err := model.UpdateSystemPrompt(prompt); err != nil {
		respondSystemPromptError(c, err)
		return
	}
	updated, err := model.GetSystemPromptByID(id)
	if err != nil {
		respondSystemPromptError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": updated})
}

func DeleteSystemPrompt(c *gin.Context) {
	id, ok := parsePositiveSystemPromptID(c)
	if !ok {
		return
	}
	resultCount, err := model.DeleteSystemPrompt(id, strings.EqualFold(c.Query("unbind"), "true"))
	if errors.Is(err, model.ErrSystemPromptInUse) {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "system prompt is in use",
			"data":    gin.H{"route_count": resultCount},
		})
		return
	}
	if err != nil {
		respondSystemPromptError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"unbound": resultCount}})
}

func parsePositiveSystemPromptID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid system prompt ID"})
		return 0, false
	}
	return id, true
}

func respondSystemPromptError(c *gin.Context, err error) {
	message := "system prompt operation failed"
	switch {
	case errors.Is(err, model.ErrInvalidSystemPrompt):
		message = "system prompt name, model name, and content are required"
	case errors.Is(err, model.ErrDuplicateSystemPrompt):
		message = "system prompt name already exists for this model"
	case errors.Is(err, gorm.ErrRecordNotFound):
		message = "system prompt not found"
	case errors.Is(err, model.ErrSystemPromptInUse):
		message = "system prompt is in use"
	default:
		common.SysLog("system prompt operation failed: " + err.Error())
	}
	c.JSON(http.StatusOK, gin.H{"success": false, "message": message})
}
