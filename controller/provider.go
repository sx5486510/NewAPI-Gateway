package controller

import (
	"encoding/json"
	"gin-template/common"
	"gin-template/model"
	"gin-template/service"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

func GetProviders(c *gin.Context) {
	p, _ := strconv.Atoi(c.Query("p"))
	if p < 0 {
		p = 0
	}
	providers, err := model.GetAllProviders(p*common.ItemsPerPage, common.ItemsPerPage)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	// Clean sensitive fields
	for _, provider := range providers {
		provider.CleanForResponse()
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": providers})
}

func CreateProvider(c *gin.Context) {
	var provider model.Provider
	if err := json.NewDecoder(c.Request.Body).Decode(&provider); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if provider.Name == "" || provider.BaseURL == "" || provider.AccessToken == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "名称、地址和 AccessToken 不能为空"})
		return
	}
	if err := provider.Insert(); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func UpdateProvider(c *gin.Context) {
	var provider model.Provider
	if err := json.NewDecoder(c.Request.Body).Decode(&provider); err != nil || provider.Id == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的参数"})
		return
	}
	if err := provider.Update(); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func DeleteProvider(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的 ID"})
		return
	}
	provider := &model.Provider{Id: id}
	if err := provider.Delete(); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": ""})
}

func SyncProviderHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的 ID"})
		return
	}
	provider, err := model.GetProviderById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "供应商不存在"})
		return
	}
	go func() {
		if err := service.SyncProvider(provider); err != nil {
			common.SysLog("sync provider failed: " + err.Error())
		}
	}()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "同步任务已启动"})
}

func CheckinProviderHandler(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的 ID"})
		return
	}
	provider, err := model.GetProviderById(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "供应商不存在"})
		return
	}
	if err := service.CheckinProvider(provider); err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "签到失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "签到成功"})
}

func GetProviderTokens(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "无效的 ID"})
		return
	}
	tokens, err := model.GetProviderTokensByProviderId(id)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": err.Error()})
		return
	}
	for _, t := range tokens {
		t.CleanForResponse()
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "", "data": tokens})
}
