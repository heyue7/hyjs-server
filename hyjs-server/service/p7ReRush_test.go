package service

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP7ReRushCompleteCatalogueMatchesRecoveredReAnchors(t *testing.T) {
	s, _, _ := worldSeedFixture(t)
	if err := s.LoadP7Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	rows, err := s.p7ReRushCatalogue(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 306+34 {
		t.Fatalf("complete 102x3 + 34 elites missing: %d", len(rows))
	}
	seen := map[string]bool{}
	for _, row := range rows {
		if seen[row.ID] || row.Source != constUtil.P7ReRushSource || !row.Enabled {
			t.Fatal("duplicate/unbound source", row.ID)
		}
		seen[row.ID] = true
		monster, err := s.validateP7Stage(row)
		if err != nil {
			t.Fatalf("%s invalid original protocol: %v", row.ID, err)
		}
		count := min(5, 2+(row.Floor-1)/20)
		if row.Kind == constUtil.P7RushElite {
			count = 1
		}
		if len(monster.Formation) != int(count) {
			t.Fatal("Re enemy count", row.ID)
		}
		poses := []int32{3, 7, 8, 9, 13}
		for i, actor := range monster.Formation {
			if actor.GetPose() != poses[i] || actor.Prop.GetHp() != actor.Inst.GetCurHp() || actor.GetSkillCastChance() != .35 {
				t.Fatal("Re actor original wire", row.ID, actor)
			}
		}
		if row.Kind == constUtil.P7RushTrial {
			preview := p7ReRushTaskProjection(row)
			if preview.GetCoin() != (2000+row.Floor*500)*row.Star || preview.GetPoint() != (50+row.Floor*10)*row.Star {
				t.Fatal("original Ready reward projection", row.ID)
			}
		}
	}
	// 七个原FieldRVA锚点的完整协议属性，三星与精英只折合Re自己的系数。
	for _, sample := range []struct {
		floor        int32
		hp, att, def float64
	}{{1, 3500, 220, 50}, {20, 60000, 4000, 700}, {40, 450000, 32000, 7000}, {60, 1800000, 125000, 28000}, {80, 4800000, 350000, 85000}, {90, 7600000, 560000, 145000}, {102, 12000000, 900000, 240000}} {
		for star := int32(1); star <= 3; star++ {
			prop, err := p7ReRushProperties(sample.floor, star, false)
			factor := []float64{1, 1.25, 1.55}[star-1]
			// Re在端点也走Log/Exp；Go和Mono的libm舍入临界可相差1点。
			if err != nil || math.Abs(prop.GetHp()-math.RoundToEven(sample.hp*factor)) > 1 || math.Abs(prop.GetAtt()-math.RoundToEven(sample.att*factor)) > 1 || math.Abs(prop.GetPdef()-math.RoundToEven(sample.def*factor)) > 1 {
				t.Fatalf("Re RVA anchor mismatch %d/%d: %v %v", sample.floor, star, prop, err)
			}
		}
	}
	first, _ := s.p7ReRushEnemy(1, 1, "")
	last, _ := s.p7ReRushEnemy(102, 3, "")
	if first.Formation[0].GetId() != "monster_044" || first.Formation[1].GetId() != "monster_033" || last.Formation[4].GetId() != "hero_024" || last.Formation[4].GetLevel() != 100 {
		t.Fatal("Re fixed roster or level drift")
	}
	// 发布工具可复用同一确定性目录；测试默认不写持久化产物。
	if output := os.Getenv("HYJS_P7_RE_CATALOGUE_EXPORT"); output != "" {
		var exported []map[string]any
		for _, row := range rows {
			exported = append(exported, map[string]any{"rule": row, "enemiesHex": hex.EncodeToString(row.Enemies)})
		}
		raw, err := json.MarshalIndent(exported, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(output, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestP7ReRushRejectsInvalidFloorAndRetainsOriginalMonsterSkills(t *testing.T) {
	for _, pair := range [][2]int32{{0, 1}, {103, 1}, {1, 0}, {1, 4}} {
		if _, err := p7ReRushProperties(pair[0], pair[1], false); err == nil {
			t.Fatal("out-of-Re range accepted", pair)
		}
	}
	s, _, _ := worldSeedFixture(t)
	monster, err := s.p7ReRushEnemy(1, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if monster.Formation[0].GetSkillOrder() != "1044_1,1044_2," || monster.Formation[1].GetSkillOrder() != "1033_1,," {
		t.Fatal("original absent monster skills were fabricated")
	}
	a, _ := proto.MarshalOptions{Deterministic: true}.Marshal(monster)
	other, err := s.p7ReRushEnemy(1, 1, "")
	b, _ := proto.MarshalOptions{Deterministic: true}.Marshal(other)
	if err != nil || !bytes.Equal(a, b) {
		t.Fatal("catalogue source is nondeterministic")
	}
	row := s.table("Hero", "id", "monster_044")
	row["SkillId1"] = "not-original"
	if _, err = s.p7ReRushEnemy(1, 1, ""); err == nil {
		t.Fatal("missing original skill source accepted")
	}
}

// 仅前段三个真实战斗。有限历史装备队伍来自既有固定夹具；不导入通关或资源。
func TestP7ReRushMySQLFirstSectionOriginalEndRewardAdditionResetSweepCold(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("HYJS_P7_MYSQL_DATABASE"), "hyjs_test_p7_") {
		t.Skip("owned isolated P7 MySQL required")
	}
	f := p7PartyBusinessFixture(t)
	if err := f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	call := func(request proto.Message, stamp int64) []proto.Message {
		t.Helper()
		out, handled, err := f.s.DispatchP7(f.ctx, f.id, request, stamp)
		if err != nil || !handled {
			t.Fatalf("%T: %v", request, err)
		}
		return out
	}
	before, _ := f.profile(t)
	var last *pb.CSRushFightEnd
	for floor := int32(1); floor <= 3; floor++ {
		tasks := call(&pb.CSRushCurrentInfo{}, 0)[0].(*pb.SCRushCurrentInfo).TaskInfoList
		if len(tasks) != 3 || tasks[0].GetCoin() != (2000+floor*500) || tasks[0].GetPoint() != (50+floor*10) {
			t.Fatal("actual Ready source", tasks)
		}
		start := call(&pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: tasks[0].TaskId, Star: proto.Int32(1)}, 19400+int64(floor))[0].(*pb.SCRushFightStart)
		if start.BattlePlayer.Formation[0].GetWeaponId() != "equip_037" {
			t.Fatal("actual equipped weapon missing from Re Start")
		}
		series, err := f.s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
		if err != nil {
			t.Fatal(err)
		}
		input, err := p7GenericMechanismRushInputAt(series, 0)
		if err != nil {
			t.Fatal(err)
		}
		client, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
		if err != nil || client.Outcome.Authoritative {
			t.Fatal("independent original test client", err)
		}
		if client.Outcome.Winner != 0 {
			t.Fatalf("finite original roster lost Re floor%d; preserve outcome %d", floor, client.Outcome.Winner)
		}
		last = &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(1), OptRecord: client.OriginalOperates}
		result := call(last, 0)[0].(*pb.SCRushFightEnd)
		if result.GetResult() != 1 || result.GetStar() != 1 {
			t.Fatal("actual Re settlement", result)
		}
		if floor < 3 {
			_, _, err = f.s.DispatchP7(f.ctx, f.id, &pb.CSRushGetSectionReward{}, 19410+int64(floor))
			if err == nil {
				t.Fatal("partial Re section paid")
			}
		}
	}
	after, _ := f.profile(t)
	if after.GetCoin()-before.GetCoin() != 9000 || after.GetPoint()-before.GetPoint() != 210 {
		t.Fatal("Re exact first three rewards", after.GetCoin()-before.GetCoin(), after.GetPoint()-before.GetPoint())
	}
	info := call(&pb.CSGetRushInfo{}, 0)[0].(*pb.SCGetRushInfo)
	if info.GetCurFloor() != 4 || info.GetIsRewardNeedGet() != 1 || info.GetIsAdditionNeedGet() != 1 {
		t.Fatal("earned Re section flags", info)
	}
	if _, _, err := f.s.DispatchP7(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local_rush_4"), Star: proto.Int32(1)}, 19420); err == nil {
		t.Fatal("next section started before claiming and choosing")
	}
	claim := call(&pb.CSRushGetSectionReward{}, 19421)[0]
	options := call(&pb.CSRushGetSectionAddition{}, 0)[0].(*pb.SCRushGetSectionAddition)
	if options.GetCostStar() != 3 || options.GetStarResult3() != "perhp,0.03" {
		t.Fatal("Re original addition options", options)
	}
	call(&pb.CSRushAddition{Select: options.StarResult3, Star: proto.Int32(3)}, 19422)
	bonus := call(&pb.CSRushAdditionView{}, 0)[0].(*pb.SCRushAdditionView).Prop
	if bonus.GetPerhp() != .03 || bonus.GetPeratt() != 0 {
		t.Fatal("Re HP additive bonus", bonus)
	}
	player, game := f.profile(t)
	party, err := f.s.battlePlayer(player, game)
	if err != nil {
		t.Fatal(err)
	}
	state, err := p7State(db.GetIns(), f.id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	boosted, err := f.s.p7ApplyReRushBonus(db.GetIns(), state, party)
	if err != nil || boosted.Formation[0].Prop.GetHp() != math.RoundToEven(party.Formation[0].Prop.GetHp()*1.03) {
		t.Fatal("purchased real bonus not folded into original battle input", err)
	}
	f.restart(t)
	replay := call(&pb.CSRushGetSectionReward{}, 19421)[0]
	if !proto.Equal(claim, replay) {
		t.Fatal("cold Re reward receipt changed")
	}
	current, currentGame := f.profile(t)
	if !proto.Equal(player, current) || !p6CopySeriesJSONEqual(t, game, currentGame) {
		t.Fatal("cold receipt minted resources")
	}
	call(last, 0)
	call(&pb.CSRushResetChallengeCount{}, 19423)
	invalidSweep := call(&pb.CSRush3Hit{}, 19424)[0].(*pb.SCRush3Hit)
	if invalidSweep.GetIsValid() != 0 || len(invalidSweep.Gain) != 0 {
		t.Fatal("one-star historic source allowed three-star sweep")
	}
	sweep := call(&pb.CSRushToTop{}, 19425)[0].(*pb.SCRushToTop)
	if len(sweep.SweepInfo) != 3 || sweep.SweepInfo[2].GetIsSectionEnd() != 1 || sweep.SweepInfo[2].GetSectionProp() != "perhp,0.03" {
		t.Fatal("original bounded sweep and section result", sweep)
	}
	sweepPlayer, sweepGame := f.profile(t)
	sweepState, err := p7State(db.GetIns(), f.id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	qualified, err := p7ReRushRunWins(db.GetIns(), sweepState, 1, 3)
	if err != nil || len(qualified) != 3 {
		t.Fatal("legitimate same-run sweep witnesses missing from elite prerequisite", err)
	}
	replayed := call(&pb.CSRushToTop{}, 19425)[0]
	if !proto.Equal(sweep, replayed) {
		t.Fatal("sweep receipt changed")
	}
	final, finalGame := f.profile(t)
	if !proto.Equal(sweepPlayer, final) || !p6CopySeriesJSONEqual(t, sweepGame, finalGame) {
		t.Fatal("sweep replay minted resources")
	}
	if p1Count(t, &db.P7RushFight{}, "player_id=?", f.id) != 3 {
		t.Fatal("sweep manufactured battle records")
	}
	// 首个精英使用已合格的原type10手控消费者，只打开匹配来源的隔离策略。
	if err = db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=?", constUtil.P7RushEliteManualDriverPolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	bosses := call(&pb.CSGetRushEliteBossInfo{}, 0)[0].(*pb.SCGetRushEliteBossInfo).BossInfoList
	if len(bosses) != 34 || bosses[0].GetEliteTaskId() != "01" || bosses[0].GetIsOpen() != 1 || bosses[1].GetIsOpen() != 0 {
		t.Fatal("Re first elite real prerequisite projection", bosses)
	}
	elite := call(&pb.CSRushFightStart{RushType: proto.Int32(2), TaskId: proto.String("01"), Star: proto.Int32(0)}, 19426)[0].(*pb.SCRushFightStart)
	if elite.GetRushType() != 2 || len(elite.BattleMonsterList[0].Formation) != 1 || elite.BattleMonsterList[0].Formation[0].GetBloodNumber() != 3 {
		t.Fatal("Re first elite actual original start", elite)
	}
	series, err := f.s.FreezeP6RushSeriesInput(elite, false, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	input, err := p7GenericMechanismRushInputAt(series, 0)
	if err != nil {
		t.Fatal(err)
	}
	client, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	won := int32(0)
	if client.Outcome.Winner == 0 {
		won = 1
	}
	eliteEnd := &pb.CSRushFightEnd{FightId: elite.FightId, Result: proto.Int32(won), OptRecord: client.OriginalOperates}
	acceptedElite := call(eliteEnd, 0)[0].(*pb.SCRushFightEnd)
	if acceptedElite.GetResult() != won {
		t.Fatal("Re elite changed the original actual winner")
	}
	t.Logf("Re first elite type10 actual original Start/End: winner=%d, reward=%v", won, acceptedElite.ResultObj)
	t.Log("Re original wire: three actual first-section fights, exact rewards, claim/addition ordering, cold receipts, real-history-only bounded sweep and no new combat records")
}

func TestP7ReRushMySQLFirstFloorActualDefeatTraderOnceReset(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("HYJS_P7_MYSQL_DATABASE"), "hyjs_test_p7_") {
		t.Skip("owned isolated P7 MySQL required")
	}
	f := p7SectionFixture(t)
	for _, load := range []func(string) error{f.s.LoadP5Tables, f.s.LoadP9Tables} {
		if err := load("../../analysis/tables"); err != nil {
			t.Fatal(err)
		}
	}
	f.s.P5Ready, f.s.P9PetsReady = true, true
	if err := f.s.InitializeP9Pets(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id=?", constUtil.P7RushManualDriverPolicyID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error { p.Diamond = proto.Int64(20); return nil }); err != nil {
		t.Fatal(err)
	}
	call := func(request proto.Message, stamp int64) []proto.Message {
		t.Helper()
		out, handled, err := f.s.DispatchP7(f.ctx, f.id, request, stamp)
		if err != nil || !handled {
			t.Fatalf("%T: %v", request, err)
		}
		return out
	}
	before, _ := f.profile(t)
	if _, _, err := f.s.DispatchP7(f.ctx, f.id, &pb.CSRushMazeTraderBuy{}, 19440); err == nil {
		t.Fatal("merchant sold without accepted real failure")
	}
	start := call(&pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local_rush_1"), Star: proto.Int32(3)}, 19441)[0].(*pb.SCRushFightStart)
	series, err := f.s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
	if err != nil {
		t.Fatal(err)
	}
	input, err := p7GenericMechanismRushInputAt(series, 0)
	if err != nil {
		t.Fatal(err)
	}
	client, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
	if err != nil {
		t.Fatal(err)
	}
	if client.Outcome.Winner == 0 {
		t.Fatal("finite unequipped starter won 3-star Re floor; preserve result and pick another real defeat source")
	}
	call(&pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(0), OptRecord: client.OriginalOperates}, 0)
	info := call(&pb.CSGetRushInfo{}, 0)[0].(*pb.SCGetRushInfo)
	if info.GetIsMustReset() != 1 || info.GetIsMazeTraderValid() != 1 || info.GetCurFloor() != 1 {
		t.Fatal("accepted failure did not expose original merchant/reset", info)
	}
	quote := call(&pb.CSRushMazeTrader{}, 0)[0].(*pb.SCRushMazeTrader)
	if quote.GetOriginalPrice() != 50 || quote.GetDiscountPrice() != 20 || quote.Result.GetId() != "item_005" || quote.Result.GetNum() != 10 {
		t.Fatal("Re actual merchant quote", quote)
	}
	buy := call(&pb.CSRushMazeTraderBuy{}, 19442)[0]
	if costs := buy.(*pb.SCRushMazeTraderBuy).GetCost(); len(costs) != 1 || costs[0].GetId() != "diamond" || costs[0].GetNum() != 20 {
		t.Fatal("original trader callback cost missing", buy)
	}
	after, game := f.profile(t)
	if after.GetDiamond() != before.GetDiamond()-20 || after.GetPoint() != before.GetPoint() {
		t.Fatal("merchant exact price or defeat award changed wallet")
	}
	f.restart(t)
	if replay := call(&pb.CSRushMazeTraderBuy{}, 19442)[0]; !proto.Equal(buy, replay) {
		t.Fatal("merchant cold receipt changed")
	}
	cold, coldGame := f.profile(t)
	if !proto.Equal(cold, after) || !p6CopySeriesJSONEqual(t, game, coldGame) {
		t.Fatal("merchant replay minted resources")
	}
	if _, _, err = f.s.DispatchP7(f.ctx, f.id, &pb.CSRushMazeTraderBuy{}, 19443); err == nil {
		t.Fatal("merchant sold twice in one run")
	}
	call(&pb.CSRushResetChallengeCount{}, 19444)
	info = call(&pb.CSGetRushInfo{}, 0)[0].(*pb.SCGetRushInfo)
	if info.GetCurFloor() != 1 || info.GetIsMustReset() != 0 || info.GetIsMazeTraderValid() != 0 {
		t.Fatal("Re reset retained stopped/trader state", info)
	}
	t.Log("Re first-floor actual original defeat: no battle reward; merchant exact 20 diamond/item005x10, cold/duplicate safe, original free reset clears stopped and merchant")
}

func TestP7ReRushMySQLPublishKeepsOldPendingFightSectionRewardAndBuff(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("HYJS_P7_MYSQL_DATABASE"), "hyjs_test_p7_") {
		t.Skip("owned isolated P7 MySQL required")
	}
	f := p7PartyBusinessFixture(t)
	p7SectionPublish(t)
	p7PublishRushAddition(t)
	// 本测试明确从尚未发布Re的新库状态开始，保留旧首层实际在途快照。
	if err := db.GetIns().Model(&db.P7ReRushPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
		t.Fatal(err)
	}
	first, err := f.s.p7Start(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String("local-section-1"), Star: proto.Int32(1)}, 19460, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	call := func(request proto.Message, stamp int64) []proto.Message {
		t.Helper()
		out, handled, err := f.s.DispatchP7(f.ctx, f.id, request, stamp)
		if err != nil || !handled {
			t.Fatalf("%T: %v", request, err)
		}
		return out
	}
	pending := call(&pb.CSRushCurrentInfo{}, 0)[0].(*pb.SCRushCurrentInfo)
	if len(pending.TaskInfoList) != 1 || pending.TaskInfoList[0].GetTaskId() != "local-section-1" {
		t.Fatal("publishing Re replaced old in-flight task", pending)
	}
	for floor := int32(1); floor <= 3; floor++ {
		start := first
		if floor > 1 {
			if _, _, err := f.s.DispatchP7(f.ctx, f.id, &pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: proto.String(fmt.Sprintf("local_rush_%d", floor)), Star: proto.Int32(1)}, 19470+int64(floor)); err == nil {
				t.Fatal("direct Re opcode mixed the frozen old first section")
			}
			tasks := call(&pb.CSRushCurrentInfo{}, 0)[0].(*pb.SCRushCurrentInfo).TaskInfoList
			if len(tasks) != 3 || tasks[0].GetTaskId() != fmt.Sprintf("local-section-%d", floor) {
				t.Fatal("old accepted first-section offer lost remaining source", tasks)
			}
			start = call(&pb.CSRushFightStart{RushType: proto.Int32(1), TaskId: tasks[0].TaskId, Star: proto.Int32(1)}, 19460+int64(floor))[0].(*pb.SCRushFightStart)
		}
		series, err := f.s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
		if err != nil {
			t.Fatal(err)
		}
		input, err := p7GenericMechanismRushInputAt(series, 0)
		if err != nil {
			t.Fatal(err)
		}
		client, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
		if err != nil || client.Outcome.Winner != 0 {
			t.Fatal("old fixed original source did not win", err)
		}
		call(&pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(1), OptRecord: client.OriginalOperates}, 0)
	}
	info := call(&pb.CSGetRushInfo{}, 0)[0].(*pb.SCGetRushInfo)
	if info.GetCurFloor() != 4 || info.GetIsRewardNeedGet() != 1 || info.GetIsAdditionNeedGet() != 1 {
		t.Fatal("old earned flags hidden by Re4", info)
	}
	claim := call(&pb.CSRushGetSectionReward{}, 19464)[0].(*pb.SCRushGetSectionReward)
	if len(claim.Gain.Simps) != 1 || claim.Gain.Simps[0].GetId() != "item_001" || claim.Gain.Simps[0].GetNum() != 1 {
		t.Fatal("old frozen reward rewritten by new Re catalogue", claim)
	}
	options := call(&pb.CSRushGetSectionAddition{}, 0)[0].(*pb.SCRushGetSectionAddition)
	if options.GetStarResult3() != "peratt,0.03" {
		t.Fatal("old frozen addition overwritten", options)
	}
	call(&pb.CSRushAddition{Star: proto.Int32(3), Select: options.StarResult3}, 19465)
	bonus := call(&pb.CSRushAdditionView{}, 0)[0].(*pb.SCRushAdditionView).Prop
	if bonus.GetPeratt() != .03 {
		t.Fatal("old legitimately purchased buff lost on Re switch", bonus)
	}
	f.restart(t)
	if replay := call(&pb.CSRushGetSectionReward{}, 19464)[0]; !proto.Equal(claim, replay) {
		t.Fatal("old original claim receipt lost after restart")
	}
	tasks := call(&pb.CSRushCurrentInfo{}, 0)[0].(*pb.SCRushCurrentInfo).TaskInfoList
	if len(tasks) != 3 || tasks[0].GetTaskId() != "local_rush_4" {
		t.Fatal("completed legacy group did not hand off to Re4", tasks)
	}
	player, game := f.profile(t)
	party, err := f.s.battlePlayer(player, game)
	if err != nil {
		t.Fatal(err)
	}
	state, err := p7State(db.GetIns(), f.id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	boosted, err := f.s.p7ApplyReRushBonus(db.GetIns(), state, party)
	if err != nil || boosted.Formation[0].Prop.GetAtt() != math.RoundToEven(party.Formation[0].Prop.GetAtt()*1.03) {
		t.Fatal("old frozen buff did not reach new Re battle snapshot", err)
	}
	t.Log("old first Start→publish full Re→same old End and remaining frozen first section→exact old reward/addition/cold receipts→Re4 with owned old buff preserved")
}

func TestP7ReRushAdditionProjectionUsesOriginalPercentFields(t *testing.T) {
	rates := &pb.BaseProp{Hp: proto.Float64(.03), Att: proto.Float64(.06), Pdef: proto.Float64(.09), Mdef: proto.Float64(.09)}
	before := proto.Clone(rates)
	wire, err := proto.Marshal(&pb.SCRushAdditionView{Prop: p7ReRushBonusProjection(rates)})
	if err != nil {
		t.Fatal(err)
	}
	cold := &pb.SCRushAdditionView{}
	if err = proto.Unmarshal(wire, cold); err != nil {
		t.Fatal(err)
	}
	if cold.Prop.GetPerhp() != .03 || cold.Prop.GetPeratt() != .06 || cold.Prop.GetPerpdef() != .09 || cold.Prop.GetPermdef() != .09 || cold.Prop.GetHp() != 0 || cold.Prop.GetAtt() != 0 {
		t.Fatal("original UI per-field consumers cannot show Re cumulative bonus", cold)
	}
	if !proto.Equal(before, rates) {
		t.Fatal("UI projection modified combat input rates")
	}
}

func TestP7ReRushMySQLPublishKeepsOldTraderQuoteReceiptAndMiss(t *testing.T) {
	if !strings.HasPrefix(os.Getenv("HYJS_P7_MYSQL_DATABASE"), "hyjs_test_p7_") {
		t.Skip("owned isolated P7 MySQL required")
	}
	for _, mode := range []string{"purchased-before", "purchase-after", "did-not-appear"} {
		t.Run(mode, func(t *testing.T) {
			f, request, at := p7MazeFixture(t)
			if err := db.GetIns().Model(&db.P7ReRushPolicy{}).Where("id=1").Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			// Keep this frozen legacy fixture on its original scoped driver.
			if err := db.GetIns().Model(&db.P7RushManualPolicy{}).Where("id IN ?", []int64{1, 2}).Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
			appear := int32(1000000)
			if mode == "did-not-appear" {
				appear = 0
			}
			p7MazePublish(t, at, appear)
			p7MazeAcceptedEnd(t, f, request, 19500)
			var accepted proto.Message
			if mode == "purchased-before" {
				out, err := f.s.P7MazeTrader(f.ctx, f.id, &pb.CSRushMazeTraderBuy{}, 19501, at)
				if err != nil {
					t.Fatal(err)
				}
				accepted = out[0]
			}
			if err := f.s.PublishP7ReRush(f.ctx); err != nil {
				t.Fatal(err)
			}
			before, game := f.profile(t)
			out, handled, err := f.s.DispatchP7(f.ctx, f.id, &pb.CSRushMazeTraderBuy{}, 19501)
			if mode == "did-not-appear" {
				if err == nil {
					t.Fatal("old frozen nonappearance acquired new Re trader")
				}
			} else {
				if err != nil || !handled {
					t.Fatal("old original merchant consumer lost", err)
				}
				if mode == "purchased-before" && !proto.Equal(out[0], accepted) {
					t.Fatal("old original merchant receipt rewritten")
				}
				result := out[0].(*pb.SCRushMazeTraderBuy)
				if len(result.Gain.Simps) != 1 || result.Gain.Simps[0].GetId() != "item_001" || result.Gain.Simps[0].GetNum() != 2 || result.Cost[0].GetNum() != 10 {
					t.Fatal("old merchant frozen goods/price overwritten by Re", result)
				}
			}
			after, afterGame := f.profile(t)
			if mode != "purchase-after" && (!proto.Equal(before, after) || !p6CopySeriesJSONEqual(t, game, afterGame)) {
				t.Fatal("publication replay/miss deducted or minted resources")
			}
			if _, _, err = f.s.DispatchP7(f.ctx, f.id, &pb.CSRushMazeTraderBuy{}, 19502); err == nil {
				t.Fatal("old one-purchase/no-appearance offer allowed another Re purchase")
			}
			state, err := p7State(db.GetIns(), f.id, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			valid, err := p7ReRushTraderValid(db.GetIns(), state)
			if err != nil || valid {
				t.Fatal("old-source defeat unlocked new Re merchant", err)
			}
		})
	}
}
