package controller

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"NewAPI-Gateway/service"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func GetModelRoutes(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	modelName := c.Query("model")
	routes, err := model.GetAllModelRoutes(modelName, p*common.ItemsPerPage, common.ItemsPerPage)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": routes})
}

func GetAllModels(c *gin.Context) {
	models, err := model.GetDistinctModels()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": models})
}

func UpdateRoute(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的 ID"})
		return
	}
	var patch model.ModelRoutePatch
	if err := c.ShouldBindJSON(&patch); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	patch.Id = id
	updates := patch.ToUpdates()
	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "没有可更新的字段"})
		return
	}
	if err := model.UpdateModelRouteFields(id, updates); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func BatchUpdateRoutes(c *gin.Context) {
	var req struct {
		Items []model.ModelRoutePatch `json:"items"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if len(req.Items) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "更新列表为空"})
		return
	}
	if err := model.BatchUpdateModelRoutes(req.Items); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func GetModelRouteOverview(c *gin.Context) {
	modelName := strings.TrimSpace(c.Query("model"))
	providerId, _ := strconv.Atoi(c.Query("provider_id"))
	enabledOnly := false
	enabledOnlyRaw := strings.TrimSpace(c.Query("enabled_only"))
	if enabledOnlyRaw == "1" || strings.EqualFold(enabledOnlyRaw, "true") {
		enabledOnly = true
	}

	overview, err := model.GetModelRouteOverview(modelName, providerId, enabledOnly)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": overview})
}

func RebuildRoutes(c *gin.Context) {
	go func() {
		if err := service.RebuildAllRoutes(); err != nil {
			common.SysLog("rebuild all routes failed: " + err.Error())
		}
	}()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "路由重建任务已启动"})
}
