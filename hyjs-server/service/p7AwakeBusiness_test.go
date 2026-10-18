package service

import (
	"context"
	"encoding/json"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func p7AwakeBusinessFixture(t *testing.T) *p1Fixture {
	t.Helper()
	f := p7PartyBusinessFixture(t)
	if err := f.s.LoadP7AwakeTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	// 原P5保存会同步开服与日常任务；使用正式初始化补齐其表/原规则，
	// 不为消费者补钱包、普通库存或已完成活动进度。
	for _, init := range []func(context.Context) error{f.s.InitializeP7Awake, f.s.InitializeP5, f.s.InitializeOpenServerActivity, f.s.InitializeDailyTasks} {
		if err := init(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.GetIns().Model(&db.P7AwakePolicy{}).Where("id IN ?", []int64{constUtil.P7AwakeExplorePolicyID, constUtil.P7AwakeBattlePolicyID}).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7AwakeStageRule{}).Where("copy_id=?", constUtil.P7AwakeExploreCopy).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	return f
}
func p7AwakeBusinessStartEnd(t *testing.T, f *p1Fixture, monGUID string, now time.Time) (*pb.SCAwakeCopyFightBegin, *pb.CSAwakeCopyFightEnd) {
	t.Helper()
	replies, err := f.s.p7AwakeFightBegin(f.ctx, f.id, &pb.CSAwakeCopyFightBegin{MonGuid: proto.String(monGUID)}, now)
	if err != nil {
		t.Fatal(err)
	}
	start := replies[0].(*pb.SCAwakeCopyFightBegin)
	input, err := f.s.FreezeBattleInput(model.FrozenBattleInput{Attacker: start.BattlePlayer, Monster: start.BattleMonser, Seed: start.GetRandomSeed(), MaxRound: start.GetMaxRoundNum(), BattleType: 16, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = P7Awake16ProofForInput(input); err != nil {
		t.Fatal(err)
	}
	// 测试客户端从真实Start独立产出原操作；不把预设胜者或提交记录作为服务端driver。
	terminal, err := f.s.computeFrozenBattleFinalHP(f.ctx, input, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	actual := int32(0)
	if terminal.Outcome.Winner == 0 {
		actual = 1
	}
	end := &pb.CSAwakeCopyFightEnd{FightId: start.FightId, IsWinClient: proto.Int32(actual), OptRecord: terminal.OriginalOperates}
	return start, end
}
func TestP7AwakeBusinessRealEndHundredScoreGrowthAndColdOnce(t *testing.T) {
	f := p7AwakeBusinessFixture(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	before, bg := f.profile(t)
	if before.GetCoin() != 0 {
		t.Fatal("fresh ordinary wallet must not be imported")
	}
	catalogue, err := f.s.p7AwakeCatalogue(f.ctx, f.id, now)
	if err != nil {
		t.Fatal(err)
	}
	foundPreview := false
	for _, chapter := range catalogue[0].(*pb.SCAwakeCopyData).Copys {
		for _, task := range chapter.SmallCopys {
			if task.GetCopyId() == "501" {
				foundPreview = strings.Contains(task.GetDropReward(), "item_320") && strings.Contains(task.GetDropReward(), "hero_exp_002") && !strings.Contains(task.GetDropReward(), ",")
			}
		}
	}
	if !foundPreview {
		t.Fatal("original Awake potential-drop ID list missing or quantity-formatted")
	}
	first := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now).(*pb.SCEnterAwakeCopy)
	for _, box := range first.Boxes {
		p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: box.Guid}, now)
	}
	start, end := p7AwakeBusinessStartEnd(t, f, first.Monsters[0].GetGuid(), now)
	if end.GetIsWinClient() != 1 {
		t.Fatal("finite original source unexpectedly lost; preserve actual result")
	}
	beforeEnd, beg := f.profile(t)
	for _, variant := range []string{"false-result", "terminal-hp", "missing-op", "duplicated-op", "transform"} {
		bad := proto.Clone(end).(*pb.CSAwakeCopyFightEnd)
		switch variant {
		case "false-result":
			bad.IsWinClient = proto.Int32(0)
		case "terminal-hp":
			bad.OptRecord.AttackerTotalLeftHp = proto.Float64(bad.OptRecord.GetAttackerTotalLeftHp() + 1)
		case "missing-op":
			if len(bad.OptRecord.SkillOpts) == 0 {
				t.Fatal("actual original automatic record empty")
			}
			bad.OptRecord.SkillOpts = bad.OptRecord.SkillOpts[1:]
		case "duplicated-op":
			bad.OptRecord.SkillOpts = append(bad.OptRecord.SkillOpts, proto.Clone(bad.OptRecord.SkillOpts[0]).(*pb.BattleSkillOperate))
		case "transform":
			bad.OptRecord.AttackerTransOpt = &pb.BattleTransOperate{RoundNum: proto.Int32(1)}
		}
		if _, err := f.s.p7AwakeFightEnd(f.ctx, f.id, bad, now); err == nil {
			t.Fatal("forged original End accepted", variant)
		}
		after, ag := f.profile(t)
		if !proto.Equal(after, beforeEnd) || !p6CopySeriesJSONEqual(t, ag, beg) {
			t.Fatal("rejected End changed profile", variant)
		}
	}
	if err := db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeBattlePolicyID).Updates(map[string]any{"enabled": false, "source": "closed-after-paid"}).Error; err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	resumed, err := f.s.p7AwakeFightBegin(f.ctx, f.id, &pb.CSAwakeCopyFightBegin{MonGuid: first.Monsters[0].Guid}, now)
	if err != nil || !proto.Equal(resumed[0], start) {
		t.Fatal("cold pending Start or source drift", err)
	}
	replies := make([]proto.Message, 4)
	errors := make([]error, 4)
	var group sync.WaitGroup
	for i := range replies {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			r, e := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now)
			errors[index] = e
			if len(r) > 0 {
				replies[index] = r[0]
			}
		}(i)
	}
	group.Wait()
	for i := range replies {
		if errors[i] != nil || !proto.Equal(replies[0], replies[i]) {
			t.Fatal("concurrent real End once", i, errors[i])
		}
	}
	accepted := replies[0].(*pb.SCAwakeCopyFightEnd)
	if accepted.GetIsWinServer() != 1 || accepted.GetRefreshData().GetScore() != 100 || accepted.GetFightMonster().GetHpPercent() != 0 {
		t.Fatal("actual monster victory did not yield true100", accepted)
	}
	afterEnd, aeg := f.profile(t)
	if afterEnd.GetCoin() != before.GetCoin() || simpleOwned(afterEnd, aeg, "item_320") != simpleOwned(before, bg, "item_320") {
		t.Fatal("battle granted pending materials before original Settle")
	}
	if p1Count(t, &db.P6BattleRecord{}, "player_id=? AND source=? AND authoritative=?", f.id, constUtil.P7AwakeBattleReplaySource, true) != 1 {
		t.Fatal("real End did not commit exactly one certified P6 record")
	}
	restored := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now).(*pb.SCEnterAwakeCopy)
	if len(restored.Monsters) != 0 || restored.GetRefreshData().GetScore() != 100 || restored.GetRefreshData().GetHpPercent() != accepted.GetRefreshData().GetHpPercent() {
		t.Fatal("cold actual monster/HP state drift")
	}
	settleReplies := make([]proto.Message, 4)
	for i := range settleReplies {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			r, e := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, 390101)
			errors[index] = e
			if len(r) > 0 {
				settleReplies[index] = r[0]
			}
		}(i)
	}
	group.Wait()
	for i := range settleReplies {
		if errors[i] != nil || !proto.Equal(settleReplies[0], settleReplies[i]) {
			t.Fatal("concurrent true100 daily reward once", i, errors[i])
		}
	}
	settled := settleReplies[0].(*pb.SCAwakeCopySettle)
	if settled.GetScore() != 100 || len(settled.GetScoreRewards()) != 2 {
		t.Fatal("true100 daily score resources missing", settled)
	}
	p, g := f.profile(t)
	if simpleOwned(p, g, "item_320")-simpleOwned(before, bg, "item_320") != 19 || p.GetCoin() < 5100 || p.GetCoin() > 5200 {
		t.Fatal("actual box/monster/daily score award source", p.GetCoin())
	}
	var target, material *pb.Warrior
	for _, hero := range g.Warriors {
		if hero.GetId() == "hero_003" {
			target = hero
		}
		if hero.GetId() == "hero_exp_002" {
			material = hero
		}
	}
	if target == nil || material == nil {
		t.Fatal("earned normal P5 material instance missing")
	}
	req := &pb.CSWarriorStrengthen{Guid: target.Guid, Cost: []*pb.BaseResource{{Id: material.Id, Guid: material.Guid}}}
	ctx := context.WithValue(context.WithValue(f.ctx, constUtil.GameServiceContextKey, f.s), constUtil.GameRequestContextKey, req)
	growth, handled, err := f.s.DispatchP5Next(ctx, f.id, req, 390102)
	if err != nil || !handled {
		t.Fatal("earned material original P5 consumer", err)
	}
	grown, gg := f.profile(t)
	if grown.GetCoin() != p.GetCoin()-5000 {
		t.Fatal("P5 did not consume exact original experience/coin cost")
	}
	for _, hero := range gg.Warriors {
		if hero.GetGuid() == material.GetGuid() {
			t.Fatal("P5 material GUID survived consumption")
		}
	}
	grownTarget, grownErr := p5Warrior(gg, target.GetGuid())
	if grownErr != nil || grownTarget == nil || grownTarget.GetLevel() <= target.GetLevel() {
		t.Fatal("P5 actual target did not grow")
	}
	// 冷恢复后复用原End、原Settle和原P5账本，真实库存与钱包只发生一次变化。
	f.restart(t)
	coldEnd, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now)
	if err != nil || !proto.Equal(coldEnd[0], accepted) {
		t.Fatal("cold actual End replay", err)
	}
	coldSettle, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, 390101)
	if err != nil || !proto.Equal(coldSettle[0], settled) {
		t.Fatal("cold score receipt replay", err)
	}
	coldGrowth, handled, err := f.s.DispatchP5Next(ctx, f.id, req, 390102)
	if err != nil || !handled || !proto.Equal(coldGrowth[0], growth[0]) {
		t.Fatal("cold P5 consumed material twice", err)
	}
	cp, cg := f.profile(t)
	if !proto.Equal(cp, grown) || !p6CopySeriesJSONEqual(t, cg, gg) {
		t.Fatal("cold receipts changed inventory")
	}
	// 正常成长后的同一真实队伍还能开启本日第二轮；旧End/Settle不影响新轮。
	if err = db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeBattlePolicyID).Updates(map[string]any{"enabled": true, "source": constUtil.P7AwakeBattleSource}).Error; err != nil {
		t.Fatal(err)
	}
	secondMessages, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now.Add(time.Minute), 390200)
	if err != nil {
		t.Fatal("normal grown party cannot replay exploration", err)
	}
	second := secondMessages[0].(*pb.SCEnterAwakeCopy)
	if second.Boxes[0].GetGuid() == first.Boxes[0].GetGuid() {
		t.Fatal("next real run reused node scope")
	}
	oldEnd, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now)
	if err != nil || !proto.Equal(oldEnd[0], accepted) {
		t.Fatal("old battle affected current run", err)
	}
	oldSettle, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now, 390101)
	if err != nil || !proto.Equal(oldSettle[0], settled) {
		t.Fatal("old settlement affected current run", err)
	}
	for _, box := range second.Boxes {
		p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: box.Guid}, now.Add(time.Minute))
	}
	_, secondEnd := p7AwakeBusinessStartEnd(t, f, second.Monsters[0].GetGuid(), now.Add(time.Minute))
	if secondEnd.GetIsWinClient() != 1 {
		t.Fatal("actual grown party second result must be retained")
	}
	if _, err = f.s.p7AwakeFightEnd(f.ctx, f.id, secondEnd, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	secondSettle, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSAwakeCopySettle{}, now.Add(time.Minute), 390201)
	if err != nil {
		t.Fatal(err)
	}
	if secondSettle[0].(*pb.SCAwakeCopySettle).GetScore() != 100 || len(secondSettle[0].(*pb.SCAwakeCopySettle).GetScoreRewards()) != 0 {
		t.Fatal("same-day repeat minted daily first100 award")
	}
	if p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P7AwakeCompleteDailyEvent) != 1 || p1Count(t, &db.EventFact{}, "player_id=? AND kind=?", f.id, constUtil.P7AwakeCompleteEvent) != 2 {
		t.Fatal("actual completion vs daily reward ledger count")
	}
	final, fg := f.profile(t)
	if simpleOwned(final, fg, "item_320")-simpleOwned(before, bg, "item_320") != 38 {
		t.Fatal("two true local runs did not produce exact19+19 box stones")
	}
	if delta := final.GetCoin() - grown.GetCoin(); delta < 100 || delta > 200 {
		t.Fatal("same-day second true100 repeated score coin", delta)
	}
	if p1Count(t, &db.P6BattleRecord{}, "player_id=? AND source=? AND authoritative=?", f.id, constUtil.P7AwakeBattleReplaySource, true) != 2 {
		t.Fatal("two real runs require two certified actual Ends")
	}
	if _, err = f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now.Add(time.Minute), 390300); err == nil {
		t.Fatal("third real run ignored original daily2 budget")
	}
}

// 显式真实typed16 HTTP Start的无DB客户端计算器；输入不能携带结果或原End。
func TestP7AwakeExportOriginalHTTPStart(t *testing.T) {
	file := os.Getenv("HYJS_P7_AWAKE_HTTP_START")
	if file == "" {
		t.Skip("explicit original HTTP Awake Start not requested")
	}
	output := os.Getenv("HYJS_P7_AWAKE_HTTP_OUTPUT")
	root, err := filepath.Abs("../../analysis/p7")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{file, output} {
		absolute, e := filepath.Abs(name)
		if e != nil || !strings.HasPrefix(absolute, root+string(os.PathSeparator)) {
			t.Fatal("owned P7 immutable source/output required")
		}
	}
	raw, err := os.ReadFile(file)
	if err != nil || p6Digest(raw) != os.Getenv("HYJS_P7_AWAKE_HTTP_START_SHA256") {
		t.Fatal("actual HTTP typed16 SHA", err)
	}
	start := &pb.SCAwakeCopyFightBegin{}
	if proto.Unmarshal(raw, start) != nil || start.GetFightId() == "" || start.GetRoomId() != "map_forest_003" || start.GetMaxRoundNum() != 40 {
		t.Fatal("exact original typed16 projection")
	}
	s := p6OriginalTables(t)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: start.BattlePlayer, Monster: start.BattleMonser, Seed: start.GetRandomSeed(), MaxRound: start.GetMaxRoundNum(), BattleType: 16, AttackerDamageFactor: proto.Float64(1), DefenderDamageFactor: proto.Float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = P7Awake16ProofForInput(input); err != nil {
		t.Fatal(err)
	}
	terminal, err := s.computeFrozenBattleFinalHP(t.Context(), input, nil, true)
	if err != nil || terminal.Outcome.Authoritative {
		t.Fatal("independent original automatic typed16 client calculation", err)
	}
	encoded, err := json.Marshal(terminal)
	if err != nil {
		t.Fatal(err)
	}
	writer, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = writer.Write(encoded); err != nil {
		writer.Close()
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestP7AwakeBusinessUnsupportedRosterDoesNotSpend(t *testing.T) {
	f := p7AwakeExploreFixture(t)
	if err := db.GetIns().Model(&db.P7AwakePolicy{}).Where("id=?", constUtil.P7AwakeBattlePolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	before, bg := f.profile(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	if _, err := f.s.p7AwakeExplore(f.ctx, f.id, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now, 399001); err == nil {
		t.Fatal("starter047 borrowed normal001003 type16 authority")
	}
	after, ag := f.profile(t)
	if !proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, bg, ag) || p1Count(t, &db.P7AwakeRun{}, "player_id=?", f.id) != 0 || p1Count(t, &db.P7AwakeExploration{}, "player_id=?", f.id) != 0 || p1Count(t, &db.P6BattleRecord{}, "player_id=? AND source=?", f.id, constUtil.P7AwakeBattleReplaySource) != 0 {
		t.Fatal("unproved entry changed business state")
	}
	var daily db.P7AwakeState
	if err := db.GetIns().Where("player_id=?", f.id).First(&daily).Error; err == nil && daily.Used != 0 {
		t.Fatal("unproved entry consumed daily count")
	}
}

func TestP7AwakeBusinessPendingTimeoutOnlySettlesActuallyEarnedBox(t *testing.T) {
	f := p7AwakeBusinessFixture(t)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	before, bg := f.profile(t)
	enter := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now).(*pb.SCEnterAwakeCopy)
	p7AwakeExploreCall(t, f, &pb.CSAwakeCopyOpenBox{BoxGuid: enter.Boxes[0].Guid}, now)
	start, end := p7AwakeBusinessStartEnd(t, f, enter.Monsters[0].GetGuid(), now)
	expired := now.Add(31 * time.Minute)
	data, err := f.s.p7AwakeCatalogue(f.ctx, f.id, expired)
	if err != nil || data[0].(*pb.SCAwakeCopyData).GetIsAutoSettled() != 1 {
		t.Fatal("expired pending original catalogue did not settle", err)
	}
	if _, err = f.s.p7AwakeFightEnd(f.ctx, f.id, end, expired); err == nil {
		t.Fatal("late true End committed after run timeout")
	}
	after, ag := f.profile(t)
	if after.GetCoin() != before.GetCoin() || simpleOwned(after, ag, "item_320")-simpleOwned(before, bg, "item_320") != 1 {
		t.Fatal("timeout granted unearned monster/score resources")
	}
	var fight db.P7AwakeFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil || fight.Status != constUtil.P7AwakeFightAbandoned || len(fight.EndRequest) != 0 || len(fight.Terminal) != 0 {
		t.Fatal("timeout forged battle result", err)
	}
	if p1Count(t, &db.P6BattleRecord{}, "player_id=? AND source=? AND authoritative=?", f.id, constUtil.P7AwakeBattleReplaySource, true) != 0 || p1Count(t, &db.EventFact{}, "player_id=? AND kind IN ?", f.id, []string{constUtil.P7AwakeCompleteDailyEvent, constUtil.P7AwakeCompleteEvent}) != 0 {
		t.Fatal("timeout minted certification/completion")
	}
	receipt := p7AwakeExploreCall(t, f, &pb.CSAwakeCopySettle{}, expired).(*pb.SCAwakeCopySettle)
	if receipt.GetScore() != 5 || len(receipt.GetScoreRewards()) != 0 {
		t.Fatal("partial timeout got full score award")
	}
}
