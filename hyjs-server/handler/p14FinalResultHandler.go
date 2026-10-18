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
	routes.RegisterPostRouter(constUtil.P14FinalResultImportPath, wrapper.Wrapper(P14FinalResultImportHandler))
}

// P14FinalResultImportHandler validates the exact signed canonical source realm result envelope.
// @Summary Import a trusted canonical source realm result
// @Description Accepts an exact HMAC signed canonical terminal result for this realm.
// @Tags Internal federation
// @Accept json
// @Produce json
// @Param X-HYJS-P14-Signature header string true "Hex HMAC SHA256 of the exact body"
// @Param body body model.P14FinalResultImportRequest true "Compressed immutable source terminal"
// @Success 200 {object} model.P14FinalResultImportResponse
// @Router /internal/p14/final-result [post]
func P14FinalResultImportHandler(c *gin.Context) error {
	// Keep the signed bytes intact while bounding decoding and rejecting extra fields.
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1048577))
	if err != nil || len(raw) == 0 || len(raw) > 1048576 {
		return model.NewErrParamsError()
	}
	var request model.P14FinalResultImportRequest
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
		request.ProtocolVersion != constUtil.P14FinalResultProtocolVersion ||
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
	// The service authenticates the trusted peer and commits the immutable remote result without a local P6 owner.
	response, err := gameService.ImportP14FinalResult(c.Request.Context(), &request, raw, signature)
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(response))
	return nil
}
