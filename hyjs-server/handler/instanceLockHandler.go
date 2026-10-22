package handler

import (
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/service"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter("/instance-lock/state", wrapper.Wrapper(InstanceLockStateHandler))
	routes.RegisterPostRouter("/instance-lock/set", wrapper.Wrapper(InstanceLockSetHandler))
}

// InstanceLockStateHandler 查询角色和装备的锁定状态。
// @Summary 读取实例锁定状态
// @Description 按当前游戏会话返回拥有角色及装备的锁定集合
// @Tags InstanceLock
// @Accept json
// @Produce json
// @Param body body model.InstanceLockStateRequest true "当前游戏会话"
// @Success 200 {object} model.InstanceLockResponse
// @Router /instance-lock/state [post]
func InstanceLockStateHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req model.InstanceLockStateRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil {
		return model.NewErrParamsError()
	}
	out, err := service.InstanceLockState(c.Request.Context(), req.PlayerID, session)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}

// InstanceLockSetHandler 保存拥有实例的锁定或解锁选择。
// @Summary 锁定或解锁角色和装备
// @Description 按原游戏会话、实时归属和锁定版本保存；锁定后禁止回收、售卖和材料消耗
// @Tags InstanceLock
// @Accept json
// @Produce json
// @Param body body model.InstanceLockSetRequest true "目标及锁定状态"
// @Success 200 {object} model.InstanceLockResponse
// @Router /instance-lock/set [post]
func InstanceLockSetHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req model.InstanceLockSetRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 || req.GUID == "" || len(req.GUID) > 128 || req.Revision < 0 {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil {
		return model.NewErrParamsError()
	}
	if _, err = service.AuthorizeAccount(c.Request.Context(), req.PlayerID, session); err != nil {
		return err
	}
	out, err := service.SetInstanceLock(c.Request.Context(), req.PlayerID, session, req.GUID, *req.Locked, req.Revision)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}
