package handler

import (
	"context"
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
	"net/http"
	"strings"
	"unicode"
)

func init() {
	routes.RegisterPostRouter("/ggo-shop/catalogue", wrapper.Wrapper(GGOShopCatalogueHandler))
	routes.RegisterPostRouter("/ggo-shop/buy", wrapper.Wrapper(GGOShopBuyHandler))
}

// GGOShopCatalogueHandler 查询荒野兑换所。
// @Summary 查询荒野兑换所
// @Description 返回当前目录版本、回收信用点余额、价格和剩余次数。
// @Tags GGOShop
// @Accept json
// @Produce json
// @Param body body model.GGOShopCatalogueRequest true "角色与当前会话"
// @Success 200 {object} model.GGOShopCatalogueResponse
// @Router /ggo-shop/catalogue [post]
func GGOShopCatalogueHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var req model.GGOShopCatalogueRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 || gameService == nil {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	ctx := context.WithValue(c.Request.Context(), constUtil.GameServiceContextKey, gameService)
	out, err := gameService.GGOShopCatalogue(ctx, req.PlayerID, session)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}

// GGOShopBuyHandler 购买一份商品。
// @Summary 荒野兑换所兑换
// @Description 使用当前目录版本和UUID购买，服务端校验价格与限购，超频道具到账账号终端；重试返回原回执。
// @Tags GGOShop
// @Accept json
// @Produce json
// @Param body body model.GGOShopBuyRequest true "角色、会话、商品、版本及UUID"
// @Success 200 {object} model.GGOShopBuyResponse
// @Router /ggo-shop/buy [post]
func GGOShopBuyHandler(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2048)
	var req model.GGOShopBuyRequest
	if c.ShouldBindJSON(&req) != nil || req.PlayerID < 1 || len(req.SessionHex) != 32 || gameService == nil || req.ProductID == "" || len(req.ProductID) > 64 || strings.TrimSpace(req.ProductID) != req.ProductID || strings.IndexFunc(req.ProductID, unicode.IsControl) >= 0 || req.Version == "" || len(req.Version) > 64 || len(req.RequestID) != 36 {
		return model.NewErrParamsError()
	}
	id, err := uuid.Parse(req.RequestID)
	if err != nil || id.String() != req.RequestID || id == uuid.Nil {
		return model.NewErrParamsError()
	}
	session, err := hex.DecodeString(req.SessionHex)
	if err != nil || len(session) != 16 {
		return model.NewErrParamsError()
	}
	ctx := context.WithValue(c.Request.Context(), constUtil.GameServiceContextKey, gameService)
	out, err := gameService.GGOShopBuy(ctx, req, session)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(out))
	return nil
}
