package handler

import (
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter("/ggo-field/snapshot", wrapper.Wrapper(GGOFieldSnapshotHandler))
	routes.RegisterPostRouter("/ggo-field/position/status", wrapper.Wrapper(GGOFieldPositionStatusHandler))
	routes.RegisterPostRouter("/ggo-field/entry", wrapper.Wrapper(GGOFieldEntryHandler))
	routes.RegisterPostRouter("/ggo-field/settlement", wrapper.Wrapper(GGOFieldSettlementHandler))
	routes.RegisterPostRouter("/ggo-field/leave/prepare", wrapper.Wrapper(GGOFieldLeavePrepareHandler))
	routes.RegisterPostRouter("/ggo-field/leave/status", wrapper.Wrapper(GGOFieldLeaveStatusHandler))
	routes.RegisterPostRouter("/ggo-field/extraction/start", wrapper.Wrapper(GGOFieldExtractionStartHandler))
	routes.RegisterPostRouter("/ggo-field/extraction/cancel", wrapper.Wrapper(GGOFieldExtractionCancelHandler))
	routes.RegisterPostRouter("/ggo-field/extraction/status", wrapper.Wrapper(GGOFieldExtractionStatusHandler))
	routes.RegisterPostRouter("/ggo-field/pvp/receipt", wrapper.Wrapper(GGOFieldPVPReceiptHandler))
	routes.RegisterPostRouter("/ggo-field/monster/receipt", wrapper.Wrapper(GGOFieldMonsterReceiptHandler))
	routes.RegisterPostRouter("/ggo-field/monster/active", wrapper.Wrapper(GGOFieldMonsterActiveHandler))
	routes.RegisterPostRouter("/ggo-field/monster/claim", wrapper.Wrapper(GGOFieldMonsterClaimHandler))
	routes.RegisterPostRouter("/ggo-field/gather/start", wrapper.Wrapper(GGOFieldGatherStartHandler))
	routes.RegisterPostRouter("/ggo-field/gather/claim", wrapper.Wrapper(GGOFieldGatherClaimHandler))
	routes.RegisterPostRouter("/ggo-field/gather/cancel", wrapper.Wrapper(GGOFieldGatherCancelHandler))
	routes.RegisterPostRouter("/ggo-field/gather/status", wrapper.Wrapper(GGOFieldGatherStatusHandler))
	routes.RegisterPostRouter("/ggo-field/protection/release", wrapper.Wrapper(GGOFieldProtectionReleaseHandler))
}

func ggoFieldSession(auth model.GGOFieldAuth) ([]byte, error) {
	if auth.PlayerID < 1 || len(auth.SessionHex) != 32 || strings.ToLower(auth.SessionHex) != auth.SessionHex {
		return nil, model.NewErrParamsError()
	}
	value, err := hex.DecodeString(auth.SessionHex)
	if err != nil || len(value) != 16 {
		return nil, model.NewErrParamsError()
	}
	return value, nil
}

func ggoFieldRequestID(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id != uuid.Nil && id.String() == value
}

// GGOFieldSnapshotHandler returns the authenticated line's authoritative scene snapshot.
// @Summary 查询沙漠公共场景
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldSnapshotRequest true "角色和会话"
// @Success 200 {object} model.GGOFieldSnapshotResponse
// @Router /ggo-field/snapshot [post]
func GGOFieldSnapshotHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldSnapshotRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldSnapshot(c.Request.Context(), req.GGOFieldAuth, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// GGOFieldProtectionReleaseHandler confirms protection release without starting another action.
// @Summary 解除沙漠入场保护
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldProtectionReleaseRequest true "角色、会话和请求编号"
// @Success 200 {object} model.GGOFieldProtectionReleaseResponse
// @Router /ggo-field/protection/release [post]
func GGOFieldProtectionReleaseHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldProtectionReleaseRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.RequestID) {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldProtectionRelease(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// GGOFieldGatherStartHandler reserves and freezes one public gather attempt.
// @Summary 开始沙漠采集
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldGatherStartRequest true "角色、会话、点位、代次及请求编号"
// @Success 200 {object} model.GGOFieldGatherStartResponse
// @Router /ggo-field/gather/start [post]
func GGOFieldGatherStartHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldGatherStartRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.RequestID) || req.PointID == "" || len(req.PointID) > 64 || strings.TrimSpace(req.PointID) != req.PointID || req.Generation < 1 {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldGatherStart(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// GGOFieldGatherClaimHandler delivers the frozen gather result once.
// @Summary 领取沙漠采集
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldGatherClaimRequest true "角色、会话、开始编号、冻结令牌及请求编号"
// @Success 200 {object} model.GGOFieldGatherClaimResponse
// @Router /ggo-field/gather/claim [post]
func GGOFieldGatherClaimHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldGatherClaimRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.RequestID) ||
		!ggoFieldRequestID(req.StartRequestID) || req.RequestID == req.StartRequestID ||
		req.AttemptToken == "" || len(req.AttemptToken) > 64 || strings.TrimSpace(req.AttemptToken) != req.AttemptToken {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldGatherClaim(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// GGOFieldGatherCancelHandler records cancellation even before a late start arrives.
// @Summary 取消沙漠采集
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldGatherCancelRequest true "角色、会话和请求编号"
// @Success 200 {object} model.GGOFieldGatherCancelResponse
// @Router /ggo-field/gather/cancel [post]
func GGOFieldGatherCancelHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldGatherCancelRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.RequestID) ||
		!ggoFieldRequestID(req.StartRequestID) || req.RequestID == req.StartRequestID ||
		len(req.AttemptToken) > 64 || strings.TrimSpace(req.AttemptToken) != req.AttemptToken {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldGatherCancel(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// GGOFieldGatherStatusHandler restores an existing attempt without drawing again.
// @Summary 查询沙漠采集状态
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldGatherStatusRequest true "角色、会话和开始编号"
// @Success 200 {object} model.GGOFieldGatherStatusResponse
// @Router /ggo-field/gather/status [post]
func GGOFieldGatherStatusHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldGatherStatusRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.StartRequestID) {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldGatherStatus(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// GGOFieldEntryHandler teleports from the safe-area guide to a frozen legal entry point.
// @Summary 从向导进入沙漠危险区
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldEntryRequest true "角色、会话和请求编号"
// @Success 200 {object} model.GGOFieldEntryResponse
// @Router /ggo-field/entry [post]
func GGOFieldEntryHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldEntryRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.RequestID) {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldEntry(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// GGOFieldSettlementHandler converts the frozen carried chips at the safe NPC.
// @Summary 安全区终端结算
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldSettlementRequest true "角色、会话和请求编号"
// @Success 200 {object} model.GGOFieldSettlementResponse
// @Router /ggo-field/settlement [post]
func GGOFieldSettlementHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldSettlementRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.RequestID) {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldSettlement(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// GGOFieldLeavePrepareHandler records an explicit scene or line change target.
// @Summary 确认沙漠离开目标
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldLeavePrepareRequest true "角色、请求编号及目标"
// @Success 200 {object} model.GGOFieldLeavePrepareResponse
// @Router /ggo-field/leave/prepare [post]
func GGOFieldLeavePrepareHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldLeavePrepareRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.RequestID) {
		return model.NewErrParamsError()
	}
	if (req.Mode != "scene" || req.TargetScene == "" || req.TargetLine != 0) && (req.Mode != "line" || req.TargetScene != "" || req.TargetLine < 1) {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldLeavePrepare(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// GGOFieldLeaveStatusHandler recovers a confirmed or completed transition.
// @Summary 查询沙漠离开回执
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldLeaveStatusRequest true "角色和离开请求编号"
// @Success 200 {object} model.GGOFieldLeaveStatusResponse
// @Router /ggo-field/leave/status [post]
func GGOFieldLeaveStatusHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldLeaveStatusRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.LeaveRequestID) {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldLeaveStatus(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// @Summary 开始沙漠撤离读条
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldExtractionStartRequest true "角色、接应点和请求编号"
// @Success 200 {object} model.GGOFieldExtractionStartResponse
// @Router /ggo-field/extraction/start [post]
func GGOFieldExtractionStartHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldExtractionStartRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.RequestID) || req.PointID == "" || len(req.PointID) > 64 || strings.TrimSpace(req.PointID) != req.PointID {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldExtractionStart(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// @Summary 取消沙漠撤离
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldExtractionCancelRequest true "角色和开始请求编号"
// @Success 200 {object} model.GGOFieldExtractionStatusResponse
// @Router /ggo-field/extraction/cancel [post]
func GGOFieldExtractionCancelHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldExtractionCancelRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.RequestID) || !ggoFieldRequestID(req.StartRequestID) || req.RequestID == req.StartRequestID {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldExtractionCancel(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// @Summary 查询沙漠撤离
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldExtractionStatusRequest true "角色和开始请求编号"
// @Success 200 {object} model.GGOFieldExtractionStatusResponse
// @Router /ggo-field/extraction/status [post]
func GGOFieldExtractionStatusHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldExtractionStatusRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.StartRequestID) {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldExtractionStatus(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// @Summary 查询沙漠PK芯片结果
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldPVPReceiptRequest true "角色、会话和战斗编号"
// @Success 200 {object} model.GGOFieldPVPReceipt
// @Router /ggo-field/pvp/receipt [post]
func GGOFieldPVPReceiptHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldPVPReceiptRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoFieldRequestID(req.FightID) {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldPVPReceipt(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

func ggoMonsterFightID(value string) bool {
	if len(value) != 24 || strings.ToLower(value) != value {
		return false
	}
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == 12
}

// @Summary 查询沙漠小怪奖励
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldMonsterReceiptRequest true "角色和战斗编号"
// @Success 200 {object} model.GGOFieldMonsterReceipt
// @Router /ggo-field/monster/receipt [post]
func GGOFieldMonsterReceiptHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldMonsterReceiptRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoMonsterFightID(req.FightID) {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldMonsterReceipt(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// @Summary 补领沙漠小怪冻结奖励
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldMonsterClaimRequest true "角色、战斗编号和请求编号"
// @Success 200 {object} model.GGOFieldMonsterReceipt
// @Router /ggo-field/monster/claim [post]
func GGOFieldMonsterClaimHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldMonsterClaimRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil || !ggoMonsterFightID(req.FightID) || !ggoFieldRequestID(req.RequestID) {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldMonsterClaim(c.Request.Context(), req, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// @Summary 恢复沙漠小怪进行中战斗
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldSnapshotRequest true "角色和会话"
// @Success 200 {object} model.GGOFieldMonsterActiveResponse
// @Router /ggo-field/monster/active [post]
func GGOFieldMonsterActiveHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldSnapshotRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldMonsterActive(c.Request.Context(), req.GGOFieldAuth, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}

// GGOFieldPositionStatusHandler checks the current session's position handshake.
// @Summary 查询沙漠权威位置与握手版本
// @Description 不改变角色位置，强制传送后须等待当前会话的原位置握手
// @Tags GGOField
// @Accept json
// @Produce json
// @Param body body model.GGOFieldSnapshotRequest true "角色和会话"
// @Success 200 {object} model.GGOFieldPositionStatusResponse
// @Router /ggo-field/position/status [post]
func GGOFieldPositionStatusHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOFieldSnapshotRequest
	if c.ShouldBindJSON(&req) != nil || gameService == nil {
		return model.NewErrParamsError()
	}
	session, err := ggoFieldSession(req.GGOFieldAuth)
	if err != nil {
		return err
	}
	result, err := gameService.GGOFieldPositionStatus(c.Request.Context(), req.GGOFieldAuth, session)
	if err != nil {
		return err
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, model.NewBizCodeData(result))
	return nil
}
