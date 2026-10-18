package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/service"
)

func TestLocalPaymentAdapterChecksActualPeerAndExplicitEnable(t *testing.T) {
	previousService, previousEnabled := gameService, testPaymentsEnabled
	defer func() { gameService, testPaymentsEnabled = previousService, previousEnabled }()
	gameService = &service.GameService{}
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name, peer, body string
		enabled          bool
		code             int64
	}{
		{"disabled", "127.0.0.1:45000", `{}`, false, constUtil.TestPaymentAdapterDenied},
		{"LAN cannot authorize via forwarded header", "172.16.28.211:45000", `{}`, true, constUtil.TestPaymentAdapterDenied},
		{"missing peer port", "127.0.0.1", `{}`, true, constUtil.TestPaymentAdapterDenied},
		{"wrong outcome", "127.0.0.1:45000", `{"orderId":"order","eventId":"event","amountCents":600,"outcome":"client_success"}`, true, constUtil.ErrParamsError},
		{"negative amount", "[::1]:45000", `{"orderId":"order","eventId":"event","amountCents":-1,"outcome":"confirmed"}`, true, constUtil.ErrParamsError},
		{"source whitespace", "127.0.0.1:45000", `{"orderId":"order","eventId":" event","amountCents":600,"outcome":"confirmed"}`, true, constUtil.ErrParamsError},
	} {
		t.Run(test.name, func(t *testing.T) {
			testPaymentsEnabled = test.enabled
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/local-test/payments/confirm", strings.NewReader(test.body))
			c.Request.RemoteAddr = test.peer
			c.Request.Header.Set("Content-Type", "application/json")
			c.Request.Header.Set("X-Forwarded-For", "127.0.0.1")
			err := TestPaymentConfirmHandler(c)
			biz, ok := err.(*model.BizCode)
			if !ok || biz.Code != test.code {
				t.Fatalf("unexpected pre-service rejection: %v", err)
			}
		})
	}
}

func TestLocalOperationsRejectsRemoteAndUnboundedRequests(t *testing.T) {
	previous := gameService
	defer func() { gameService = previous }()
	gameService = &service.GameService{}
	for _, test := range []struct{ peer, body string }{
		{"172.16.28.211:45000", `{"playerId":4}`},
		{"127.0.0.1:45000", `{"playerId":0}`},
		{"127.0.0.1:45000", `{"playerId":4,"instanceId":" leading"}`},
	} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/local/operations", strings.NewReader(test.body))
		c.Request.RemoteAddr = test.peer
		c.Request.Header.Set("Content-Type", "application/json")
		if err := LocalOperationsHandler(c); err == nil {
			t.Fatal("request reached service without a bounded loopback query")
		}
	}
}
