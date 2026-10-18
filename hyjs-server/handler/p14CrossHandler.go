package handler

import (
	"encoding/hex"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
	"io"
	"math"
	"net/http"
	"strings"
	"time"
)

func init() {
	routes.RegisterPostRouter(constUtil.P14DeliveryPath, wrapper.Wrapper(P14SnapshotImportHandler))
}

// P14SnapshotImportHandler 验证内部载荷结构后交由业务服务执行原服互信与持久化导入。
// @Summary 导入可信原服跨服阵容
// @Description 原服以HMAC签名精确载荷提交已认证角色快照，客户端不能调用此互信入口。
// @Tags P14内部互信
// @Accept json
// @Produce json
// @Param X-HYJS-P14-Signature header string true "载荷HMAC-SHA256签名"
// @Param body body model.P14SnapshotImportRequest true "可信原服快照"
// @Success 200 {object} model.P14SnapshotImportResponse
// @Router /internal/p14/snapshot [post]
func P14SnapshotImportHandler(c *gin.Context) error {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1048577))
	if err != nil || len(raw) == 0 || len(raw) > 1048576 {
		return model.NewErrParamsError()
	}
	var request model.P14SnapshotImportRequest
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&request); err != nil {
		return model.NewErrParamsError()
	}
	var extra any
	if err = decoder.Decode(&extra); err != io.EOF {
		return model.NewErrParamsError()
	}
	signature := c.GetHeader("X-HYJS-P14-Signature")
	decoded, err := hex.DecodeString(signature)
	if err != nil || len(decoded) != 32 || request.ProtocolVersion <= 0 || request.SourceServerID <= 0 || request.SourceServerID > math.MaxInt32 || request.DestinationServerID <= 0 || request.DestinationServerID > math.MaxInt32 || request.PlayerID <= 0 || request.SeasonIndex <= 0 || request.Revision <= 0 || request.CapturedAt <= 0 || request.FightPower <= 0 || len(request.Formation) == 0 || len(request.Battle) == 0 || len(request.Formation) > 262144 || len(request.Battle) > 524288 || len(request.ClientVersion) == 0 || len(request.ClientVersion) > 128 || len(request.EngineVersion) == 0 || len(request.EngineVersion) > 128 {
		return model.NewErrParamsError()
	}
	if request.ProtocolVersion != constUtil.P14ProtocolVersion && request.ProtocolVersion != constUtil.P14PromotionFormationProtocolVersion && request.ProtocolVersion != constUtil.P14FinalFormationProtocolVersion {
		return model.NewErrParamsError()
	}
	if request.ProtocolVersion == constUtil.P14FinalFormationProtocolVersion {
		if request.RoundStage != constUtil.P14Final16Stage {
			return model.NewErrParamsError()
		}
		for _, value := range []string{request.FinalQualificationHash, request.FinalOfferHash} {
			decoded, err = hex.DecodeString(value)
			if err != nil || len(decoded) != 32 {
				return model.NewErrParamsError()
			}
		}
	} else if request.RoundStage != 0 || request.FinalQualificationHash != "" || request.FinalOfferHash != "" {
		return model.NewErrParamsError()
	}
	for _, value := range []string{request.DeliveryID, request.SnapshotHash, request.ConfigurationHash} {
		decoded, err = hex.DecodeString(value)
		if err != nil || len(decoded) != 32 {
			return model.NewErrParamsError()
		}
	}
	if gameService == nil {
		return model.NewErrSystemError()
	}
	response, err := gameService.ImportP14Snapshot(c.Request.Context(), request, raw, signature, time.Now())
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(response))
	return nil
}
