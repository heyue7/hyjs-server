package handler

import (
	"errors"
	"fmt"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSproutStatusInputLimits78(t *testing.T) {
	previous := gameService
	gameService = &service.GameService{}
	t.Cleanup(func() { gameService = previous })
	key := strings.Repeat("a", 32)
	var ids []string
	for i := 1; i <= 101; i++ {
		ids = append(ids, fmt.Sprint(i))
	}
	for name, body := range map[string]string{
		"missing player": `{"sessionHex":"` + key + `","playerIds":[1]}`,
		"missing peers":  `{"playerId":1,"sessionHex":"` + key + `","playerIds":[]}`,
		"short session":  `{"playerId":1,"sessionHex":"aa","playerIds":[1]}`,
		"invalid hex":    `{"playerId":1,"sessionHex":"` + strings.Repeat("z", 32) + `","playerIds":[1]}`,
		"duplicate":      `{"playerId":1,"sessionHex":"` + key + `","playerIds":[1,1]}`,
		"negative peer":  `{"playerId":1,"sessionHex":"` + key + `","playerIds":[-1]}`,
		"too many":       `{"playerId":1,"sessionHex":"` + key + `","playerIds":[` + strings.Join(ids, ",") + `]}`,
		"oversized":      `{"playerId":1,"sessionHex":"` + key + `","playerIds":[1],"padding":"` + strings.Repeat("x", 4096) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/sprout-status", strings.NewReader(body))
			var biz *model.BizCode
			if err := SproutStatusHandler(c); !errors.As(err, &biz) || biz.Code != constUtil.ErrParamsError {
				t.Fatalf("invalid request was not rejected: %v", err)
			}
		})
	}
}
