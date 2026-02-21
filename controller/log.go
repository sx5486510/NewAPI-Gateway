package controller

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func parseLogListQuery(c *gin.Context) (int, int, model.UsageLogQuery) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", strconv.Itoa(common.ItemsPerPage)))
	if pageSize <= 0 {
		pageSize = common.ItemsPerPage
	}

	query := model.UsageLogQuery{
		Offset:       p * pageSize,
		Limit:        pageSize,
		Keyword:      strings.TrimSpace(c.Query("keyword")),
		ProviderName: strings.TrimSpace(c.Query("provider")),
		Status:       strings.TrimSpace(c.DefaultQuery("status", "all")),
		ViewTab:      strings.TrimSpace(c.DefaultQuery("view", "all")),
	}
	return p, pageSize, query
}

func GetSelfLogs(c *gin.Context) {
	userId := c.GetInt("id")
	p, pageSize, query := parseLogListQuery(c)
	query.UserID = &userId

	logs, total, err := model.QueryUsageLogs(query)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	providers, err := model.QueryUsageLogProviders(query)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	summary, err := model.QueryUsageLogSummary(query)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":     logs,
			"total":     total,
			"page":      p,
			"page_size": pageSize,
			"providers": providers,
			"summary":   summary,
		},
	})
}

func GetAllLogs(c *gin.Context) {
	p, pageSize, query := parseLogListQuery(c)
	logs, total, err := model.QueryUsageLogs(query)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	providers, err := model.QueryUsageLogProviders(query)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	summary, err := model.QueryUsageLogSummary(query)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data": gin.H{
			"items":     logs,
			"total":     total,
			"page":      p,
			"page_size": pageSize,
			"providers": providers,
			"summary":   summary,
		},
	})
}

func GetDashboard(c *gin.Context) {
	stats, err := model.GetDashboardStats()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": stats})
}
