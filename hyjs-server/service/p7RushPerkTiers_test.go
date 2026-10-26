package service

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// 两个新路径使用真实高星End支付6/9星，覆盖全部现六层及Reset后的来源隔离。
// 3星攻击链已有原机及业务证明，本测试不复跑该选择或扩充本地目录。
func TestP7PartyRushSixNineStarsActualPerksSixFloorsRewardShopColdReset(t *testing.T) {
	for _, star := range []int32{2, 3} {
		t.Run(fmt.Sprintf("star%d", star), func(t *testing.T) {
			f := p7PartyBusinessFixture(t)
			p7StarActivityOperations75(t, f)
			p7SectionPublish(t)
			p7PublishRushAddition(t)
			p7PublishContinuation(t)
			var baseline *pb.BattlePlayer
			var paid proto.Message
			var last *pb.CSRushFightEnd
			var accepted proto.Message
			var purchase *pb.CSRushAddition
			stamp := int64(20800 + star*100)
			for floor := int32(1); floor <= 6; floor++ {
				if floor == 4 {
					section, err := f.s.P7SectionReward(f.ctx, f.id, &pb.CSRushGetSectionReward{}, stamp+40, time.Now())
					if err != nil {
						t.Fatal(err)
					}
					gain := section[0].(*pb.SCRushGetSectionReward).GetGain().GetSimps()
					if len(gain) != 1 || gain[0].GetId() != "item_001" || gain[0].GetNum() != star {
						t.Fatal("actual six/nine-star section tier reward", gain)
					}
					options, err := f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushGetSectionAddition{}, 0, time.Now())
					if err != nil {
						t.Fatal(err)
					}
					option := options[0].(*pb.SCRushGetSectionAddition)
					if option.GetCostStar() != star*3 {
						t.Fatal("high-star source was not earned by three actual wins", option)
					}
					selected := option.GetStarResult6()
					if star == 3 {
						selected = option.GetStarResult9()
					}
					if _, err = f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushAddition{Select: proto.String(selected), Star: proto.Int32(3)}, stamp+41, time.Now()); err == nil {
						t.Fatal("higher-tier effect accepted a forged lower cost")
					}
					if star == 2 {
						if _, err = f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushAddition{Select: option.StarResult9, Star: proto.Int32(9)}, stamp+42, time.Now()); err == nil {
							t.Fatal("six real stars paid for a nine-star option")
						}
					}
					purchase = &pb.CSRushAddition{Select: proto.String(selected), Star: proto.Int32(star * 3)}
					messages, err := f.s.P7RushAddition(f.ctx, f.id, purchase, stamp+43, time.Now())
					if err != nil {
						t.Fatal(err)
					}
					paid = messages[0]
					var state db.P7RushState
					if err = db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil || state.TotalStars != star*3 {
						t.Fatal("perk cost changed earned star history", err)
					}
					left, err := f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushGetSectionAddition{}, 0, time.Now())
					if err != nil || left[0].(*pb.SCRushGetSectionAddition).GetCostStar() != 0 {
						t.Fatal("six/nine-star spendable balance", err)
					}
				}
				query, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now())
				if err != nil {
					p7PartyScopeFailure(t, f)
					t.Fatal("complete party high-star Ready", floor, err)
				}
				var task *pb.RushTaskInfo
				for _, candidate := range query[0].(*pb.SCRushCurrentInfo).TaskInfoList {
					if candidate.GetStar() == star {
						task = candidate
					}
				}
				if task == nil {
					t.Fatal("original Ready omitted published high-star option", floor, star)
				}
				start, err := f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: task.TaskId, Star: proto.Int32(star)}, stamp+int64(floor), false)
				if err != nil {
					t.Fatal("actual high-star Start", floor, err)
				}
				if floor == 3 {
					baseline = proto.Clone(start.BattlePlayer).(*pb.BattlePlayer)
				}
				if floor >= 4 {
					for i, actor := range start.BattlePlayer.Formation {
						original := baseline.Formation[i]
						if star == 2 {
							if actor.Prop.GetHp() != math.Floor(original.Prop.GetHp()*1.06) || actor.Inst.GetCurHp() != actor.Prop.GetHp() || actor.Prop.GetPdef() != original.Prop.GetPdef() {
								t.Fatal("paid six-star HP did not reach actual successor input", floor, i)
							}
						} else if actor.Prop.GetPdef() != math.Floor(original.Prop.GetPdef()*1.09) || actor.Prop.GetHp() != original.Prop.GetHp() {
							t.Fatal("paid nine-star PDef did not reach actual successor input", floor, i)
						}
					}
				}
				end := p7PartyActualEnd(t, f, start)
				if end.GetResult() != 1 {
					t.Fatal("real source lost; preserve actual outcome", floor, star)
				}
				messages, err := f.s.p7End(f.ctx, f.id, end)
				if err != nil || messages[0].(*pb.SCRushFightEnd).GetStar() != star {
					t.Fatal("actual high-star End not accepted at selected tier", floor, err)
				}
				last, accepted = end, messages[0]
			}
			var state db.P7RushState
			if err := db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil || state.Floor != 7 || state.TotalStars != 6*star || state.ActiveFight != "" || p1Count(t, &db.P7RushManualCalculation{}, "player_id=? AND status=?", f.id, constUtil.P7RushManualAccepted) != 6 {
				t.Fatal("six actual floors did not retain exact high-star history", err)
			}
			// 当前具名策略只冻结首段，不把第6层推断成新奖励或第二次购买机会。
			if flag, err := f.s.p7RushAdditionFlag(db.GetIns(), &state, time.Now()); err != nil || flag != 0 || p1Count(t, &db.P7RushAdditionOffer{}, "player_id=?", f.id) != 1 {
				t.Fatal("sixth floor fabricated another first-section offer", err)
			}
			for _, initialize := range []func(context.Context) error{f.s.InitializeShop, f.s.InitializeP4} {
				if err := initialize(f.ctx); err != nil {
					t.Fatal(err)
				}
			}
			p7PointShopOperations75(t, f)
			var activity db.PeriodInstance
			if err := db.GetIns().Where("kind=? AND enabled=?", constUtil.P7RushStarEvent, true).First(&activity).Error; err != nil {
				t.Fatal(err)
			}
			info, err := f.s.P1Activity(f.ctx, f.id, &pb.CSActivityInfo{ActivityId: proto.String(activity.ID)})
			if err != nil {
				t.Fatal(err)
			}
			activityRequest := &pb.CSActivityReward{ActivityId: proto.String(activity.ID), Index: proto.Int32(1), GetResources: info[0].(*pb.SCActivityInfo).ItemList[0].RewardList}
			if _, err = f.s.P1Activity(f.ctx, f.id, activityRequest); err != nil {
				t.Fatal(err)
			}
			shopRequest := &pb.CSShopBuy{ShopType: proto.Int32(5), ShopItemId: proto.String("p4Point_1"), BuyCount: proto.Int32(1)}
			if _, err = f.s.p4Commerce(f.ctx, f.id, shopRequest, stamp+50, time.Now()); err != nil {
				t.Fatal(err)
			}
			before, inventory := f.profile(t)
			if before.GetPoint() != 60 || simpleOwned(before, inventory, "item_021") != 1 || simpleOwned(before, inventory, "item_001") < int64(star) {
				t.Fatal("actual wins and star activity did not pay original point shop")
			}
			f.restart(t)
			repeated, err := f.s.p7End(f.ctx, f.id, last)
			if err != nil || !proto.Equal(repeated[0], accepted) {
				t.Fatal("cold sixth End receipt", err)
			}
			retry, err := f.s.P7RushAddition(f.ctx, f.id, purchase, stamp+43, time.Now())
			if err != nil || !proto.Equal(retry[0], paid) {
				t.Fatal("cold higher-tier payment receipt", err)
			}
			if _, err = f.s.p4Commerce(f.ctx, f.id, shopRequest, stamp+50, time.Now()); err != nil {
				t.Fatal(err)
			}
			after, current := f.profile(t)
			if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, inventory, current) {
				t.Fatal("cold high-tier actions minted rewards twice")
			}
			if _, err = f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushResetChallengeCount{}, stamp+60); err != nil {
				t.Fatal("real six-floor run original Reset", err)
			}
			if err = db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil || state.RunID != 2 || state.Floor != 1 || state.TotalStars != 0 || state.RecordStars < star*6 {
				t.Fatal("Reset changed high-star history or leaked current run stars", err)
			}
			if _, err = f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushGetSectionAddition{}, 0, time.Now()); err == nil {
				t.Fatal("prior high-star run authorized next-run purchase")
			}
			view, err := f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushAdditionView{}, 0, time.Now())
			if err != nil || !proto.Equal(view[0].(*pb.SCRushAdditionView).Prop, &pb.BaseProp{}) {
				t.Fatal("paid previous-run perk survived Reset", err)
			}
		})
	}
}

// 星档只改变本地已声明的获星元数据，不能暗改原NPC、驱动或胜利条件。
func TestP7RushPerkPublishedStarsShareCertifiedCombatInput(t *testing.T) {
	first, err := p7SectionCatalogue(true)
	if err != nil {
		t.Fatal(err)
	}
	next, err := p7RushContinuationCatalogue(4, 6, true)
	if err != nil {
		t.Fatal(err)
	}
	rows := append(first, next...)
	if len(rows) != 18 {
		t.Fatal("current six-floor three-star catalogue denominator")
	}
	for floor := int32(1); floor <= 6; floor++ {
		var base *db.P7RushStageRule
		seen := map[int32]bool{}
		for i := range rows {
			row := &rows[i]
			if row.Floor != floor {
				continue
			}
			if seen[row.Star] || row.Star < 1 || row.Star > 3 {
				t.Fatal("duplicate/invalid original star choice", floor)
			}
			seen[row.Star] = true
			if base == nil {
				base = row
			} else if !bytes.Equal(row.Enemies, base.Enemies) || row.Scene != base.Scene || row.MaxRound != base.MaxRound || row.WinKey != base.WinKey || row.WinValue != base.WinValue || row.Reward != base.Reward || row.TaskID != base.TaskID {
				t.Fatal("local star choice changed certified actual combat mechanism", floor)
			}
			monster := &pb.BattleMonster{}
			if proto.Unmarshal(row.Enemies, monster) != nil || len(monster.Formation) != 1 || monster.Formation[0].GetId() != "hero_047" || monster.Formation[0].GetSkillOrder() != "047_1,047_2,047_3" || row.WinKey != 0 || row.WinValue != 0 || row.MaxRound != 40 {
				t.Fatal("published local star tier exceeds original single-wave mechanism", floor)
			}
		}
		if len(seen) != 3 {
			t.Fatal("missing original Ready star choice", floor)
		}
	}
}
