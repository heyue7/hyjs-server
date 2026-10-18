package handler

import (
	"bytes"
	"encoding/json"
	"hyjs-server/constUtil"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestP14FinalSealImportRejectsEnvelopeBeforeService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := gameService
	gameService = nil
	defer func() { gameService = previous }()
	valid := map[string]any{"protocolVersion": 1, "deliveryId": strings.Repeat("a", 64), "sourceServerId": 1, "destinationServerId": 2, "snapshot": map[string]any{"version": 1}}
	body, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	run := func(raw []byte, signature string) error {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", constUtil.P14FinalGameImportPath, bytes.NewReader(raw))
		c.Request.Header.Set("X-HYJS-P14-Signature", signature)
		return P14FinalSealImportHandler(c)
	}
	// An otherwise valid envelope reaches only the explicit unconfigured-service error.
	systemErr := run(body, strings.Repeat("b", 64))
	if systemErr == nil {
		t.Fatal("unconfigured service accepted import")
	}
	invalid := [][]byte{nil, []byte("null"), []byte("{}"), []byte("[1]"), append(append([]byte{}, body...), []byte(" {}")...), []byte(strings.Repeat("x", 1048577))}
	for key, value := range map[string]any{"protocolVersion": 2, "sourceServerId": 0, "destinationServerId": 1, "deliveryId": "bad", "snapshot": nil, "unknown": true} {
		changed := make(map[string]any)
		for k, v := range valid {
			changed[k] = v
		}
		changed[key] = value
		raw, e := json.Marshal(changed)
		if e != nil {
			t.Fatal(e)
		}
		invalid = append(invalid, raw)
	}
	for _, raw := range invalid {
		if e := run(raw, strings.Repeat("b", 64)); e == nil || e.Error() == systemErr.Error() {
			t.Fatal("invalid envelope reached service")
		}
	}
	// Both realm IDs stay within the documented 32-realm federation boundary.
	for _, key := range []string{"sourceServerId", "destinationServerId"} {
		changed := make(map[string]any)
		for name, value := range valid {
			changed[name] = value
		}
		changed[key] = 33
		raw, marshalErr := json.Marshal(changed)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if e := run(raw, strings.Repeat("b", 64)); e == nil || e.Error() == systemErr.Error() {
			t.Fatal("out-of-range source seal realm reached service")
		}
	}

	for _, signature := range []string{"", strings.Repeat("b", 63), strings.Repeat("g", 64)} {
		if e := run(body, signature); e == nil || e.Error() == systemErr.Error() {
			t.Fatal("invalid signature reached service")
		}
	}
}
