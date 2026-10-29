package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"hyjs-server/service"
)

func feedback84WantedHTTP(t *testing.T, id, stamp int64, session []byte, message proto.Message) proto.Message {
	t.Helper()
	response := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(response)
	c.Request = httptest.NewRequest(http.MethodPost, "/sao/", bytes.NewReader(trial82HTTPFrame(t, id, stamp, session, message)))
	GameHandler(c)
	raw := response.Body.Bytes()
	if response.Code != http.StatusOK || len(raw) < 8 || int(binary.BigEndian.Uint32(raw)) > len(raw)-8 {
		t.Fatal("invalid original HTTP frame", response.Code, len(raw))
	}
	length := int(binary.BigEndian.Uint32(raw))
	result, err := model.GameMessage(model.OpcodeNames[int32(binary.BigEndian.Uint32(raw[4:]))], nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := proto.Unmarshal(raw[8:8+length], result); err != nil {
		t.Fatal(err)
	}
	return result
}

// A finite original boss is a prerequisite; the native signed HTTP consumers
// own all costs, computed battle facts, rejection, recovery and settlement.
func TestFeedback84WantedSignedHTTPRecoveryAndErrorCodes(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "wanted84-http.sqlite3")); err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	t.Cleanup(func() { connection.Close() })
	s := &service.GameService{}
	for _, load := range []func(string) error{s.LoadTables, s.LoadP5Tables, s.LoadP7Tables, s.LoadP11Tables, s.LoadP12WantedTables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	for _, init := range []func(context.Context) error{s.InitializeGameplayPools, s.InitializeP1, s.InitializeP6, s.InitializeP4, s.InitializeP5, s.InitializeOpenServerActivity, s.InitializeDailyTasks, s.InitializeP12Wanted} {
		if err := init(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s.P12WantedReady = true
	if err := db.GetIns().Model(&db.P12WantedPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	account, err := service.LoginAccount(ctx, "wanted84-wire", "isolated-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.CreatePlayer(ctx, account.Id, "悬赏协议验证", "hero_047"); err != nil {
		t.Fatal(err)
	}
	var row db.Profile
	if err := db.GetIns().First(&row, account.Id).Error; err != nil {
		t.Fatal(err)
	}
	var player pb.Player
	var game model.GameState
	if json.Unmarshal([]byte(*row.Player), &player) != nil || json.Unmarshal([]byte(*row.Game), &game) != nil {
		t.Fatal("profile fixture")
	}
	player.Level, player.WantedAttTimes = proto.Int32(35), proto.Int32(10)
	playerRaw, _ := json.Marshal(&player)
	if err := db.GetIns().Model(&row).Update("player", string(playerRaw)).Error; err != nil {
		t.Fatal(err)
	}
	previous := gameService
	ConfigureGame(s)
	t.Cleanup(func() { gameService = previous })
	if result := feedback84WantedHTTP(t, account.Id, 0, account.Session, &pb.CSGetRebels{}); result == nil {
		t.Fatal("missing list")
	} else if failure, ok := result.(*pb.ErrorMessage); ok {
		t.Fatal(failure)
	}
	var policy db.P12WantedPolicy
	if err := db.GetIns().First(&policy, 1).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	instance := db.P12WantedInstance{ID: "wanted84-http-original", ServerID: 1, FinderID: account.Id, FinderName: "finite original prerequisite", WantedID: "1", Level: 1, Difficulty: 1, State: constUtil.P12WantedActive, CreatedAt: now.UnixMilli(), ExpiresAt: now.Add(30 * time.Minute).UnixMilli(), MonsterHPJSON: "null", DefinitionJSON: policy.DefinitionJSON, DefinitionHash: policy.DefinitionHash}
	preview, _, err := s.BuildP12WantedStart(&player, &game, &instance, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	var definition struct {
		Tables map[string][]map[string]string `json:"tables"`
	}
	if err := json.Unmarshal([]byte(policy.DefinitionJSON), &definition); err != nil {
		t.Fatal(err)
	}
	hps := map[string]float64{}
	for _, enemy := range definition.Tables["WantedEnemy"] {
		if enemy["WantedID"] != "1" {
			continue
		}
		guid := "wanted-" + strings.TrimPrefix(enemy["WantedEnemyId"], "wanted_")
		for _, wave := range preview.BattleMonsterList {
			for _, actor := range wave.Formation {
				if actor.GetGuid() == guid {
					hps[enemy["WantedEnemyId"]] = actor.GetInst().GetCurHp()
					instance.MaxHP += actor.GetProp().GetHp()
				}
			}
		}
	}
	if len(hps) == 0 || instance.MaxHP <= 0 {
		t.Fatal("original boss HP absent")
	}
	instance.CurrentHP = instance.MaxHP
	hpRaw, _ := json.Marshal(hps)
	instance.MonsterHPJSON = string(hpRaw)
	if err := db.GetIns().Create(&instance).Error; err != nil {
		t.Fatal(err)
	}
	startMessage := feedback84WantedHTTP(t, account.Id, 84011, account.Session, &pb.CSRebelFightStart{RebelId: proto.String(instance.ID), Type: proto.Int32(1)})
	start, ok := startMessage.(*pb.SCRebelFightStart)
	if !ok {
		t.Fatal("signed original Start failed", startMessage)
	}
	var fight db.P12WantedFight
	if err := db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal([]byte(fight.TerminalJSON), &terminal) != nil || terminal.Outcome == nil || terminal.OriginalOperates == nil {
		t.Fatal("actual computed terminal absent")
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	end := &pb.CSRebelFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	bad := proto.Clone(end).(*pb.CSRebelFightEnd)
	bad.OptRecord.DefenderTotalLeftHp = proto.Float64(bad.OptRecord.GetDefenderTotalLeftHp() + 1)
	failure, ok := feedback84WantedHTTP(t, account.Id, 84012, account.Session, bad).(*pb.ErrorMessage)
	if !ok || failure.GetCode() != constUtil.P12WantedRetryableEndCode || !strings.HasPrefix(failure.GetMsg(), constUtil.P6ReplayConflict) {
		t.Fatal("specific native ErrorMessage mapping", failure)
	}
	recovery := &pb.CSRebelFightStart{RebelId: proto.String(constUtil.P12WantedRecoveryPrefix + fight.ID), Type: proto.Int32(1)}
	recovered, ok := feedback84WantedHTTP(t, account.Id, 84013, account.Session, recovery).(*pb.SCRebelFightStart)
	if !ok || !proto.Equal(recovered, start) {
		t.Fatal("named original 55003 failed")
	}
	accepted, ok := feedback84WantedHTTP(t, account.Id, 84014, account.Session, end).(*pb.SCRebelFightEnd)
	if !ok {
		t.Fatal("actual End failed")
	}
	repeated, ok := feedback84WantedHTTP(t, account.Id, 84015, account.Session, end).(*pb.SCRebelFightEnd)
	if !ok || !proto.Equal(accepted, repeated) {
		t.Fatal("exact HTTP receipt changed")
	}
	failure, ok = feedback84WantedHTTP(t, account.Id, 84016, account.Session, recovery).(*pb.ErrorMessage)
	if !ok || failure.GetCode() != constUtil.ErrGameRequest || failure.GetMsg() != constUtil.P12WantedRecoveryCommitted {
		t.Fatal("committed recovery error mapping", failure)
	}
	var count int64
	if err := db.GetIns().Model(&db.P12WantedFight{}).Where("player_id=?", account.Id).Count(&count).Error; err != nil || count != 1 {
		t.Fatal("HTTP recovery created another paid fight", count, err)
	}
	endRaw, _ := proto.MarshalOptions{Deterministic: true}.Marshal(end)
	digest := sha256.Sum256(endRaw)
	if err := db.GetIns().First(&fight, "id=?", fight.ID).Error; err != nil || !fight.Committed || fight.EndRequestHash != hex.EncodeToString(digest[:]) {
		t.Fatal("accepted original request identity", err)
	}
}
