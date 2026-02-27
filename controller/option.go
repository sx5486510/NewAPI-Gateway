package controller

import (
	"NewAPI-Gateway/common"
	"NewAPI-Gateway/model"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func GetOptions(c *gin.Context) {
	var options []*model.Option
	common.OptionMapRWMutex.Lock()
	for k, v := range common.OptionMap {
		if strings.Contains(k, "Token") || strings.Contains(k, "Secret") {
			continue
		}
		options = append(options, &model.Option{
			Key:   k,
			Value: common.Interface2String(v),
		})
	}
	common.OptionMapRWMutex.Unlock()
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    options,
	})
	return
}

func UpdateOption(c *gin.Context) {
	var option model.Option
	err := json.NewDecoder(c.Request.Body).Decode(&option)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}
	switch option.Key {
	case "GitHubOAuthEnabled":
		if option.Value == "true" && common.GitHubClientId == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 GitHub OAuth，请先填入 GitHub Client ID 以及 GitHub Client Secret！",
			})
			return
		}
	case "WeChatAuthEnabled":
		if option.Value == "true" && common.WeChatServerAddress == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用微信登录，请先填入微信登录相关配置信息！",
			})
			return
		}
	case "TurnstileCheckEnabled":
		if option.Value == "true" && common.TurnstileSiteKey == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "无法启用 Turnstile 校验，请先填入 Turnstile 校验相关配置信息！",
			})
			return
		}
	case "HTTPProxy", "HTTPSProxy":
		trimmed := strings.TrimSpace(option.Value)
		if trimmed == "" {
			break
		}
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "invalid proxy url, expected http/https",
			})
			return
		}
		scheme := strings.ToLower(parsed.Scheme)
		if scheme != "http" && scheme != "https" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "proxy scheme must be http or https",
			})
			return
		}
	case "RoutingUsageWindowHours":
		value, err := strconv.Atoi(strings.TrimSpace(option.Value))
		if err != nil || value < 1 || value > 24*30 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "路由统计窗口必须是 1 到 720 小时的整数",
			})
			return
		}
	case "RoutingHealthAdjustmentEnabled":
		normalized := strings.TrimSpace(strings.ToLower(option.Value))
		if normalized != "true" && normalized != "false" {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "健康调节开关必须是 true 或 false",
			})
			return
		}
	case "RoutingBaseWeightFactor", "RoutingValueScoreFactor":
		value, err := strconv.ParseFloat(strings.TrimSpace(option.Value), 64)
		if err != nil || value < 0 || value > 10 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "路由系数必须是 0 到 10 之间的数字",
			})
			return
		}
	case "RoutingHealthWindowHours":
		value, err := strconv.Atoi(strings.TrimSpace(option.Value))
		if err != nil || value < 1 || value > 24*30 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "健康统计窗口必须是 1 到 720 小时的整数",
			})
			return
		}
	case "RoutingFailurePenaltyAlpha":
		value, err := strconv.ParseFloat(strings.TrimSpace(option.Value), 64)
		if err != nil || value < 0 || value > 20 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "故障惩罚系数必须是 0 到 20 之间的数字",
			})
			return
		}
	case "RoutingHealthRewardBeta":
		value, err := strconv.ParseFloat(strings.TrimSpace(option.Value), 64)
		if err != nil || value < 0 || value > 2 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "健康奖励系数必须是 0 到 2 之间的数字",
			})
			return
		}
	case "RoutingHealthMinMultiplier", "RoutingHealthMaxMultiplier":
		value, err := strconv.ParseFloat(strings.TrimSpace(option.Value), 64)
		if err != nil || value < 0 || value > 10 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "健康倍率阈值必须是 0 到 10 之间的数字",
			})
			return
		}
	case "RoutingHealthMinSamples":
		value, err := strconv.Atoi(strings.TrimSpace(option.Value))
		if err != nil || value < 1 || value > 1000 {
			c.JSON(http.StatusOK, gin.H{
				"success": false,
				"message": "健康最小样本数必须是 1 到 1000 的整数",
			})
			return
		}
	}
	err = model.UpdateOption(option.Key, option.Value)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
	})
	return
}

func TestProxy(c *gin.Context) {
	var option model.Option
	err := json.NewDecoder(c.Request.Body).Decode(&option)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "无效的参数",
		})
		return
	}

	proxy := strings.TrimSpace(option.Value)
	if proxy == "" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "代理地址不能为空",
		})
		return
	}

	// Validate proxy URL format
	proxyURL, err := url.Parse(proxy)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "代理地址格式错误: " + err.Error(),
		})
		return
	}

	if proxyURL.Scheme != "http" && proxyURL.Scheme != "https" && proxyURL.Scheme != "socks5" {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "代理协议必须是 http、https 或 sockss5",
		})
		return
	}

	// Test proxy by making a request to Google
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Proxy: func(*http.Request) (*url.URL, error) {
				return proxyURL, nil
			},
		},
	}

	req, err := http.NewRequest("GET", "https://www.google.com", nil)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "创建测试请求失败: " + err.Error(),
		})
		return
	}

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "代理连接失败: " + err.Error(),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "代理连接成功！",
			"data": gin.H{
				"status_code": resp.StatusCode,
			},
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": false,
		"message": "代理返回错误状态码: " + strconv.Itoa(resp.StatusCode),
	})
}
