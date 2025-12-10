package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nativedb/internal/core"

	"github.com/gin-contrib/cors"
	"github.com/gin-contrib/gzip"
	"github.com/gin-gonic/gin"
)

/**
 * @brief 启动服务器
 * @param config 应用配置
 * @return error 启动错误
 */
func Start(config *core.AppConfig) error {
	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(gzip.Gzip(gzip.DefaultCompression))

	// CORS 配置
	r.Use(cors.New(cors.Config{
		AllowOrigins:     strings.Split(config.AllowOrigins, ","),
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Cache"},
		ExposeHeaders:    []string{"Content-Length", "X-Cache"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	// 注册路由
	registerRoutes(r)

	// 前端静态文件服务
	if config.FrontendPath != "" {
		if _, err := os.Stat(config.FrontendPath); !os.IsNotExist(err) {
			fmt.Printf("Serving frontend from: %s\n", config.FrontendPath)
			r.NoRoute(func(c *gin.Context) {
				path := c.Request.URL.Path
				if strings.HasPrefix(path, "/api") {
					c.JSON(http.StatusNotFound, gin.H{"error": "Not Found"})
					return
				}
				file := filepath.Join(config.FrontendPath, path)
				if info, err := os.Stat(file); err == nil && !info.IsDir() {
					c.File(file)
					return
				}
				c.File(filepath.Join(config.FrontendPath, "index.html"))
			})
		}
	}

	fmt.Printf("Server starting on %s\n", config.BindPort)
	return r.Run(config.BindPort)
}

/**
 * @brief 注册路由
 * @param r Gin 路由实例
 */
func registerRoutes(r *gin.Engine) {
	api := r.Group("/api")
	{
		// 公开接口
		api.GET("/natives", GetNativesList)
		api.GET("/native/:hash", GetNativeDetail)
		api.GET("/native/:hash/source", GetNativeSource)
		api.GET("/native/:hash/example", GetNativeExamples)
		api.POST("/auth/login", LoginHandler)

		// 管理接口
		protected := api.Group("/")
		protected.Use(AuthMiddleware())
		{
			protected.GET("/auth/me", GetCurrentUser)
			protected.POST("/auth/change-password", ChangePasswordHandler)
			protected.GET("/checkAuth", func(c *gin.Context) { c.Status(200) })
			protected.POST("/native/:hash/translate", UpdateNativeTranslation)
			protected.POST("/native/:hash/params", UpdateNativeParams)
			protected.POST("/native/:hash/example", AddOrUpdateExample)
			protected.DELETE("/native/:hash/example", DeleteExample)
		}
	}
}
