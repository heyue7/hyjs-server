package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Real tower launch/impact and natural-revive producers are exercised. Only
// initial low HP and two carried ore are limited fixtures; no P6 winner or
// authority certificate is fabricated.
func TestP13UFResourceTowerDeathRefusesActionsPreservesClockColdRevive(t *testing.T) {
	for _, collecting := range []bool{false, true} {
		t.Run(fmt.Sprintf("collecting-%t", collecting), func(t *testing.T) {
			f, ids, first, clock, match := p13UFFieldSetup(t)
			now := func() time.Time { return time.UnixMilli(clock.Load()) }
			if err := f.s.InitializeP6(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated tower-death/resource lifecycle fixture"}).Error; err != nil {
				t.Fatal(err)
			}
			var roster p13UFMatchRoster
			if json.Unmarshal(match.Roster, &roster) != nil {
				t.Fatal("roster")
			}
			camp := int64(-1)
			for c, side := range [][]p13UFMatchFighter{roster.Left, roster.Right} {
				for _, fighter := range side {
					if fighter.PlayerID == ids[0] {
						camp = int64(c)
					}
				}
			}
			if camp < 0 {
				t.Fatal("actual participant absent")
			}
			original, err := f.s.p13UFFieldOriginal(match, db.GetIns())
			if err != nil {
				t.Fatal(err)
			}
			var chosen map[string]string
			for _, row := range original.Tables["UnionFightTower"] {
				if row["Camp"] != fmt.Sprint(camp) {
					chosen = row
					break
				}
			}
			if chosen == nil {
				t.Fatal("enemy original tower")
			}
			point, err := fieldPoint(chosen["Position"])
			if err != nil {
				t.Fatal(err)
			}
			// The admitted field map projects original tower height onto ground Y=0.
			point.Y = proto.Float32(0)
			// Each subtest owns these source rows in the shared isolated schema.
			// Keep production uniqueness and remove only this fixture on exit.
			t.Cleanup(func() {
				if e := db.GetIns().Where("day=? AND source_key=?", match.Day, "tower-death-node").Delete(&db.P13UnionFightResourceSource{}).Error; e != nil {
					t.Error(e)
				}
				if e := db.GetIns().Where("day=? AND version=?", match.Day, "isolated-resource-death").Delete(&db.P13UnionFightTowerSource{}).Error; e != nil {
					t.Error(e)
				}
			})
			if err = db.GetIns().Create(&db.P13UnionFightResourceSource{Day: match.Day, SourceKey: "tower-death-node", ResourceID: "1", Enabled: true, X: point.GetX(), Y: point.GetY(), Z: point.GetZ(), RewardRes: 2, PublishedAt: clock.Load(), Source: "isolated original tower-position/node1 fixture"}).Error; err != nil {
				t.Fatal(err)
			}
			for id := int64(1); id <= 6; id++ {
				if err = db.GetIns().Create(&db.P13UnionFightTowerSource{Day: match.Day, TowerID: id, Enabled: true, Version: "isolated-resource-death", HPPercent: 5, PlayerAttack: 65, PlayerDamageRule: constUtil.P13UnionFightTowerPlayerDamageRule, PublishedAt: clock.Load(), Source: "isolated ATT65/car5 producer qualification"}).Error; err != nil {
					t.Fatal(err)
				}
			}
			var profile db.Profile
			if err = db.GetIns().First(&profile, ids[0]).Error; err != nil {
				t.Fatal(err)
			}
			t.Cleanup(f.s.RegisterP10Client(f.ctx, ids[0], profile.Session, func(proto.Message) error { return nil }))
			if _, err = f.s.p13UFFieldEnter(f.ctx, ids[0], now()); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.p13UFFieldFrames(f.ctx, ids[0], &pb.CSCurrentPosition{}, now()); err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.SyncP13UnionFightField(f.ctx, ids[0], false); err != nil {
				t.Fatal(err)
			}
			var actor db.P13UnionFightFieldPlayer
			read := func() *pb.InstUFPlayer {
				t.Helper()
				if err := db.GetIns().First(&actor, "match_id=? AND player_id=?", match.ID, ids[0]).Error; err != nil {
					t.Fatal(err)
				}
				frame, err := p13UFFieldReadActor(&actor)
				if err != nil {
					t.Fatal(err)
				}
				return frame
			}
			frame := read()
			var tower db.P13UnionFightTowerState
			if err = db.GetIns().Where("match_id=? AND camp<>? AND tower_id=?", match.ID, camp, chosen["UFTId"]).Order("tower_id").First(&tower).Error; err != nil {
				t.Fatal(err)
			}
			actor.X, actor.Y, actor.Z = tower.X, tower.Y, tower.Z
			frame.OwnRes = &pb.InstUFResource{ResourceNum: proto.Float64(2)}
			for i := range frame.FightInfo.HpPos {
				frame.FightInfo.HpPos[i] = 0
			}
			frame.FightInfo.HpPos[0] = 1
			if err = p13UFFieldSaveActor(db.GetIns(), &actor, frame); err != nil {
				t.Fatal(err)
			}
			var node db.P13UnionFightResourceNode
			if err = db.GetIns().First(&node, "match_id=? AND guid=?", match.ID, p13UFFieldGUID(match.ID, "resource", "tower-death-node")).Error; err != nil {
				t.Fatal(err)
			}
			clock.Store(first + 1000)
			if collecting {
				request := &pb.CSUFPlayerCollect{ResGuid: proto.Int64(node.GUID)}
				if _, err = f.s.p13UFResourceBegin(f.ctx, ids[0], request, now()); err != nil {
					t.Fatal(err)
				}
				if _, err = f.s.p13UFResourceBegin(f.ctx, ids[0], request, now()); err != nil {
					t.Fatal("same state2 lease retry", err)
				}
			}
			if err = f.s.TickP13UnionFightTower(f.ctx, now()); err != nil {
				t.Fatal(err)
			}
			clock.Store(first + 1600)
			if err = f.s.TickP13UnionFightTower(f.ctx, now()); err != nil {
				t.Fatal(err)
			}
			dead := read()
			if dead.GetState() != 3 || dead.GetDeadNum() != 1 || dead.FightInfo.HpPos[0] != 0 {
				t.Fatal("real tower did not kill", dead)
			}
			var hit db.P13UnionFightTowerHit
			if err = db.GetIns().Where("match_id=? AND dead_player_id=?", match.ID, ids[0]).First(&hit).Error; err != nil {
				t.Fatal(err)
			}
			if err = db.GetIns().First(&tower, "match_id=? AND tower_guid=?", match.ID, hit.TowerGUID).Error; err != nil {
				t.Fatal(err)
			}
			if hit.ReviveAt != clock.Load()+5000 || p13UFTowerHitVerify(&hit, &tower) != nil {
				t.Fatal("real tower death proof/clock", hit.ReviveAt)
			}
			for _, model := range []any{&db.P13UnionFightPlayerBattleAttempt{}, &db.P13UnionFightGuardBattleAttempt{}} {
				var count int64
				if err = db.GetIns().Model(model).Where("match_id=?", match.ID).Count(&count).Error; err != nil || count != 0 {
					t.Fatal("test must have no ordinary/guard history", count, err)
				}
			}
			beforeHash := actor.FrameHash
			if _, err = f.s.p13UFResourceBegin(f.ctx, ids[0], &pb.CSUFPlayerCollect{ResGuid: proto.Int64(node.GUID)}, now()); err == nil || !strings.Contains(err.Error(), "死亡") {
				t.Fatal("tower corpse mining did not hit life guard", err)
			}
			npc := "2"
			if camp == 1 {
				npc = "5"
			}
			if _, err = f.s.p13UFResourceDeposit(f.ctx, ids[0], &pb.CSUFPlayerUpResource{NpcId: proto.String(npc)}, 777, now()); err == nil || !strings.Contains(err.Error(), "死亡") {
				t.Fatal("tower corpse deposit did not hit life guard", err)
			}
			if collecting {
				err = db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
					var gate db.P13GuildPolicy
					if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
						return err
					}
					_, _, err := f.s.p13UFResourceCancel(tx, match.ID, ids[0], now())
					return err
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			afterCancel := read()
			if actor.FrameHash != beforeHash || !proto.Equal(afterCancel, dead) {
				t.Fatal("mining cancellation overwrote independent tower death clock")
			}
			if err = db.GetIns().First(&node, "match_id=? AND guid=?", match.ID, node.GUID).Error; err != nil || node.RemainingUses != 5 || node.BusyTaskID != "" {
				t.Fatal("canceled corpse consumed node", node, err)
			}
			sql, err := db.GetIns().DB()
			if err != nil {
				t.Fatal(err)
			}
			if err = sql.Close(); err != nil {
				t.Fatal(err)
			}
			f.open(t)
			if err = f.s.InitializeP13Guild(f.ctx); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(f.s.RegisterP10Client(f.ctx, ids[0], profile.Session, func(proto.Message) error { return nil }))
			clock.Store(hit.ReviveAt - 1)
			if err = f.s.TickP13UnionFightNaturalRevive(f.ctx, now()); err != nil {
				t.Fatal(err)
			}
			if frame = read(); frame.GetState() != 3 || frame.GetDeadNum() != 1 {
				t.Fatal("cold restart lost frozen due")
			}
			clock.Store(hit.ReviveAt)
			if err = f.s.TickP13UnionFightNaturalRevive(f.ctx, now()); err != nil {
				t.Fatal(err)
			}
			revived := read()
			if revived.GetState() != 0 || revived.GetDeadNum() != 1 || !p13UFResourceLiving(revived) || revived.FightInfo.GetAnger() != 0 {
				t.Fatal("natural revival missing after refused actions", revived)
			}
			var receipt db.P13UnionFightReviveReceipt
			if err = db.GetIns().First(&receipt, "match_id=? AND player_id=? AND dead_ordinal=1", match.ID, ids[0]).Error; err != nil || receipt.DeathKind != "tower" || receipt.DueAt != hit.ReviveAt {
				t.Fatal("real tower natural revival receipt", receipt, err)
			}
		})
	}
}
