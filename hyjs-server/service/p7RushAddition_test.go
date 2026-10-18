package service

import (
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

func p7PublishRushAddition(t *testing.T) {
	t.Helper()
	now := time.Now().UnixMilli()
	var original db.P7RushAdditionPolicy
	if err := db.GetIns().First(&original, constUtil.P7RushAdditionPolicyID).Error; err != nil {
		t.Fatal(err)
	}
	policy := original
	policy.Enabled = true
	policy.PublishedAt = now - 1000
	policy.StartAt = now - 1000
	policy.EndAt = now + 3600000
	if err := db.GetIns().Save(&policy).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.GetIns().Save(&original) })
}

func TestP7RushAdditionRealStarsFrozenPaymentConsumerColdReset(t *testing.T) {
	f := p7SectionFixture(t)
	p7SectionPublish(t)
	p7PublishRushAddition(t)
	for floor := int32(1); floor <= 3; floor++ {
		p7SectionPlayFloor(t, f, floor, 1, 18200+int64(floor))
		if floor < 3 {
			if _, err := f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushGetSectionAddition{}, 0, time.Now()); err == nil {
				t.Fatal("incomplete actual section offered addition")
			}
		}
	}
	info, err := f.s.P7RushQuery(f.ctx, f.id, &pb.CSGetRushInfo{}, time.Now())
	if err != nil || info[0].(*pb.SCGetRushInfo).GetIsAdditionNeedGet() != 1 {
		t.Fatal("original fourth-floor addition entry", err)
	}
	preview, err := f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushGetSectionAddition{}, 0, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	view := preview[0].(*pb.SCRushGetSectionAddition)
	if view.GetCostStar() != 3 || view.GetStarResult3() != "peratt,0.03" || view.GetStarResult6() != "perhp,0.06" || view.GetStarResult9() != "perpdef,0.09" || view.GetProp() == nil {
		t.Fatal("original option/currency projection", view)
	}
	for i, request := range []*pb.CSRushAddition{
		{Select: proto.String(view.GetStarResult6()), Star: proto.Int32(6)},
		{Select: proto.String("peratt,1"), Star: proto.Int32(3)},
		{Select: proto.String(view.GetStarResult3()), Star: proto.Int32(0)},
	} {
		if _, err = f.s.P7RushAddition(f.ctx, f.id, request, 18210+int64(i), time.Now()); err == nil {
			t.Fatal("unearned stars or forged option paid", request)
		}
	}
	before, gameBefore := f.profile(t)
	request := &pb.CSRushAddition{Select: proto.String(view.GetStarResult3()), Star: proto.Int32(3)}
	paid, err := f.s.P7RushAddition(f.ctx, f.id, request, 18220, time.Now())
	if err != nil || paid[0].(*pb.SCRushAddition).GetProp().GetPeratt() != 0.03 {
		t.Fatal("actual once purchase", err)
	}
	after, gameAfter := f.profile(t)
	if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, gameBefore, gameAfter) {
		t.Fatal("addition changed profile currency or base panels")
	}
	var state db.P7RushState
	if err = db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil || state.TotalStars != 3 || state.RecordStars < 3 {
		t.Fatal("spent stars changed earned total/rank", err)
	}
	preview, err = f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushGetSectionAddition{}, 0, time.Now())
	if err != nil || preview[0].(*pb.SCRushGetSectionAddition).GetCostStar() != 0 {
		t.Fatal("original spendable star balance", err)
	}
	if _, err = f.s.P7RushAddition(f.ctx, f.id, request, 18221, time.Now()); err == nil {
		t.Fatal("second payment accepted")
	}
	// 修改当前运营配置后，首层冻结的旧选项和已支付效果继续履约。
	if err = db.GetIns().Model(&db.P7RushAdditionPolicy{}).Where("id=?", constUtil.P7RushAdditionPolicyID).Updates(map[string]any{"enabled": false, "source": "changed-live-policy", "three_option": "perhp,1"}).Error; err != nil {
		t.Fatal(err)
	}
	var baseline, buffed *pb.BattlePlayer
	if err = updateProfileWithTx(f.ctx, f.id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var e error
		baseline, e = f.s.battlePlayer(p, g)
		if e != nil {
			return e
		}
		buffed, e = f.s.p7ApplyRushAddition(tx, &state, baseline)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if buffed.Formation[0].Prop.GetAtt() != math.Floor(baseline.Formation[0].Prop.GetAtt()*1.03) || buffed.Formation[0].Prop.GetPeratt() != baseline.Formation[0].Prop.GetPeratt() || buffed.Formation[0].Prop.GetHp() != baseline.Formation[0].Prop.GetHp() {
		t.Fatal("real next-battle snapshot consumer differs from named rule")
	}
	f.restart(t)
	retry, err := f.s.P7RushAddition(f.ctx, f.id, request, 18220, time.Now())
	if err != nil || !proto.Equal(paid[0], retry[0]) {
		t.Fatal("cold receipt replay", err)
	}
	viewed, err := f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushAdditionView{}, 0, time.Now())
	if err != nil || viewed[0].(*pb.SCRushAdditionView).GetProp().GetPeratt() != 0.03 {
		t.Fatal("cold frozen addition view", err)
	}
	if _, err = f.s.P7RushMutation(f.ctx, f.id, &pb.CSRushResetChallengeCount{}, 18230); err != nil {
		t.Fatal("original real successful-run reset", err)
	}
	viewed, err = f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushAdditionView{}, 0, time.Now())
	if err != nil || viewed[0].(*pb.SCRushAdditionView).GetProp().GetPeratt() != 0 {
		t.Fatal("previous-run addition leaked after reset", err)
	}
	if err = db.GetIns().First(&state, "player_id=?", f.id).Error; err != nil || state.RunID != 2 || state.TotalStars != 0 || state.RecordStars < 3 {
		t.Fatal("real reset did not isolate run/record", err)
	}
	if allowed, e := f.s.p7CompletedLocalSectionResetAllowed(db.GetIns().WithContext(f.ctx), &state); e != nil || allowed {
		t.Fatal("earlier accepted run granted next-run reset", e)
	}
	if err = db.GetIns().Transaction(func(tx *gorm.DB) error {
		got, e := f.s.p7ApplyRushAddition(tx, &state, baseline)
		if e == nil && !proto.Equal(got, baseline) {
			t.Error("old purchased effect crossed run")
		}
		return e
	}); err != nil {
		t.Fatal(err)
	}
}

func TestP7RushAdditionFoldPreservesSourceAndHealthFraction(t *testing.T) {
	player := &pb.BattlePlayer{Formation: []*pb.BattleHero{{Prop: &pb.BaseProp{Att: proto.Float64(123), Hp: proto.Float64(101), Pdef: proto.Float64(57), Mdef: proto.Float64(29)}, Inst: &pb.BattleHeroInst{CurHp: proto.Float64(50)}}}}
	before := proto.Clone(player)
	for option, want := range map[string]float64{"peratt,0.03": 126, "perhp,0.06": 107, "perpdef,0.09": 62, "permdef,0.1": 31} {
		folded, err := p7FoldRushAddition(player, option)
		if err != nil {
			t.Fatal(err)
		}
		prop := folded.Formation[0].Prop
		var value float64
		switch option {
		case "peratt,0.03":
			value = prop.GetAtt()
		case "perhp,0.06":
			value = prop.GetHp()
			if folded.Formation[0].Inst.GetCurHp() != 53 {
				t.Fatal("HP addition healed damaged hero")
			}
		case "perpdef,0.09":
			value = prop.GetPdef()
		case "permdef,0.1":
			value = prop.GetMdef()
		}
		if value != want || !proto.Equal(player, before) || prop.GetPeratt() != 0 || prop.GetPerhp() != 0 || prop.GetPerpdef() != 0 || prop.GetPermdef() != 0 {
			t.Fatal("local fold/source/engine metadata", option, value, want)
		}
	}
	for _, invalid := range []string{"peratt,NaN", "perhp,Inf", "perhp,-0.1", "att,0.1", "peratt,10", "perhp,0.1,coin", "peratt,0"} {
		if _, err := p7FoldRushAddition(player, invalid); err == nil {
			t.Fatal("invalid local option", invalid)
		}
	}
}

func TestP7RushAdditionResetRejectsUnprovenStarsAndOtherRun(t *testing.T) {
	f := p7SectionFixture(t)
	state := &db.P7RushState{PlayerID: f.id, RunID: 1, Floor: 4, TotalStars: 9, RecordStars: 9}
	allowed, err := f.s.p7CompletedLocalSectionResetAllowed(db.GetIns().WithContext(f.ctx), state)
	if err != nil || allowed {
		t.Fatal("unproven history granted successful reset", err)
	}
	if _, err = f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushGetSectionAddition{}, 0, time.Now()); err == nil {
		t.Fatal("default-off policy created offer")
	}
	view, err := f.s.P7RushAddition(f.ctx, f.id, &pb.CSRushAdditionView{}, 0, time.Now())
	if err != nil || view[0].(*pb.SCRushAdditionView).GetProp() == nil || view[0].(*pb.SCRushAdditionView).GetProp().GetPeratt() != 0 {
		t.Fatal("empty original addition view", err)
	}
}
