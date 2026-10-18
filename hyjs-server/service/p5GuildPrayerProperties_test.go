package service

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Each choice is an explicit one-option local publication. Initial guild-entry
// funds/level remain finite prerequisites; all consumed contribution is earned
// through the original fifteen donations, never inserted into either balance.
func TestP5GuildPrayerActualFourPercentCurrentColdExpiryAndFrozenInput(t *testing.T) {
	for _, kind := range []string{"peratt", "perhp", "perpdef", "permdef"} {
		t.Run(kind, func(t *testing.T) {
			f, ids, now := p13PrayerSetup(t)
			id := ids[0]
			if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
				t.Fatal(err)
			}
			at := now
			ctx := context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
			ctx = context.WithValue(ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time { return at })
			f.ctx = ctx
			capture := func(checkClone bool) (*pb.BattlePlayer, int32) {
				t.Helper()
				var battle *pb.BattlePlayer
				var power int32
				err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
					var row db.Profile
					if err := tx.First(&row, id).Error; err != nil {
						return err
					}
					p, g, err := decodeProfile(&row)
					if err != nil {
						return err
					}
					if err = f.s.RefreshP5GuildPrayerPower(ctx, tx, p, g, at); err != nil {
						return err
					}
					battle, err = f.s.battlePlayer(p, g)
					if err != nil {
						return err
					}
					power, err = f.s.formationFightPower(p, g)
					if err != nil {
						return err
					}
					panel, err := f.s.p10FormationProfile(tx, id)
					if err != nil {
						return err
					}
					if panel.GetFightPower() != power || !proto.Equal(panel.GetFormationItemInfoList()[0].GetWarInfo().GetBaseProp(), battle.GetFormation()[0].GetProp()) {
						t.Fatal("original public panel and BattleHero disagree")
					}
					if checkClone {
						encoded, e := json.Marshal(g)
						if e != nil {
							return e
						}
						if bytes.Contains(encoded, []byte("GuildPrayerProjection")) || bytes.Contains(encoded, []byte(g.GuildPrayerProjection.SourceHash)) && g.GuildPrayerProjection.SourceHash != "" {
							t.Fatal("transaction prayer leaked into saved GameState JSON")
						}
						before := g.GuildPrayerProjection
						// This single finite coin grant exercises the real reward JSON clone.
						if _, e = f.s.GrantFixedReward(p, g, "coin,1"); e != nil {
							return e
						}
						if g.GuildPrayerProjection == before || !proto.Equal(g.GuildPrayerProjection.Properties, before.Properties) || g.GuildPrayerProjection.SourceHash != before.SourceHash {
							t.Fatal("reward clone lost or aliased current transaction prayer")
						}
						changed, e := f.s.battlePlayer(p, g)
						if e != nil || !proto.Equal(changed, battle) {
							t.Fatal("reward clone changed current attributes", e)
						}
					}
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
				return battle, power
			}
			base, basePower := capture(false)
			pool, err := json.Marshal([]p13GuildPrayerChoice{{Type: kind, Value: 10, Weight: 1}})
			if err != nil {
				t.Fatal(err)
			}
			if err = db.GetIns().Model(&db.P13GuildPrayerPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "version": "local-p5-single-original-percent-v1", "source": constUtil.P13GuildPrayerSource, "pool_json": string(pool), "pool_hash": p6Digest(pool), "published_at": now.Add(-time.Second).UnixMilli()}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err = f.s.p13GuildPrayer(ctx, id, &pb.CSUnionPray{}, 101, at); err != nil {
				t.Fatal(err)
			}
			active, activePower := capture(true)
			expected := proto.Clone(base).(*pb.BattlePlayer)
			prop := expected.Formation[0].Prop
			switch kind {
			case "peratt":
				prop.Att = proto.Float64(prop.GetAtt() * 1.1)
			case "perhp":
				prop.Hp = proto.Float64(prop.GetHp() * 1.1)
				expected.Formation[0].Inst.CurHp = prop.Hp
			case "perpdef":
				prop.Pdef = proto.Float64(prop.GetPdef() * 1.1)
			case "permdef":
				prop.Mdef = proto.Float64(prop.GetMdef() * 1.1)
			}
			if !proto.Equal(expected, active) || activePower <= basePower {
				t.Fatal("original percent points were not applied once at combined stage", kind, expected, active, basePower, activePower)
			}
			frozen, err := proto.Marshal(active)
			if err != nil {
				t.Fatal(err)
			}
			// Closing the future pool leaves the fulfilled immutable roll unchanged.
			if err = db.GetIns().Model(&db.P13GuildPrayerPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			connection, _ := db.GetIns().DB()
			connection.Close()
			f.open(t)
			at = at.Add(time.Hour)
			cold, coldPower := capture(false)
			if !proto.Equal(cold, active) || coldPower != activePower {
				t.Fatal("cold/closed pool lost fulfilled attribute")
			}
			if kind == "peratt" {
				guild := p13GuildID(t, f, id, at)
				p13Join(t, f, id, ids[1], guild, 200, at)
				p13GuildCall(t, f, id, &pb.CSTransferChairman{PlayerId: proto.Int64(ids[1])}, 201, at)
				p13GuildCall(t, f, id, &pb.CSQuitUnion{}, 202, at)
				left, power := capture(false)
				if !proto.Equal(left, base) || power != basePower {
					t.Fatal("actual guild exit retained temporary prayer")
				}
			}
			at = now.Add(24 * time.Hour)
			expired, expiredPower := capture(false)
			if !proto.Equal(expired, base) || expiredPower != basePower {
				t.Fatal("exact expiry retained prayer in new attributes")
			}
			stillFrozen, err := proto.Marshal(active)
			if err != nil || !bytes.Equal(stillFrozen, frozen) {
				t.Fatal("current expiry rewrote the old frozen battle input", err)
			}
		})
	}
}

func TestP5GuildPrayerRejectsInvalidTransientSourceWithoutSQL(t *testing.T) {
	p := &pb.Player{PlayerId: proto.Int64(7)}
	g := &model.GameState{GuildPrayerProjection: &model.P5GuildPrayerProjection{PlayerID: 7, Properties: &pb.BaseProp{Peratt: proto.Float64(math.NaN())}, SourceHash: p6Digest([]byte("negative-only")), CapturedAt: 1, ExpiresAt: 2}}
	if err := AddP5GuildPrayerPercent(p, g, map[string]float64{}); err == nil {
		t.Fatal("nonfinite projection accepted")
	}
	g.GuildPrayerProjection.Properties = &pb.BaseProp{Att: proto.Float64(1)}
	if err := AddP5GuildPrayerPercent(p, g, map[string]float64{}); err == nil {
		t.Fatal("prayer constant was injected before percent")
	}
}
