package service

import (
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

// TestP11PlunderSweepActualIndependentChildrenEnergyStopAndCold uses real
// computed type3 children. PPM0 tests five actual outcomes without a guaranteed
// fragment; PPM1m tests the actual obtained-target stopping rule.
func TestP11PlunderSweepActualIndependentChildrenEnergyStopAndCold(t *testing.T) {
	for _, variant := range []struct {
		name      string
		energy    int32
		ppm       int64
		wantCount int32
	}{{"five-real-outcomes", 10, 0, 5}, {"actual-energy-stop", 3, 0, 1}, {"actual-fragment-stop", 10, 1000000, 0}} {
		t.Run(variant.name, func(t *testing.T) {
			f, opponent, _, now := p11PlunderFightSetup(t)
			if err := f.s.InitializeP11PlunderSweep(f.ctx); err != nil {
				t.Fatal(err)
			}
			for _, id := range []int64{f.id, opponent} {
				if err := updateProfile(f.ctx, id, func(p *pb.Player, g *model.GameState) error {
					p.Level = proto.Int32(20)
					if id == f.id {
						p.Level = proto.Int32(100)
						p.Energy = proto.Int32(variant.energy)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
			}
			p11PlunderFiniteItems(t, f, opponent, "frag_trea_001_1,1")
			p11PlunderPublish(t, now)
			if err := db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Updates(map[string]any{"drop_ppm": variant.ppm, "level_difference": 100, "sweep_enabled": true}).Error; err != nil {
				t.Fatal(err)
			}
			before, _ := p10LuckyProfile(t, f.id)
			request := &pb.CSSweepTreasureFight{FragId: proto.String("frag_trea_001_1"), SweepNumber: proto.Int32(5), AutoAddEnergy: proto.Int32(0)}
			result, err := f.s.p11PlunderSweepAt(f.ctx, f.id, request, 7101, now)
			if err != nil {
				t.Fatal(err)
			}
			actual := result[0].(*pb.SCSweepTreasureFight)
			n := int32(len(actual.RewardList))
			if n < 1 || n > 5 || variant.wantCount != 0 && n != variant.wantCount || len(actual.Cost) != 1 || actual.Cost[0].GetId() != "energy" || actual.Cost[0].GetNum() != 2*n {
				t.Fatal("actual accepted count/fee", actual)
			}
			var children []db.P11PlunderFightSettlement
			if err = db.GetIns().Where("player_id=?", f.id).Find(&children).Error; err != nil {
				t.Fatal(err)
			}
			if len(children) != int(n) {
				t.Fatal("child count", len(children), n)
			}
			coins, exp, transfers := int32(0), int32(0), int32(0)
			for _, child := range children {
				if child.Stamp >= 0 || child.EnergySpent != 2 || child.ScopeProof != constUtil.P6PlunderNativeProof {
					t.Fatal("independent actual child", child.Stamp)
				}
				receipt, e := p11ReadPlunderReceipt(db.GetIns(), &child)
				if e != nil {
					t.Fatal(e)
				}
				if receipt.GetMaxRoundNum() != 20 || len(receipt.PlayerList) != 2 {
					t.Fatal("original child shape")
				}
				coins += child.RewardCoin
				exp += child.RewardExp
				if child.FragmentTransferred {
					transfers++
				}
			}
			after, game := p10LuckyProfile(t, f.id)
			_, other := p10LuckyProfile(t, opponent)
			owned, _ := p11PlunderFragmentCount(game, "frag_trea_001_1")
			held, _ := p11PlunderFragmentCount(other, "frag_trea_001_1")
			if after.GetEnergy() != before.GetEnergy()-2*n || after.GetCoin() != before.GetCoin()+int64(coins) || after.GetExp() != before.GetExp()+int64(exp) || owned != transfers || held+owned != 1 {
				t.Fatal("actual cost/gain/fragment conservation")
			}
			if variant.ppm == 0 && transfers != 0 || variant.ppm == 1000000 && transfers != 1 {
				t.Fatal("actual local finite PPM", transfers)
			}
			if err = db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			conn, err := db.GetIns().DB()
			if err != nil {
				t.Fatal(err)
			}
			if err = conn.Close(); err != nil {
				t.Fatal(err)
			}
			f.open(t)
			for i := 0; i < 3; i++ {
				retry, e := f.s.p11PlunderSweepAt(f.ctx, f.id, request, 7101, now.Add(time.Second))
				if e != nil || !proto.Equal(retry[0], actual) {
					t.Fatal("cold packet must reuse complete actual batch", e)
				}
			}
			current, _ := p10LuckyProfile(t, f.id)
			if !proto.Equal(current, after) {
				t.Fatal("batch retry consumed resources")
			}
		})
	}
}

func TestP11PlunderSweepOriginalORGateAndAutoMedicineClosed(t *testing.T) {
	for _, vip := range []int32{0, 3} {
		t.Run(fmt.Sprintf("vip%d", vip), func(t *testing.T) {
			f, opponent, _, now := p11PlunderFightSetup(t)
			if err := f.s.InitializeP11PlunderSweep(f.ctx); err != nil {
				t.Fatal(err)
			}
			if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(20)
				p.Vip = proto.Int32(vip)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			p11PlunderFiniteItems(t, f, opponent, "frag_trea_001_1,1")
			p11PlunderPublish(t, now)
			if err := db.GetIns().Model(&db.P11PlunderFightPolicy{}).Where("id=1").Update("sweep_enabled", true).Error; err != nil {
				t.Fatal(err)
			}
			req := &pb.CSSweepTreasureFight{FragId: proto.String("frag_trea_001_1"), SweepNumber: proto.Int32(5), AutoAddEnergy: proto.Int32(1)}
			before, _ := p10LuckyProfile(t, f.id)
			if _, err := f.s.p11PlunderSweepAt(f.ctx, f.id, req, 7201, now); err == nil {
				t.Fatal("automatic medicine must remain closed")
			}
			req.AutoAddEnergy = proto.Int32(0)
			result, err := f.s.p11PlunderSweepAt(f.ctx, f.id, req, 7202, now)
			if vip == 0 {
				if err == nil {
					t.Fatal("neither original OR qualification met")
				}
				after, _ := p10LuckyProfile(t, f.id)
				if !proto.Equal(before, after) {
					t.Fatal("failed gate changed resources")
				}
			} else if err != nil || len(result[0].(*pb.SCSweepTreasureFight).RewardList) < 1 {
				t.Fatal("originalVIP3 suffices below30", err)
			}
		})
	}
}
