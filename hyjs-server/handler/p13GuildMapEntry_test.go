package handler

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
)

func TestP13GuildMapEntryHandlerRejectsMalformedIdentityAndOversizedJSON(t *testing.T) {
	previous := gameService
	gameService = &service.GameService{}
	t.Cleanup(func() { gameService = previous })
	gin.SetMode(gin.TestMode)
	for _, row := range []struct{ name, body string }{
		{"missing", `{}`},
		{"zero id", `{"playerId":0,"sessionHex":"` + strings.Repeat("a", 32) + `"}`},
		{"negative id", `{"playerId":-1,"sessionHex":"` + strings.Repeat("a", 32) + `"}`},
		{"noninteger id", `{"playerId":1.5,"sessionHex":"` + strings.Repeat("a", 32) + `"}`},
		{"overflow id", `{"playerId":9223372036854775808,"sessionHex":"` + strings.Repeat("a", 32) + `"}`},
		{"short hex", `{"playerId":1,"sessionHex":"` + strings.Repeat("a", 30) + `"}`},
		{"long hex", `{"playerId":1,"sessionHex":"` + strings.Repeat("a", 34) + `"}`},
		{"nonhex", `{"playerId":1,"sessionHex":"` + strings.Repeat("z", 32) + `"}`},
		{"invalid json", `{"playerId":1,"sessionHex":`},
		{"unknown field", `{"playerId":1,"sessionHex":"` + strings.Repeat("a", 32) + `","matchId":"client-cannot-select-field"}`},
		{"second JSON value", `{"playerId":1,"sessionHex":"` + strings.Repeat("a", 32) + `"} {}`},
		{"trailing non JSON", `{"playerId":1,"sessionHex":"` + strings.Repeat("a", 32) + `"} trailing`},
		{"oversized trailing whitespace", `{"playerId":1,"sessionHex":"` + strings.Repeat("a", 32) + `"}` + strings.Repeat(" ", 2048)},
		// The long field is inside the first JSON object, forcing the decoder
		// to consume past the actual MaxBytesReader bound before authorization.
		{"oversized object", `{"playerId":1,"sessionHex":"` + strings.Repeat("a", 32) + `","padding":"` + strings.Repeat("x", 2048) + `"}`},
	} {
		t.Run(row.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(response)
			ctx.Request = httptest.NewRequest("POST", "/guild-war/map-entry", strings.NewReader(row.body))
			ctx.Request.Header.Set("Content-Type", "application/json")
			var code *model.BizCode
			if err := P13GuildMapEntryHandler(ctx); !errors.As(err, &code) || code.Code != constUtil.ErrParamsError {
				t.Fatal("malformed map request reached account/field lookup", err)
			}
			if response.Body.Len() != 0 {
				t.Fatal("invalid map identity produced a success response")
			}
		})
	}
}

func TestP13GuildMapEntryHandlerUnavailableServiceIsSystemError(t *testing.T) {
	previous := gameService
	gameService = nil
	t.Cleanup(func() { gameService = previous })
	gin.SetMode(gin.TestMode)
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest("POST", "/guild-war/map-entry", strings.NewReader(`{"playerId":1,"sessionHex":"`+strings.Repeat("a", 32)+`"}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	var code *model.BizCode
	if err := P13GuildMapEntryHandler(ctx); !errors.As(err, &code) || code.Code != constUtil.ErrSystemError {
		t.Fatal("missing service did not report system failure", err)
	}
	if response.Body.Len() != 0 {
		t.Fatal("unavailable service returned a map")
	}
}

func TestP13GuildMapEntryHandlerRequiresJSONContentType(t *testing.T) {
	response := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(response)
	ctx.Request = httptest.NewRequest("POST", "/guild-war/map-entry", strings.NewReader(`{"playerId":1,"sessionHex":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`))
	ctx.Request.Header.Set("Content-Type", "text/plain")
	var code *model.BizCode
	if err := P13GuildMapEntryHandler(ctx); !errors.As(err, &code) || code.Code != constUtil.ErrParamsError {
		t.Fatal("wrong content type accepted", err)
	}
}
