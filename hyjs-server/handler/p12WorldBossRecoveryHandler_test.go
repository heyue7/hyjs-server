package handler

import (
	"encoding/base64"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
)

// TestP12WorldBossRecoveryRejectsUntrustedState 阻断恢复/放弃端口夹带伤害、跨战斗参数和伪会话。
func TestP12WorldBossRecoveryRejectsUntrustedState(t *testing.T) {
	previous := gameService
	gameService = nil
	defer func() { gameService = previous }()
	gin.SetMode(gin.TestMode)
	for _, entry := range []struct {
		path    string
		handler func(*gin.Context) error
		valid   string
		invalid []string
	}{
		{"/world-boss/resume", P12WorldBossResumeHandler, `{}`, []string{
			`null`, `[]`, `{"fightId":"foreign"}`, `{"damage":1}`, `{} {}`, ``,
		}},
		{"/world-boss/abandon", P12WorldBossAbandonHandler, `{"fightId":"server-fight_1"}`, []string{
			`{}`, `null`, `{"fightId":"../x"}`, `{"fightId":" x"}`, `{"fightId":"a","damage":1}`,
			`{"fightId":"a"}{}`, `{"fightId":"` + strings.Repeat("a", 65) + `"}`,
		}},
	} {
		call := func(body, session string) error {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", entry.path, strings.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("X-Hyjs-Player-ID", "4")
			c.Request.Header.Set("X-Hyjs-Session-Key", session)
			return entry.handler(c)
		}
		session := base64.StdEncoding.EncodeToString(make([]byte, 16))
		for _, body := range entry.invalid {
			var code *model.BizCode
			if err := call(body, session); !errors.As(err, &code) || code.Code != constUtil.ErrParamsError {
				t.Fatalf("%s invalid body reached service: %q, %v", entry.path, body, err)
			}
		}
		for _, session := range []string{"", "invalid", base64.StdEncoding.EncodeToString(make([]byte, 15))} {
			var code *model.BizCode
			if err := call(entry.valid, session); !errors.As(err, &code) || code.Code != constUtil.ErrParamsError {
				t.Fatalf("%s invalid session reached service: %v", entry.path, err)
			}
		}
		// 正常形状应通过边界校验；此处故意无service，必须停在系统未初始化处。
		var code *model.BizCode
		if err := call(entry.valid, session); !errors.As(err, &code) || code.Code != constUtil.ErrSystemError {
			t.Fatalf("%s valid request was rejected as malformed: %v", entry.path, err)
		}
	}
}
