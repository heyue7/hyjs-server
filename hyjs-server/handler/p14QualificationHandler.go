package handler

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	"hyjs-server/routes"
	"hyjs-server/wrapper"
)

func init() {
	routes.RegisterPostRouter(constUtil.P14QualificationPath, wrapper.Wrapper(P14QualificationImportHandler))
}

// P14QualificationImportHandler validates the exact signed internal certificate envelope.
func P14QualificationImportHandler(c *gin.Context) error {
	raw, err := io.ReadAll(io.LimitReader(c.Request.Body, 1048577))
	if err != nil || len(raw) == 0 || len(raw) > 1048576 {
		return model.NewErrParamsError()
	}
	var request model.P14QualificationImportRequest
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
	if err != nil || len(signed) != 32 || idErr != nil || len(id) != 32 || request.ProtocolVersion <= 0 || request.SourceServerID <= 0 || request.SourceServerID > math.MaxInt32 || request.DestinationServerID <= 0 || request.DestinationServerID > math.MaxInt32 || request.SeasonIndex <= 0 || request.Source != constUtil.P14QualificationSource {
		return model.NewErrParamsError()
	}
	for _, value := range []json.RawMessage{request.Boundary, request.Roster, request.Qualification} {
		if len(value) == 0 || len(value) > 1048576 || !json.Valid(value) {
			return model.NewErrParamsError()
		}
	}
	if gameService == nil {
		return model.NewErrSystemError()
	}
	response, err := gameService.ImportP14Qualification(c.Request.Context(), request, raw, signature, time.Now())
	if err != nil {
		return err
	}
	c.JSON(http.StatusOK, model.NewBizCodeData(response))
	return nil
}
