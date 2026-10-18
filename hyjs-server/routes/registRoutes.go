package routes

import (
	"github.com/gin-gonic/gin"
)

// POST 路由映射
var routerPostMap = make(map[string]func(c *gin.Context))

// GET 路由映射用于具名只读音频配置查询。
var routerGetMap = make(map[string]func(c *gin.Context))

func RegisterGetRouter(url string, fn func(c *gin.Context)) {
	routerGetMap[url] = fn
}

// 注册需要认证的 POST 路由
var routerPostAuthMap = make(map[string]func(c *gin.Context))

// 注册 POST 路由
func RegisterPostRouter(url string, fn func(c *gin.Context)) {
	routerPostMap[url] = fn
}

func RegisterPostAuthRouter(url string, fn func(c *gin.Context)) {
	routerPostAuthMap[url] = fn
}

// 注册所有路由到 gin.Engine
func RouteRegister(app *gin.Engine) {
	for url, handler := range routerGetMap {
		app.GET(url, handler)
	}
	for url, handler := range routerPostMap {
		app.POST(url, handler)
	}
	for url, handler := range routerPostAuthMap {
		app.POST(url, handler)
	}
}
