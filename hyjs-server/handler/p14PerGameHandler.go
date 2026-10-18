package handler

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter(constUtil.P14PerGameImportPath, wrapper.Wrapper(P14PerGameImportHandler))
}

// P14PerGameImportHandler validates the exact signed per-game formation or origin seal envelope.
// @Summary Import a trusted per-game formation or origin seal
// @Description Accepts an exact HMAC signed final formation seal for this realm.
// @Tags Internal federation
// @Accept json
// @Produce json
// @Param X-HYJS-P14-Signature header string true "Hex HMAC SHA256 of the exact body"
// @Param body body model.P14PerGameImportRequest true "Immutable source lock boundary"
// @Success 200 {object} model.P14PerGameImportResponse
// @Router /internal/p14/per-game-formation [post]
func P14PerGameImportHandler(c *gin.Context) error {
	// Keep the signed bytes intact while bounding decoding and rejecting extra fields.
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1048577))
	if err != nil || len(raw) == 0 || len(raw) > 1048576 {
		return model.NewErrParamsError()
	}
	var request model.P14PerGameImportRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(&request); err != nil {
		return model.NewErrParamsError()
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return model.NewErrParamsError()
	}
	signature := c.GetHeader("X-HYJS-P14-Signature")
	signed, err := hex.DecodeString(signature)
	id, idErr := hex.DecodeString(request.DeliveryID)
	if err != nil || len(signed) != 32 || idErr != nil || len(id) != 32 ||
		request.ProtocolVersion != constUtil.P14PerGameProtocolVersion ||
		request.SourceServerID <= 0 || request.SourceServerID > 32 ||
		request.DestinationServerID <= 0 || request.DestinationServerID > 32 ||
		request.SourceServerID == request.DestinationServerID {
		return model.NewErrParamsError()
	}
	snapshot := bytes.TrimSpace(request.Snapshot)
	if len(snapshot) < 2 || snapshot[0] != '{' || !json.Valid(snapshot) {
		return model.NewErrParamsError()
	}
	if gameService == nil {
		return model.NewErrSystemError()
	}
	// The service authenticates the trusted peer and commits the immutable source seal.
	response, err := gameService.ImportP14PerGame(c.Request.Context(), request, raw, signature)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(response))
	return nil
}
