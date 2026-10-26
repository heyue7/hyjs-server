package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// The prayer comes from the original guild/donation/paid-prayer consumers.
// Only the derived cache is damaged; a query must persist its repair even when
// the existing guild refresh has already corrected that value in memory.
func TestP5PowerCache75GuildQueryPersistsAlreadyProjectedPower(t *testing.T) {
	f, ids, now := p13PrayerSetup(t)
	id := ids[0]
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.s.P5Ready = true
	ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	ctx = context.WithValue(ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time { return now })
	p13PrayerPublish(t, now.Add(-time.Second))
	if _, err := f.s.p13GuildPrayer(ctx, id, &pb.CSUnionPray{}, 755101, now); err != nil {
		t.Fatal(err)
	}
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	p, g, err := decodeProfile(&row)
	if err != nil {
		t.Fatal(err)
	}
	var want int32
	if err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := f.s.PrepareP5GuildPrayerProperties(ctx, tx, p, g, now); err != nil {
			return err
		}
		if g.GuildPrayerProjection.SourceHash == "" {
			t.Fatal("original prayer did not produce a current projection")
		}
		want, err = f.s.formationFightPower(p, g)
		return err
	}); err != nil || want <= 1 {
		t.Fatal("original projected power", want, err)
	}
	// Stable, finite resource prerequisites ensure recovery cannot accidentally
	// cause the save that this regression requires from the power difference.
	p.Energy = proto.Int32(constUtil.EnergyRecoveryLimit)
	p.Stamina = proto.Int32(constUtil.StaminaRecoveryLimit)
	p.EnergyLastRefreshTime = proto.Int64(time.Now().Add(-time.Second).UnixMilli())
	p.StaminaLastRefreshTime = proto.Int64(time.Now().Add(-time.Second).UnixMilli())
	p.FightPower = proto.Int32(1)
	probe := proto.Clone(p).(*pb.Player)
	if changed, e := settleResourceRecovery(probe, time.Now().UnixMilli()); e != nil || changed {
		t.Fatal("recovery would hide missing power-only persistence", changed, e)
	}
	broken, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if result := db.GetIns().Model(&db.Profile{}).Where("id=?", id).Update("player", string(broken)); result.Error != nil || result.RowsAffected != 1 {
		t.Fatal("isolated derived-cache prerequisite", result.RowsAffected, result.Error)
	}
	brokenGame, err := json.Marshal(g)
	if err != nil {
		t.Fatal(err)
	}
	query := context.WithValue(ctx, constUtil.GameRequestContextKey, &pb.CSPlayer{})
	got, _, err := f.s.resourceProfile(query, id)
	if err != nil || got.GetFightPower() != want {
		t.Fatal("current guild query reply", got.GetFightPower(), want, err)
	}
	if err = db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	after, ag, err := decodeProfile(&row)
	if err != nil || after.GetFightPower() != want {
		t.Fatal("guild-projected repair was returned but not persisted", err)
	}
	after.FightPower = p.FightPower
	agraw, err := json.Marshal(ag)
	if err != nil || !proto.Equal(after, p) || string(agraw) != string(brokenGame) {
		t.Fatal("power-only query changed other assets or gameplay state", err)
	}
	var rolls int64
	if err = db.GetIns().Model(&db.P13GuildPrayerRoll{}).Where("player_id=?", id).Count(&rolls).Error; err != nil || rolls != 1 {
		t.Fatal("query created another paid prayer", rolls, err)
	}
}

func TestP5PowerCache75DirectTreasureWearPersistsExistingPower(t *testing.T) {
	for _, off := range []bool{false, true} {
		name := "wear"
		if off {
			name = "unwear"
		}
		t.Run(name, func(t *testing.T) {
			f, _, _ := p5PowerCacheFixture75(t)
			var treasure *pb.Treasure
			if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
				gain, err := f.s.GrantFixedReward(p, g, "trea_001,1")
				if err == nil {
					treasure = gain.GetTreas()[0]
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			on := &pb.CSFormationOn{Id: treasure.TreasureId, Guid: treasure.Guid, FormatPos: proto.Int32(1)}
			var replyPower int32
			if off {
				if _, err := f.s.p5TreasureFormationOn(p5PowerCtx75(f, on), f.id, on, 755102); err != nil {
					t.Fatal(err)
				}
				p5PowerEqual75(t, f, "native treasure wear prerequisite")
				req := &pb.CSFormationOff{Id: treasure.TreasureId, Guid: treasure.Guid, FormatPos: proto.Int32(1)}
				out, err := f.s.treasureFormationOff(p5PowerCtx75(f, req), f.id, req)
				if err != nil {
					t.Fatal(err)
				}
				replyPower = out[0].(*pb.SCFormationOff).GetFightpower()
			} else {
				out, err := f.s.treasureFormationOn(p5PowerCtx75(f, on), f.id, on)
				if err != nil {
					t.Fatal(err)
				}
				replyPower = out[0].(*pb.SCFormationOn).GetFightpower()
			}
			p, g := f.profile(t)
			actual, err := f.s.formationFightPower(p, g)
			if err != nil || actual != replyPower {
				t.Fatal("original direct callback power", actual, replyPower, err)
			}
			if p.GetFightPower() != actual {
				t.Fatalf("direct treasure %s returned current power=%d but persisted stale=%d", name, actual, p.GetFightPower())
			}
		})
	}
}

// Added after the direct compatibility consumer was fixed. This is a current
// regression assertion, with no claim of an earlier failing execution.
func TestP5PowerCache75DirectWarriorStrengthenPersistsAndReturnsPower(t *testing.T) {
	f, _, _ := p5PowerCacheFixture75(t)
	var units []*pb.Warrior
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		gain, err := f.s.GrantFixedReward(p, g, "hero_011,2")
		if err == nil {
			units = gain.GetWars()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(units) != 2 {
		t.Fatal("finite original partner and material prerequisite")
	}
	on := &pb.CSFormationOn{Id: units[0].Id, Guid: units[0].Guid, FormatPos: proto.Int32(1)}
	if _, err := f.s.p5FormationOnNative(p5PowerCtx75(f, on), f.id, on, 755103); err != nil {
		t.Fatal(err)
	}
	priorPower := p5PowerEqual75(t, f, "original level-one partner wear")
	before, _ := f.profile(t)
	req := &pb.CSWarriorStrengthen{Guid: units[0].Guid, Cost: []*pb.BaseResource{{Id: units[1].Id, Guid: units[1].Guid}}}
	out, err := f.s.warriorStrengthen(p5PowerCtx75(f, req), f.id, req)
	if err != nil {
		t.Fatal(err)
	}
	actual := p5PowerEqual75(t, f, "direct original partner growth")
	if actual <= priorPower {
		t.Fatal("original material growth did not increase current party power", priorPower, actual)
	}
	if len(out) != 2 || out[1].(*pb.SCPlayer).GetPlayer().GetFightPower() != actual {
		t.Fatal("direct growth callback order or current player power")
	}
	after, g := f.profile(t)
	grown, err := p5Warrior(g, units[0].GetGuid())
	if err != nil || grown.GetLevel() != 4 || grown.GetExp() != 151 || before.GetCoin()-after.GetCoin() != 1000 {
		t.Fatal("original finite material experience or coin cost changed", grown, err)
	}
	if _, err = p5Warrior(g, units[1].GetGuid()); err == nil {
		t.Fatal("direct growth retained consumed material")
	}
}
