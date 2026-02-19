package middleware

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowHeaders = []string{
		"Origin", "Content-Type", "Accept", "Authorization",
		"x-api-key", "x-goog-api-key", "anthropic-version",
		"Accept-Encoding", "Accept-Language",
	}
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowCredentials = false
	return cors.New(config)
}
