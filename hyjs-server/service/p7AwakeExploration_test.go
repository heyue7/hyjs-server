package service

import (
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"strings"
	"testing"
	"time"
)

func p7AwakeExploreFixture(t *testing.T) *p1Fixture {
	t.Helper()
	f := p7AwakeSetup(t)
	// 隔离领域前提只设原玩家等级边界，普通初始库存/钱包及所有探索事实保留新号默认。
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Level = proto.Int32(70); return nil }); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeExplorePolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", constUtil.P7AwakeExploreCopy).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	return f
}
func p7AwakeExploreCall(t *testing.T, f *p1Fixture, request proto.Message, now time.Time) proto.Message {
	t.Helper()
	reply, err := f.s.p7AwakeExplore(f.ctx, f.id, request, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(reply) != 1 {
		t.Fatal("one original callback required")
	}
	return reply[0]
}
func TestP7AwakeExploreRealBoxesColdOnceAndPolicyFreeze(t *testing.T) {
	f := p7AwakeExploreFixture(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	before, bg := f.profile(t)
	enter := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now).(*pb.SCEnterAwakeCopy)
	if len(enter.GetBoxes()) != 4 || len(enter.GetMonsters()) != 1 || enter.GetRefreshData().GetScore() != 0 {
		t.Fatal("explicit local instance denominator")
	}
	if !strings.Contains(enter.GetRefreshData().GetHpPercent(), "/") {
		t.Fatal("original HeroPanel requires current/max HP text")
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return p7AwakeRequireIdle(tx, f.id) }); err == nil {
		t.Fatal("shared combat ownership ignored active exploration")
	}
	repeated := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now).(*pb.SCEnterAwakeCopy)
	if !proto.Equal(enter, repeated) {
		t.Fatal("duplicate enter rerolled instance")
	}
	for _, box := range enter.Boxes {
		first := p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: box.Guid}, now).(*pb.SCAwakeCopyOpenBox)
		duplicate := p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: box.Guid}, now).(*pb.SCAwakeCopyOpenBox)
		if len(first.Rewards) == 0 || len(duplicate.Rewards) != 0 || first.GetRefreshData().GetScore() != duplicate.GetRefreshData().GetScore() {
			t.Fatal("box repeated score/loot")
		}
	}
	pending, pg := f.profile(t)
	if simpleOwned(before, bg, "item_320") != simpleOwned(pending, pg, "item_320") || pending.GetCoin() != before.GetCoin() {
		t.Fatal("box reward granted before settle")
	}
	// 关闭及改源只影响后续入场，已付一次日次数的实例继续按冻结帧恢复和结算。
	if err := db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeExplorePolicyID).Updates(map[string]any{"enabled": false, "source": "changed-after-paid"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", "501").Updates(map[string]any{"enabled": false, "content": []byte("changed-after-paid")}).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	resume := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now).(*pb.SCEnterAwakeCopy)
	if resume.GetRefreshData().GetScore() != 20 || len(resume.Rewards) < 4 {
		t.Fatal("cold paid state/loot missing")
	}
	if resume.GetRefreshData().GetHpPercent() != enter.GetRefreshData().GetHpPercent() {
		t.Fatal("cold resume replaced frozen camp0 HP")
	}
	for _, box := range resume.Boxes {
		if box.GetIsOpen() != 1 {
			t.Fatal("cold opened node reverted")
		}
	}
	catalogue, err := f.s.p7AwakeCatalogue(f.ctx, f.id, now)
	if err != nil {
		t.Fatal(err)
	}
	running := 0
	for _, chapter := range catalogue[0].(*pb.SCAwakeCopyData).Copys {
		for _, task := range chapter.SmallCopys {
			if task.GetIsRunning() == 1 && task.GetCopyId() == "501" {
				running++
			}
		}
	}
	if running != 1 {
		t.Fatal("original running copy projection")
	}
	settled := p7AwakeExploreCall(t, f, &pb.CSAwakeCopySettle{}, now).(*pb.SCAwakeCopySettle)
	if settled.GetScore() != 20 || len(settled.GetScoreRewards()) != 0 || settled.GetCoinRewards() != 0 || settled.GetExp() != 0 {
		t.Fatal("partial exploration fabricated completion rewards")
	}
	after, ag := f.profile(t)
	if simpleOwned(after, ag, "item_320")-simpleOwned(before, bg, "item_320") != 19 {
		t.Fatal("actual original simple-weight box stone total")
	}
	retry := p7AwakeExploreCall(t, f, &pb.CSAwakeCopySettle{}, now).(*pb.SCAwakeCopySettle)
	cold, cg := f.profile(t)
	if !proto.Equal(retry, settled) || simpleOwned(cold, cg, "item_320") != simpleOwned(after, ag, "item_320") {
		t.Fatal("settle receipt repeated inventory")
	}
	if err := db.GetIns().Transaction(func(tx *gorm.DB) error { return p7AwakeRequireIdle(tx, f.id) }); err != nil {
		t.Fatal("settled exploration retained combat ownership", err)
	}
	var daily db.P7AwakeState
	if err = db.GetIns().First(&daily, "player_id=?", f.id).Error; err != nil || daily.Used != 1 {
		t.Fatal("duplicate enter spent daily budget", err)
	}
	if _, err = f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now); err == nil {
		t.Fatal("closed policy accepted new run")
	}
}
func TestP7AwakeExploreExpiryForeignNodeDailyAndBattleGate(t *testing.T) {
	f := p7AwakeExploreFixture(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	before, bg := f.profile(t)
	enter := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now).(*pb.SCEnterAwakeCopy)
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id+123, &pb.CSAwakeCopyOpenBox{BoxGuid: enter.Boxes[0].Guid}, now); err == nil {
		t.Fatal("foreign owner accepted node")
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopyOpenBox{BoxGuid: proto.String("foreign-guid")}, now); err == nil {
		t.Fatal("invented node")
	}
	p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: enter.Boxes[0].Guid}, now)
	for _, req := range []proto.Message{&pb.CSAwakeCopyFightBegin{MonGuid: enter.Monsters[0].Guid}, &pb.CSAwakeCopyFightEnd{FightId: proto.String("fake"), IsWinClient: proto.Int32(1)}} {
		if _, handled, err := f.s.DispatchP7Awake(f.ctx, f.id, req, 0); !handled || err == nil {
			t.Fatal("unproved type16 or forged winner accepted")
		}
	}
	expired := now.Add(30 * time.Minute)
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopyOpenBox{BoxGuid: enter.Boxes[1].Guid}, expired); err == nil {
		t.Fatal("expired box minted reward")
	}
	result, err := f.s.p7AwakeCatalogue(f.ctx, f.id, expired)
	if err != nil || result[0].(*pb.SCAwakeCopyData).GetIsAutoSettled() != 1 {
		t.Fatal("original timeout auto-settle projection", err)
	}
	after, ag := f.profile(t)
	if simpleOwned(after, ag, "item_320")-simpleOwned(before, bg, "item_320") != 1 {
		t.Fatal("timeout only earned first box")
	}
	second := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, expired).(*pb.SCEnterAwakeCopy)
	if second.Boxes[0].GetGuid() == enter.Boxes[0].GetGuid() || second.Monsters[0].GetGuid() == enter.Monsters[0].GetGuid() {
		t.Fatal("new run reused old node GUID")
	}
	if _, err = f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopyOpenBox{BoxGuid: enter.Boxes[0].Guid}, expired); err == nil {
		t.Fatal("previous-run delayed open accepted in new instance")
	}
	p7AwakeExploreCall(t, f, &pb.CSAwakeCopySettle{}, expired)
	if _, err = f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, expired); err == nil {
		t.Fatal("third daily run")
	}
	if _, err = f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now.AddDate(0, 0, 3)); err == nil {
		t.Fatal("closed original weekday")
	}
}
func TestP7AwakeBoxProbabilitiesAreWeightedAndBounded(t *testing.T) {
	f := p7AwakeSetup(t)
	for _, draw := range []int64{0, 999} {
		result, err := f.s.p7AwakeRollBox("box4", func(limit int64) (int64, error) {
			if limit != 1000 {
				t.Fatal("weight denominator")
			}
			return draw, nil
		})
		if err != nil || !strings.Contains(result, "item_320,10") || strings.Contains(result, "item_312,1") != (draw == 999) {
			t.Fatal("probability expression interpreted as direct reward", draw, result, err)
		}
	}
	if _, err := f.s.p7AwakeRollBox("box17", func(int64) (int64, error) { t.Fatal("unsupported class grammar reached RNG"); return 0, nil }); err == nil {
		t.Fatal("unknown class-select box grammar accepted")
	}
	if _, err := f.s.p7AwakeRollBox("box1", func(int64) (int64, error) { return 1000, nil }); err == nil {
		t.Fatal("RNG out of range")
	}
	// 原UI显示全camp0当前/最大HP，不能返回比例、主角单体或固定满血文本。
	actor := &pb.BattlePlayer{Formation: []*pb.BattleHero{
		{Prop: &pb.BaseProp{Hp: proto.Float64(100)}, Inst: &pb.BattleHeroInst{CurHp: proto.Float64(10)}},
		{Prop: &pb.BaseProp{Hp: proto.Float64(300)}, Inst: &pb.BattleHeroInst{CurHp: proto.Float64(30)}},
	}}
	raw, err := proto.Marshal(actor)
	if err != nil {
		t.Fatal(err)
	}
	refresh, err := p7AwakeRefresh(&db.P7AwakeRun{Actor: raw}, &p7AwakeFrozen{}, &p7AwakeExplorationState{})
	if err != nil || refresh.GetHpPercent() != "40/400" {
		t.Fatal("original camp0 HeroPanel HP contract", refresh, err)
	}
	actor.Formation[1].Prop.Hp = proto.Float64(0)
	raw, err = proto.Marshal(actor)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p7AwakeRefresh(&db.P7AwakeRun{Actor: raw}, &p7AwakeFrozen{}, &p7AwakeExplorationState{}); err == nil {
		t.Fatal("invalid frozen maxHP became display-ready")
	}
}

func TestP7AwakeExploreRequestStampCannotSettleNewRun(t *testing.T) {
	f := p7AwakeExploreFixture(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	call := func(request proto.Message, stamp int64) proto.Message {
		t.Helper()
		result, err := f.s.p7AwakeExplore(f.ctx, f.id, request, now, stamp)
		if err != nil {
			t.Fatal(err)
		}
		return result[0]
	}
	first := call(&pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, 100).(*pb.SCEnterAwakeCopy)
	call(&pb.CSAwakeCopyOpenBox{BoxGuid: first.Boxes[0].Guid}, 101)
	oldReceipt := call(&pb.CSAwakeCopySettle{}, 102)
	second := call(&pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, 200).(*pb.SCEnterAwakeCopy)
	if first.Boxes[0].GetGuid() == second.Boxes[0].GetGuid() {
		t.Fatal("old node scope reused")
	}
	if !proto.Equal(call(&pb.CSAwakeCopySettle{}, 102), oldReceipt) {
		t.Fatal("old accepted stamp did not replay its own receipt")
	}
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, 150); err == nil {
		t.Fatal("unaccepted old request settled current run")
	}
	if count := p1Count(t, &db.P7AwakeExploration{}, "player_id=? AND status=?", f.id, constUtil.P7AwakeExploreActive); count != 1 {
		t.Fatal("old settle affected current active run")
	}
	current := call(&pb.CSAwakeCopySettle{}, 201).(*pb.SCAwakeCopySettle)
	if current.GetScore() != 0 || len(current.GetBoxRewards()) != 0 {
		t.Fatal("new run inherited previous box reward")
	}
	if !proto.Equal(call(&pb.CSAwakeCopySettle{}, 102), oldReceipt) {
		t.Fatal("new receipt overwrote earlier exact request stamp")
	}
}
