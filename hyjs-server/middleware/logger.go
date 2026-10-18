package middleware

import (
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"hyjs-server/elog"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery
		if raw != "" {
			path = path + "?" + raw
		}
		clientIP := c.ClientIP()
		method := c.Request.Method

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		elog.Debug("request",
			zap.String("method", method),
			zap.String("path", path),
			zap.Int("status", status),
			zap.Duration("latency", latency),
			zap.String("clientIP", clientIP),
		)
	}
}

func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				elog.Error("panic recovered",
					zap.Any("err", err),
					zap.String("stack", string(debug.Stack())),
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
					zap.String("clientIP", c.ClientIP()),
				)
				c.JSON(http.StatusOK, gin.H{"code": 500, "message": "system error"})
				c.Abort()
			}
		}()
		c.Next()
	}
}

// CORS accepts browser origins from the request's own authority or an exact deployment allowlist.
// Native game requests without Origin retain their existing request and OPTIONS behavior.
func CORS() gin.HandlerFunc {
	allowedOrigins := make(map[string]bool)
	for _, configured := range strings.Split(os.Getenv("HYJS_CORS_ORIGINS"), ",") {
		origin := strings.TrimSpace(configured)
		if _, valid := corsParseOrigin(origin); valid {
			allowedOrigins[origin] = true
		}
	}
	return func(c *gin.Context) {
		// Keep caches from sharing an allowed origin's credentialed response with another origin.
		c.Writer.Header().Add("Vary", "Origin")
		origin := c.GetHeader("Origin")
		if origin != "" {
			parsed, valid := corsParseOrigin(origin)
			if !valid || (!allowedOrigins[origin] && !corsSameOrigin(parsed, c.Request)) {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.Writer.Header().Set("Access-Control-Allow-Origin", origin)
			c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
			c.Writer.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Authorization, X-CSRF-Token, X-Request-ID")
			c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, HEAD, PUT, DELETE")
		}

		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// corsParseOrigin rejects wildcards, opaque origins and URL components outside an origin.
func corsParseOrigin(origin string) (*url.URL, bool) {
	if origin == "" || strings.ContainsAny(origin, " \t\r\n?#") {
		return nil, false
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Hostname() == "" || parsed.User != nil || parsed.Path != "" || parsed.Opaque != "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, false
	}
	return parsed, true
}

// corsSameOrigin derives the transport scheme locally; forwarded headers never grant trust.
func corsSameOrigin(origin *url.URL, request *http.Request) bool {
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	if origin.Scheme != scheme {
		return false
	}
	requestOrigin := &url.URL{Scheme: scheme, Host: request.Host}
	port := func(value *url.URL) string {
		if explicit := value.Port(); explicit != "" {
			return explicit
		}
		if value.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	return strings.EqualFold(origin.Hostname(), requestOrigin.Hostname()) && port(origin) == port(requestOrigin)
}
