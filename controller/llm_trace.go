package controller

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func parseLLMTraceQuery(c *gin.Context) (int, int, model.LLMTraceQuery) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(common.ItemsPerPage)))
	if pageSize <= 0 {
		pageSize = common.ItemsPerPage
	}
	query := model.LLMTraceQuery{
		Offset:       p * pageSize,
		Limit:        pageSize,
		Keyword:      strings.TrimSpace(c.Query("keyword")),
		ProviderName: strings.TrimSpace(c.Query("provider")),
		ModelName:    strings.TrimSpace(c.Query("model")),
		Status:       strings.TrimSpace(c.DefaultQuery("status", "all")),
		RiskLevel:    strings.TrimSpace(c.DefaultQuery("risk_level", "all")),
	}
	return p, pageSize, query
}

func GetLLMTraces(c *gin.Context) {
	p, pageSize, query := parseLLMTraceQuery(c)
	traces, total, err := model.QueryLLMTraces(query)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":     traces,
			"total":     total,
			"page":      p,
			"page_size": pageSize,
		},
	})
}

func GetLLMTrace(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "invalid trace id"})
		return
	}
	trace, err := model.GetLLMTraceByID(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": trace})
}

func DeleteLLMTraces(c *gin.Context) {
	deleted, err := model.DeleteAllLLMTraces()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"deleted": deleted,
		},
	})
}
