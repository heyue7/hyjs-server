package service

import (
	"context"
	"reflect"
	"strconv"
	"sync"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 用原首关普通随机掉落验证材料加倍，不固定随机数量或改造原掉落池。
func TestP4CopyBenefitsSweepActualDropsDoubleUniqueOnceAndCold(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(strconv.FormatBool(enabled), func(t *testing.T) {
			f, rule, now := p4CopySetup(t)
			for _, initialize := range []func(context.Context) error{f.s.InitCooking, f.s.InitializeDailyCopy, f.s.InitializeCopyDrops} {
				if err := initialize(f.ctx); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.GetIns().Model(&db.P4CopyBenefitRule{}).Where("id=?", rule.ID).Update("enabled", enabled).Error; err != nil {
				t.Fatal(err)
			}
			if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
				p.Level = proto.Int32(14)
				g.Copy.PassedCopyInfo = []*pb.CopySimpleInfo{{ChapterId: proto.String("1"), MissionId: proto.String("1"), Star: proto.Int32(5)}}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			before, beforeGame := f.profile(t)
			request := &pb.CSCopySweep{ChapterId: proto.String("1"), MissionId: proto.String("1")}
			var group sync.WaitGroup
			var replies [8][]proto.Message
			var failures [8]error
			for i := range replies {
				group.Add(1)
				go func(index int) {
					defer group.Done()
					replies[index], failures[index] = f.s.copySweepAt(f.ctx, f.id, request, 905001, now)
				}(i)
			}
			group.Wait()
			for i := range replies {
				if failures[i] != nil || len(replies[i]) == 0 || !proto.Equal(replies[0][0], replies[i][0]) {
					t.Fatal("sweep random drops rerolled on identical request", i, failures[i])
				}
			}
			gain := replies[0][0].(*pb.SCCopySweep).GetGain()
			counts := map[string]int32{}
			for _, item := range gain.GetSimps() {
				counts[item.GetId()] += item.GetNum()
			}
			after, game := f.profile(t)
			if len(game.Warriors) != len(beforeGame.Warriors)+len(gain.GetWars()) || game.Copy.GetPassedCopyInfo()[0].GetChallengeTimes() != 1 {
				t.Fatal("single real sweep inventory or quota mismatch")
			}
			seen := map[string]bool{}
			for _, warrior := range gain.GetWars() {
				if warrior.GetId() != "hero_exp_006" || warrior.GetGuid() == "" || seen[warrior.GetGuid()] {
					t.Fatal("original material identity or unique GUID", warrior)
				}
				seen[warrior.GetGuid()] = true
			}
			if enabled {
				var receipt db.P4CopyBenefitGrant
				if err := db.GetIns().First(&receipt, "player_id=? AND source_key=?", f.id, constUtil.P4CopySweepDropSourcePrefix+"905001:1/1").Error; err != nil {
					t.Fatal(err)
				}
				bonus := &pb.ResultObject{}
				if proto.Unmarshal(receipt.Bonus, bonus) != nil || len(bonus.GetWars()) < 1 || len(bonus.GetWars()) > 3 || len(gain.GetWars()) != 2*len(bonus.GetWars()) {
					t.Fatal("actual ordinary material draw not doubled", gain, bonus)
				}
				for _, warrior := range bonus.GetWars() {
					if !seen[warrior.GetGuid()] {
						t.Fatal("bonus GUID absent from original result")
					}
				}
				if counts["coin"] != 4380 || counts["exp"] != 310 || counts["item_124"]%2 != 0 || counts["item_124"] > 2 || after.GetCoin()-before.GetCoin() != 4380 || after.GetLevel() != 15 {
					t.Fatal("loot bonus multiplied already doubled currency or changed original draw", counts)
				}
			} else if len(gain.GetWars()) < 1 || len(gain.GetWars()) > 3 || counts["item_124"] > 1 || counts["coin"] != 2190 || counts["exp"] != 155 || p1Count(t, &db.P4CopyBenefitGrant{}, "player_id=?", f.id) != 0 {
				t.Fatal("closed welfare changed base random drops", gain)
			}
			f.restart(t)
			cold, err := f.s.copySweepAt(f.ctx, f.id, request, 905001, now)
			if err != nil || !proto.Equal(cold[0], replies[0][0]) {
				t.Fatal("cold random/bonus receipt changed", err)
			}
			still, stillGame := f.profile(t)
			if !proto.Equal(after, still) || !reflect.DeepEqual(game, stillGame) {
				t.Fatal("cold repeated original or bonus grants")
			}
		})
	}
}
