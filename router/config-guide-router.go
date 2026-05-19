package router

import (
	"github.com/QuantumNous/new-api/controller"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

func SetConfigGuideRouter(router *gin.Engine) {
	configGuideRoute := router.Group("/config-guides")
	configGuideRoute.Use(middleware.RouteTag("config_guides"))
	configGuideRoute.Use(gzip.Gzip(gzip.DefaultCompression))
	configGuideRoute.Use(middleware.GlobalWebRateLimit())
	{
		ompRoute := configGuideRoute.Group("/omp-openai")
		{
			ompRoute.GET("/manifest.json", controller.GetOMPConfigGuideManifest)
			ompRoute.GET("/plugin.txt", controller.GetOMPConfigGuidePlugin)
			ompRoute.GET("/models.yml", controller.GetOMPConfigGuideModels)
			ompRoute.GET("/config.yml", controller.GetOMPConfigGuideConfig)
			ompRoute.GET("/image-generator.md", controller.GetOMPConfigGuideImageGenerator)
		}

		openCodeRoute := configGuideRoute.Group("/opencode-openai")
		{
			openCodeRoute.GET("/manifest.json", controller.GetOpenCodeConfigGuideManifest)
			openCodeRoute.GET("/opencode.json", controller.GetOpenCodeConfigGuideJSON)
		}
	}
}
