package service

import (
	"context"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// The producer fixture earns funds via original donations. Consumer tests add
// only real badge donations and original building/upgrade/member operations.
// Player wallets/levels/items remain the producer's explicit finite preparation.
func TestP5GuildTechOriginalEightAttributesRawMembershipAndCold(t *testing.T) {
	f, ids, now, _ := p13TechSetup(t)
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP5(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.ctx = context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s)
	f.ctx = context.WithValue(f.ctx, constUtil.P13UnionFightClockContextKey{}, func() time.Time { return now })
	bases := map[int64]*pb.BattlePlayer{}
	powers := map[int64]int32{}
	capture := func(id int64) (*pb.BattlePlayer, int32) {
		t.Helper()
		var battle *pb.BattlePlayer
		var power int32
		err := db.GetIns().WithContext(f.ctx).Transaction(func(tx *gorm.DB) error {
			var row db.Profile
			if err := tx.First(&row, id).Error; err != nil {
				return err
			}
			p, g, err := decodeProfileForUpdate(&row)
			if err != nil {
				return err
			}
			if err = f.s.RefreshP5GuildPrayerPower(f.ctx, tx, p, g, now); err != nil {
				return err
			}
			battle, err = f.s.battlePlayer(p, g)
			if err != nil {
				return err
			}
			power, err = f.s.formationFightPower(p, g)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return battle, power
	}
	for _, id := range ids {
		bases[id], powers[id] = capture(id)
	}
	for i := int64(0); i < 10; i++ {
		p13GuildCall(t, f, ids[1], &pb.CSUnionDonate{DonateType: proto.String("badge")}, 5000+i, now)
	}
	if _, err := f.s.p13GuildOpenBuilder(f.ctx, ids[0], &pb.CSUnionOpenBuilder{Type: proto.String("GuardTree")}, 5100, now); err != nil {
		t.Fatal(err)
	}
	p13TechPublish(t, now.Add(-time.Second))
	kinds := []string{"peratt", "perhp", "perpdef", "permdef", "cri", "hit", "anticri", "dodge"}
	for i, kind := range kinds {
		building := constUtil.P13GuildTechGuardTree
		if kind == "peratt" || kind == "cri" || kind == "hit" {
			building = constUtil.P13GuildTechHeroStatue
		}
		p13TechUpgrade(t, f, ids[0], kind, building, int64(5200+i), now)
	}
	for _, id := range ids {
		current, power := capture(id)
		expected := proto.Clone(bases[id]).(*pb.BattlePlayer)
		prop := expected.Formation[0].Prop
		for _, key := range []string{"hp", "att", "pdef", "mdef"} {
			field := prop.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(key))
			prop.ProtoReflect().Set(field, protoreflect.ValueOfFloat64(prop.ProtoReflect().Get(field).Float()*1.01))
		}
		expected.Formation[0].Inst.CurHp = prop.Hp
		for _, key := range []string{"cri", "hit", "antiCri", "dodge"} {
			field := prop.ProtoReflect().Descriptor().Fields().ByName(protoreflect.Name(key))
			prop.ProtoReflect().Set(field, protoreflect.ValueOfFloat64(prop.ProtoReflect().Get(field).Float()+1))
		}
		if !proto.Equal(expected, current) || power <= powers[id] {
			t.Fatal("original tech percent/constant integration order or units", id, expected, current, powers[id], power)
		}
		// One original coin donation persists the actual source-derived cache.
		// Current Player queries may return refreshed data without an inventory write.
		p13GuildCall(t, f, id, &pb.CSUnionDonate{DonateType: proto.String("coin")}, 5800, now)
		cached, _ := p10LuckyProfile(t, id)
		if cached.GetFightPower() != power {
			t.Fatal("current Player cache not actual tech power", id, cached.GetFightPower(), power)
		}
	}
	frozen, _ := capture(ids[1])
	wire, _ := proto.Marshal(frozen)
	assertCleared := func(id int64) {
		t.Helper()
		p, _ := p10LuckyProfile(t, id)
		if p.GetFightPower() != powers[id] {
			t.Fatal("actual membership mutation left stale raw FP before another query", id, p.GetFightPower(), powers[id])
		}
		b, power := capture(id)
		if !proto.Equal(b, bases[id]) || power != powers[id] {
			t.Fatal("departed member retains guild properties", id)
		}
	}
	p13GuildCall(t, f, ids[1], &pb.CSQuitUnion{}, 5300, now)
	assertCleared(ids[1])
	p13GuildCall(t, f, ids[0], &pb.CSRemoveUnionPlayer{PlayerId: proto.Int64(ids[2])}, 5301, now)
	assertCleared(ids[2])
	p13GuildCall(t, f, ids[0], &pb.CSDissolveUnion{}, 5302, now)
	for _, id := range []int64{ids[0], ids[3], ids[4]} {
		assertCleared(id)
	}
	sql, _ := db.GetIns().DB()
	sql.Close()
	f.open(t)
	for _, id := range ids {
		assertCleared(id)
	}
	after, _ := proto.Marshal(frozen)
	if string(after) != string(wire) {
		t.Fatal("membership change rewrote old frozen input")
	}
	// Current source must be JSON-ephemeral; Clone deep-copies both source PBs.
	original := &model.P5GuildPrayerProjection{PlayerID: ids[0], TechProperties: &pb.BaseProp{Hit: proto.Float64(1)}}
	cloned := original.Clone()
	cloned.TechProperties.Hit = proto.Float64(9)
	if original.TechProperties.GetHit() != 1 {
		t.Fatal("tech clone aliased source")
	}
}
