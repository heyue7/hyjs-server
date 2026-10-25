package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/wrapper"
)

func TestContentModerationRegistrationShowsActionableError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/game-account/register", wrapper.Wrapper(GameAccountRegisterHandler))
	for _, input := range []string{
		`{"username":"pornhub","nickname":"玩家","password":"secret-password","confirmPassword":"secret-password"}`,
		`{"username":"safe-user","nickname":"习近平","password":"secret-password","confirmPassword":"secret-password"}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/game-account/register", strings.NewReader(input))
		request.Header.Set("Content-Type", "application/json")
		request.RemoteAddr = "198.51.100.92:1234"
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		var body struct {
			Code int
			Msg  string
		}
		if response.Code != http.StatusBadRequest || json.Unmarshal(response.Body.Bytes(), &body) != nil || body.Code != 400 || body.Msg != constUtil.ErrNameSensitive.Error() {
			t.Fatalf("missing name correction prompt: %d %s", response.Code, response.Body.String())
		}
	}
}

func TestContentModerationAdminShowsActionableError(t *testing.T) {
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	adminError(ctx, constUtil.ErrNameSensitive)
	if response.Code != 400 || !strings.Contains(response.Body.String(), constUtil.ErrNameSensitive.Error()) {
		t.Fatal("admin sensitive name became internal error")
	}
}
