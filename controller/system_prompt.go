package controller

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

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
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
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
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
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
		c.JSON(http.StatusOK, gin.H{"success": false, "message": systemPromptErrorMessage(err)})
		return
	}
	updated, err := model.GetSystemPromptByID(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": systemPromptErrorMessage(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": updated})
}

func DeleteSystemPrompt(c *gin.Context) {
	id, ok := parsePositiveSystemPromptID(c)
	if !ok {
		return
	}
	unbound, err := model.DeleteSystemPrompt(id, strings.EqualFold(c.Query("unbind"), "true"))
	if errors.Is(err, model.ErrSystemPromptInUse) {
		routeCount := systemPromptRouteCount(id)
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
			"data":    gin.H{"route_count": routeCount},
		})
		return
	}
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": systemPromptErrorMessage(err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": gin.H{"unbound": unbound}})
}

func parsePositiveSystemPromptID(c *gin.Context) (int, bool) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid system prompt ID"})
		return 0, false
	}
	return id, true
}

func systemPromptErrorMessage(err error) string {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "system prompt not found"
	}
	return err.Error()
}

func systemPromptRouteCount(id int) int64 {
	prompt, err := model.GetSystemPromptByID(id)
	if err != nil {
		return 0
	}
	prompts, err := model.ListSystemPrompts(prompt.ModelName, "")
	if err != nil {
		return 0
	}
	for _, item := range prompts {
		if item.Id == id {
			return item.RouteCount
		}
	}
	return 0
}
