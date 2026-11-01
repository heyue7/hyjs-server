package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestGGOPublishedVersionPreservesPendingActions(t *testing.T) {
	t.Setenv("HYJS_P1_MYSQL_DATABASE", "")
	f := ggoTradeSetup(t)
	if err := db.GetIns().AutoMigrate(&db.GGOFieldLine{}, &db.GGOFieldPoint{}); err != nil {
		t.Fatal(err)
	}
	f.s.GGOIntegrationReady = true
	now := time.Now().UnixMilli()
	var attempt db.P8GatherAttempt
	var oldMonster ggoMonsterNode
	err := db.GetIns().Transaction(func(tx *gorm.DB) error {
		row, state, _, err := ggoLoadLine(tx, f.s.OriginalServerID(), 1, f.config, "old-release", now)
		if err != nil {
			return err
		}
		n := &state.Resources[0]
		n.ReservedBy, n.AttemptToken, n.ReadyAtMS = f.ids[0], "old-gather-token", now+10000
		frozen := ggoGatherFrozen{p8GatherConfig: p8GatherConfig{Source: "ggo-v4:old-release", ResourceID: "group_mine_003", Kind: n.Kind, Level: int32(n.Level), GatherMS: 1000, CooldownMS: 1000, Times: 5, Fatigue: 4, Reward: "MineItem_009,1", Point: &pb.PointNew{X: proto.Float32(0), Y: proto.Float32(0), Z: proto.Float32(0)}, Radius: 3}, StartRequestID: "old-start", Line: 1, EconomyVersion: f.config.Runtime.EconomyVersion, MaterialNumerator: 5}
		raw, err := json.Marshal(frozen)
		if err != nil {
			return err
		}
		attempt = db.P8GatherAttempt{PlayerID: f.ids[0], Token: n.AttemptToken, Scene: constUtil.GGOFieldScene, PosID: n.PointID, Generation: n.Generation, StartedAt: now, ReadyAt: n.ReadyAtMS, Active: true, Snapshot: string(raw), Rewards: "MineItem_009,1"}
		if err = tx.Create(&attempt).Error; err != nil {
			return err
		}
		m := &state.Monsters[0]
		m.ReservedBy, m.FightID = f.ids[1], "old-monster-fight"
		oldMonster = *m
		return ggoLineSave(tx, row, state, now)
	})
	if err != nil {
		t.Fatal(err)
	}
	var currentGeneration int64
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		_, state, changed, err := ggoLoadLine(tx, f.s.OriginalServerID(), 1, f.config, "isolated-trade-config", now+10)
		if err != nil {
			return err
		}
		if !changed || !ggoLegacyBlocked(state, attempt.PosID) || !ggoLegacyBlocked(state, oldMonster.PointID) {
			t.Fatal("new release lost pending action blocks")
		}
		for _, node := range state.Resources {
			if node.PointID == attempt.PosID {
				currentGeneration = node.Generation
			}
		}
		if currentGeneration <= attempt.Generation {
			t.Fatal("new release reused old generation")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.s.GGOFieldGatherStart(f.ctx, model.GGOFieldGatherStartRequest{GGOFieldMutation: model.GGOFieldMutation{GGOFieldAuth: f.auth(1), RequestID: "00000000-0000-4000-8000-000000000099"}, PointID: attempt.PosID, Generation: currentGeneration}, f.sessions[1])
	var biz *model.BizCode
	if !errors.As(err, &biz) || biz.Code != constUtil.GGOFieldBusy {
		t.Fatalf("new generation did not reject prior reservation with busy: %v", err)
	}
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		var profile db.Profile
		if err := tx.First(&profile, f.ids[0]).Error; err != nil {
			return err
		}
		player, game, err := decodeProfile(&profile)
		if err != nil {
			return err
		}
		if _, err = ggoCancelAttempt(tx, f.s, f.ids[0], game, &attempt, "00000000-0000-4000-8000-000000000098", "cancel", now+20); err != nil {
			return err
		}
		return saveProfile(tx, &profile, player, game)
	})
	if err != nil {
		t.Fatal(err)
	}
	err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		row, state, _, err := ggoLoadLine(tx, f.s.OriginalServerID(), 1, f.config, "isolated-trade-config", now+30)
		if err != nil {
			return err
		}
		if ggoLegacyBlocked(state, attempt.PosID) {
			t.Fatal("completed cancellation still blocks new generation")
		}
		fight := &model.GGOFieldMonsterFight{FightID: oldMonster.FightID, MonsterUUID: oldMonster.UUID, PointID: oldMonster.PointID, Generation: oldMonster.Generation, DeploymentRevision: "old-release"}
		node, detached, err := ggoResolveMonsterReservation(state, fight, f.ids[1], now+30)
		if err != nil || !detached || node == nil || node.Generation == oldMonster.Generation || ggoLegacyBlocked(state, oldMonster.PointID) {
			t.Fatal("old battle did not release exact block", err)
		}
		if _, _, err = ggoResolveMonsterReservation(state, &model.GGOFieldMonsterFight{FightID: "different", PointID: node.PointID, Generation: node.Generation, DeploymentRevision: "isolated-trade-config"}, f.ids[1], now+30); err == nil {
			t.Fatal("same-version wrong reservation accepted")
		}
		return ggoLineSave(tx, row, state, now+30)
	})
	if err != nil {
		t.Fatal(err)
	}
}
