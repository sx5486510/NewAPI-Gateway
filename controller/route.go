package controller

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"NewAPI-Gateway/service"
	"net/http"
	"strconv"

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
	var route model.ModelRoute
	if err := c.ShouldBindJSON(&route); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	route.Id = id
	if err := route.Update(); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func RebuildRoutes(c *gin.Context) {
	go func() {
		if err := service.RebuildAllRoutes(); err != nil {
			common.SysLog("rebuild all routes failed: " + err.Error())
		}
	}()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "路由重建任务已启动"})
}
