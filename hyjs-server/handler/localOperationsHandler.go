package handler

import (
	"context"
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
)

var testPaymentsEnabled bool

// ConfigureTestPayments 仅在开放监听前按显式启动选项设置测试确认能力。
func ConfigureTestPayments(enabled bool) { testPaymentsEnabled = enabled }

func init() {
	routes.RegisterPostRouter("/local-test/payments/confirm", wrapper.Wrapper(TestPaymentConfirmHandler))
	routes.RegisterPostRouter("/local/operations", wrapper.Wrapper(LocalOperationsHandler))
}

// TestPaymentConfirmHandler 由本机可信适配器确认隔离测试订单。
// @Summary 确认本地测试订单
// @Description 仅loopback连接且显式启用测试确认；服务限制demo角色，没有真实收费。
// @Tags LocalTest
// @Accept json
// @Produce json
// @Param body body model.TestPaymentConfirmRequest true "测试确认"
// @Success 200 {object} model.TestPaymentConfirmResponse
// @Router /local-test/payments/confirm [post]
func TestPaymentConfirmHandler(c *gin.Context) error {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	ip := net.ParseIP(host)
	// 使用TCP实际对端，忽略转发头，授权不来自游戏请求或客户端字段。
	if err != nil || ip == nil || !ip.IsLoopback() || !testPaymentsEnabled || gameService == nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, constUtil.TestPaymentAdapterDeniedMessage)
	}
	var req model.TestPaymentConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.OrderID != strings.TrimSpace(req.OrderID) || req.EventID != strings.TrimSpace(req.EventID) {
		return model.NewErrParamsError()
	}
	ctx := context.WithValue(c.Request.Context(), constUtil.P3TestPaymentContextKey, true)
	order, err := gameService.ConfirmTestPayment(ctx, req.OrderID, req.EventID, req.AmountCents, req.Outcome)
	if err != nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterRejected, err.Error())
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(model.TestPaymentConfirmResponse{OrderID: order.ID, Status: order.Status, TestOnly: true}))
	return nil
}

// LocalOperationsHandler 返回单角色或活动实例的有限只读运行摘要。
// @Summary 查询本机业务运行摘要
// @Description 仅TCP实际loopback对端，输出限定计数与状态，不含账号凭据或邮件正文。
// @Tags LocalTest
// @Accept json
// @Produce json
// @Param body body model.LocalOperationsRequest true "查询范围"
// @Success 200 {object} model.P1OperationsSnapshot
// @Router /local/operations [post]
func LocalOperationsHandler(c *gin.Context) error {
	host, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	ip := net.ParseIP(host)
	if err != nil || ip == nil || !ip.IsLoopback() || gameService == nil {
		return model.NewErrBizCodeMsg(constUtil.TestPaymentAdapterDenied, constUtil.TestPaymentAdapterDeniedMessage)
	}
	var req model.LocalOperationsRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.InstanceID != strings.TrimSpace(req.InstanceID) {
		return model.NewErrParamsError()
	}
	result, err := gameService.P1Operations(c.Request.Context(), req.PlayerID, req.InstanceID)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}
