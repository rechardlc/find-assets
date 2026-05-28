package server

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/find-assets/scanner/internal/service"
)

// NewRouter 构造一个内置基础中间件的 Gin 引擎。
func NewRouter(svc *service.ScanService) (*gin.Engine, *ScanHandler) {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), gin.LoggerWithWriter(gin.DefaultWriter, "/api/v1/health"))
	r.Use(corsMiddleware())

	store := NewTaskStore()
	h := NewScanHandler(svc, store)

	r.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"name":    "find-assets scanner",
			"version": "v1.0.0",
			"docs":    "/api/v1",
		})
	})

	api := r.Group("/api/v1")
	{
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		api.GET("/strategies", h.ListStrategies)
		api.POST("/scans", h.SyncScan)
		api.POST("/scans/async", h.AsyncScan)
		api.GET("/scans/:id", h.GetResult)
		api.GET("/scans/:id/stream", h.Stream)
		api.GET("/scans/:id/export", h.Export)
	}
	return r, h
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type,Accept,Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
