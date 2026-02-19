package router

import (
	"gin-template/controller"
	"gin-template/middleware"

	"github.com/gin-gonic/gin"
)

func SetRelayRouter(router *gin.Engine) {
	relay := router.Group("/")
	relay.Use(middleware.AggTokenAuth())
	{
		// OpenAI compatible endpoints
		relay.POST("/v1/chat/completions", controller.Relay)
		relay.POST("/v1/completions", controller.Relay)
		relay.POST("/v1/embeddings", controller.Relay)
		relay.POST("/v1/images/generations", controller.Relay)
		relay.POST("/v1/audio/speech", controller.Relay)
		relay.POST("/v1/audio/transcriptions", controller.Relay)
		relay.POST("/v1/moderations", controller.Relay)
		relay.POST("/v1/rerank", controller.Relay)
		relay.POST("/v1/video/generations", controller.Relay)

		// OpenAI Responses API
		relay.POST("/v1/responses", controller.Relay)

		// Anthropic compatible
		relay.POST("/v1/messages", controller.Relay)

		// Gemini compatible
		relay.POST("/v1beta/models/*path", controller.Relay)

		// Model listing
		relay.GET("/v1/models", controller.ListModels)
		relay.GET("/v1/models/:model", controller.GetModel)

		// Billing compatibility (fake, for client compat)
		relay.GET("/dashboard/billing/subscription", controller.BillingSubscription)
		relay.GET("/dashboard/billing/usage", controller.BillingUsage)
	}
}
