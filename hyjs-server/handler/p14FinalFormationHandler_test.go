package handler

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"hyjs-server/model"
)

func TestP14FinalFormationPurpose3AndLegacySnapshotRequestBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previous := gameService
	gameService = nil
	defer func() { gameService = previous }()
	valid := model.P14SnapshotImportRequest{ProtocolVersion: 3, DeliveryID: strings.Repeat("a", 64), SourceServerID: 1, DestinationServerID: 2, SeasonIndex: 61, PlayerID: 1, Revision: 1, CapturedAt: 1, FightPower: 330, Formation: []byte{1}, Battle: []byte{2}, SnapshotHash: strings.Repeat("b", 64), ConfigurationHash: strings.Repeat("c", 64), ClientVersion: "client", EngineVersion: "engine", RoundStage: 8, FinalQualificationHash: strings.Repeat("d", 64), FinalOfferHash: strings.Repeat("e", 64)}
	run := func(request model.P14SnapshotImportRequest) error {
		raw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("POST", "/internal/p14/snapshot", bytes.NewReader(raw))
		c.Request.Header.Set("X-HYJS-P14-Signature", strings.Repeat("f", 64))
		return P14SnapshotImportHandler(c)
	}
	systemErr := run(valid)
	if systemErr == nil {
		t.Fatal("missing service accepted request")
	}
	for _, version := range []int32{1, 2} {
		legacy := valid
		legacy.ProtocolVersion = version
		legacy.RoundStage = 0
		legacy.FinalQualificationHash = ""
		legacy.FinalOfferHash = ""
		if err := run(legacy); err == nil || err.Error() != systemErr.Error() {
			t.Fatal("legacy request no longer reaches service", version, err)
		}
	}
	for _, mutate := range []func(*model.P14SnapshotImportRequest){
		func(r *model.P14SnapshotImportRequest) { r.ProtocolVersion = 4 },
		func(r *model.P14SnapshotImportRequest) { r.ProtocolVersion = 0 },
		func(r *model.P14SnapshotImportRequest) { r.RoundStage = 0 },
		func(r *model.P14SnapshotImportRequest) { r.RoundStage = 7 },
		func(r *model.P14SnapshotImportRequest) { r.RoundStage = 10 },
		func(r *model.P14SnapshotImportRequest) { r.FinalQualificationHash = strings.Repeat("d", 63) },
		func(r *model.P14SnapshotImportRequest) { r.FinalQualificationHash = strings.Repeat("g", 64) },
		func(r *model.P14SnapshotImportRequest) { r.FinalOfferHash = "" },
		func(r *model.P14SnapshotImportRequest) { r.FinalOfferHash = strings.Repeat("e", 66) },
		func(r *model.P14SnapshotImportRequest) { r.ProtocolVersion = 1 },
		func(r *model.P14SnapshotImportRequest) { r.ProtocolVersion = 2 },
	} {
		changed := valid
		mutate(&changed)
		if err := run(changed); err == nil || err.Error() == systemErr.Error() {
			t.Fatal("invalid purpose metadata reached service")
		}
	}
}
