package handler

import (
	"github.com/gin-gonic/gin"
	"hyjs-server/model"
	"hyjs-server/service"
	"net/http"
	"strings"
)

// adminSecurityObservation records outcomes outside handler transactions, with
// fixed reason codes and route templates instead of bodies, queries or cookies.
func adminSecurityObservation(c *gin.Context) {
	c.Next()
	c.Set("hyjs-security-observed", true)
	status := c.Writer.Status()
	action := "admin.request"
	if strings.HasSuffix(c.FullPath(), "/auth/login") {
		action = "admin.login"
	}
	if strings.HasSuffix(c.FullPath(), "/auth/logout") {
		action = "admin.logout"
	}
	if status < 400 && c.Request.Method == http.MethodGet {
		return
	}
	actor, _ := c.Get("hyjs-login-actor")
	username, _ := actor.(string)
	if p := adminPrincipal(c); p != nil {
		username = p.Username
	}
	outcome, reason := "success", "completed"
	switch {
	case status == 429:
		outcome, reason = "throttled", "retry_later"
	case status == 401:
		outcome, reason = "failure", "invalid_authentication"
	case status == 403:
		outcome, reason = "denied", "permission_or_csrf"
	case status >= 500:
		outcome, reason = "failure", "service_unavailable"
	case status >= 400:
		outcome, reason = "denied", "invalid_request"
	}
	service.ObserveSecurityEvent(c.Request.Context(), model.SecurityEventInput{Actor: username, SourceIP: c.ClientIP(), Action: action, Target: c.FullPath(), Outcome: outcome, ReasonCode: reason, RequestID: c.Writer.Header().Get("X-Request-ID")})
}

// AdminSecurityFallback observes Origin refusals made before the admin route
// group executes. It does not inspect the rejected request body or credentials.
func AdminSecurityFallback(c *gin.Context) {
	c.Next()
	seen, _ := c.Get("hyjs-security-observed")
	if seen == true || c.Writer.Status() != http.StatusForbidden || !strings.HasPrefix(c.Request.URL.Path, "/admin/api/") {
		return
	}
	service.ObserveSecurityEvent(c.Request.Context(), model.SecurityEventInput{SourceIP: c.ClientIP(), Action: "admin.request", Target: c.FullPath(), Outcome: "denied", ReasonCode: "origin_rejected", RequestID: c.Writer.Header().Get("X-Request-ID")})
}
