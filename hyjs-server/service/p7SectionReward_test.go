package service

import (
	"encoding/json"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"testing"
	"time"
)

func p7SectionFixture(t *testing.T) *p1Fixture {
	t.Helper()
	f := p7Setup(t)
	for _, m := range []any{&db.P7RushPolicy{}, &db.P7RushScopedPolicy{}} {
		if err := db.GetIns().Model(m).Where("id=1").Update("enabled", true).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P7RushStageRule{}).Where("source=?", constUtil.P7SectionStageSource).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.formationList(f.ctx, f.id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Model(&db.P7SectionRewardPolicy{}).Where("id=1").Update("enabled", false) })
	return f
}
func p7SectionPublish(t *testing.T) {
	t.Helper()
	now := time.Now().UnixMilli()
	if err := db.GetIns().Model(&db.P7SectionRewardPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "published_at": now - 1000, "start_at": now - 1000, "end_at": now + 3600000}).Error; err != nil {
		t.Fatal(err)
	}
}
func p7SectionPlayFloor(t *testing.T, f *p1Fixture, floor, star int32, stamp int64) *pb.CSRushFightEnd {
	t.Helper()
	start, err := f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String(fmt.Sprintf("local-section-%d", floor)), Star: proto.Int32(star)}, stamp, false)
	if err != nil {
		t.Fatal(err)
	}
	var calculation db.P7RushScopedCalculation
	if err = db.GetIns().First(&calculation, "fight_id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if err = json.Unmarshal(calculation.Terminal, &terminal); err != nil {
		t.Fatal(err)
	}
	result := int32(0)
	if terminal.Outcome.Winner == 0 {
		result = 1
	}
	end := &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(result), OptRecord: terminal.OriginalOperates}
	if _, err = f.s.p7End(f.ctx, f.id, end); err != nil {
		t.Fatal(err)
	}
	if result != 1 {
		t.Fatalf("actual source lost: winner=%d; preserve source/outcome", terminal.Outcome.Winner)
	}
	return end
}

func TestP7SectionRewardThreeRealFloorsOriginalListsOnceUseCold(t *testing.T) {
	// Three finite cases exercise all three original displayed star thresholds.
	for _, star := range []int32{1, 2, 3} {
		t.Run(fmt.Sprint(star), func(t *testing.T) {
			f := p7SectionFixture(t)
			p7SectionPublish(t)
			before, _ := f.profile(t)
			for floor := int32(1); floor <= 3; floor++ {
				tasks, e := f.s.P7RushQuery(f.ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now())
				if e != nil {
					t.Fatal(e)
				}
				for _, task := range tasks[0].(*pb.SCRushCurrentInfo).TaskInfoList {
					if task.GetPoint() != 10 || task.GetCoin() != 0 {
						t.Fatal("actual Ready reward preview", task)
					}
				}
				p7SectionPlayFloor(t, f, floor, star, 17800+int64(floor))
				preview, err := f.s.P7SectionReward(f.ctx, f.id, &pb.CSRushRewardPreview{}, 0, time.Now())
				if err != nil {
					t.Fatal(err)
				}
				view := preview[0].(*pb.SCRushRewardPreview)
				if view.GetSectionStar() != floor*star || view.GetSectionFrom() != 1 || view.GetSectionTo() != 3 || len(view.ThreeStarReward) != 1 || view.NineStarReward[0].GetNum() != 3 {
					t.Fatal(view)
				}
				if floor < 3 {
					if _, err = f.s.P7SectionReward(f.ctx, f.id, &pb.CSRushGetSectionReward{}, 17810+int64(floor), time.Now()); err == nil {
						t.Fatal("partial section paid")
					}
				}
			}
			var accepted db.P7RushFight
			if err := db.GetIns().Where("player_id=? AND task_id=?", f.id, "local-section-2").First(&accepted).Error; err != nil {
				t.Fatal(err)
			}
			original := append([]byte(nil), accepted.EndRequest...)
			if err := db.GetIns().Model(&accepted).Update("end_request", []byte{1}).Error; err != nil {
				t.Fatal(err)
			}
			heldBefore, _ := f.profile(t)
			if _, err := f.s.P7SectionReward(f.ctx, f.id, &pb.CSRushGetSectionReward{}, 17816, time.Now()); err == nil {
				t.Fatal("corrupted accepted witness paid")
			}
			heldAfter, _ := f.profile(t)
			if !proto.Equal(heldBefore, heldAfter) {
				t.Fatal("rejected witness changed wallet")
			}
			if err := db.GetIns().Model(&accepted).Update("end_request", original).Error; err != nil {
				t.Fatal(err)
			}
			info, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushInfo{}, time.Now())
			if err != nil || info[0].(*pb.SCGetRushInfo).GetIsRewardNeedGet() != 1 || info[0].(*pb.SCGetRushInfo).GetCurFloor() != 4 {
				t.Fatal("earned-only last-floor query", info, err)
			}
			if _, err = f.s.P7RushQuery(f.ctx, f.id, &pb.CSRushCurrentInfo{}, time.Now()); err == nil {
				t.Fatal("unpublished fourth-floor tasks")
			}
			if _, err = f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local-section-4"), Star: proto.Int32(1)}, 17815, false); err == nil {
				t.Fatal("fourth floor opened")
			}
			if err = db.GetIns().Model(&db.P7SectionRewardPolicy{}).Where("id=1").Updates(map[string]any{"enabled": false, "nine_reward": "coin,99999"}).Error; err != nil {
				t.Fatal(err)
			}
			reward, err := f.s.P7SectionReward(f.ctx, f.id, &pb.CSRushGetSectionReward{}, 17820, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			after, g := f.profile(t)
			if p5LifeItemCount(g, "item_001") != star || after.GetPoint() != before.GetPoint()+30 {
				t.Fatal("actual earned source", g.Items, after.GetPoint())
			}
			if _, err = f.s.P7SectionReward(f.ctx, f.id, &pb.CSRushGetSectionReward{}, 17821, time.Now()); err == nil {
				t.Fatal("second claim")
			}
			if _, err = f.s.itemUse(f.ctx, f.id, &pb.CSItemUse{CostItem: &pb.BaseResource{Id: proto.String("item_001"), Num: proto.Int32(star)}}, 17822); err != nil {
				t.Fatal(err)
			}
			held, game := f.profile(t)
			if held.GetEnergy() != after.GetEnergy()+star*10 || p5LifeItemCount(game, "item_001") != 0 {
				t.Fatal("earned item use")
			}
			f.restart(t)
			retry, err := f.s.P7SectionReward(f.ctx, f.id, &pb.CSRushGetSectionReward{}, 17820, time.Now())
			if err != nil || !proto.Equal(reward[0], retry[0]) {
				t.Fatal("cold once", err)
			}
			again, current := f.profile(t)
			if !proto.Equal(held, again) || !p6CopySeriesJSONEqual(t, game, current) {
				t.Fatal("cold duplicated inventory")
			}
			info, err = f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushInfo{}, time.Now())
			if err != nil || info[0].(*pb.SCGetRushInfo).GetIsRewardNeedGet() != 0 {
				t.Fatal("postclaim query", err)
			}
			if p1Count(t, &db.P7SectionRewardOffer{}, "player_id=?", f.id) != 1 || p1Count(t, &db.P7RushReceipt{}, "player_id=? AND domain=?", f.id, constUtil.P7SectionRewardReceipt) != 1 {
				t.Fatal("duplicate offer/receipt")
			}
			authority, err := p7AuthorityAvailable(db.GetIns())
			if err != nil || authority {
				t.Fatal("global authority changed", err)
			}
		})
	}
}

func TestP7SectionRewardLatePublicationAndWrongRunDoNotCreateStars(t *testing.T) {
	f := p7SectionFixture(t)
	if _, err := f.s.P7SectionReward(f.ctx, f.id, &pb.CSRushRewardPreview{}, 0, time.Now()); err == nil {
		t.Fatal("default off preview")
	}
	p7SectionPlayFloor(t, f, 1, 1, 17830)
	p7SectionPublish(t)
	if _, err := f.s.P7SectionReward(f.ctx, f.id, &pb.CSRushGetSectionReward{}, 17831, time.Now()); err == nil {
		t.Fatal("late backfill")
	}
	if p1Count(t, &db.P7SectionRewardOffer{}, "player_id=?", f.id) != 0 {
		t.Fatal("late offer")
	}
}
