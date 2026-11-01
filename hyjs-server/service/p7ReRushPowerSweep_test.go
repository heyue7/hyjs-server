package service

import (
	"fmt"
	"math"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP7ReRushPowerSweepEligibilityReceiptsAndHistory(t *testing.T) {
	f := p7Setup(t)
	if err := f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P7RushPolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	player, game := f.profile(t)
	power, err := f.s.formationFightPower(player, game)
	if err != nil || power <= 0 {
		t.Fatalf("server formation power=%d: %v", power, err)
	}
	// 存档显示值不能替代服务端重新计算的阵容战力。
	if err = updateProfile(f.ctx, f.id, func(p *pb.Player, _ *model.GameState) error {
		p.FightPower = proto.Int32(math.MaxInt32)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	setPower := func(floor int32, required float64) {
		t.Helper()
		result := db.GetIns().Model(&db.P7RushStageRule{}).Where("id=?", fmt.Sprintf("re-rush:%d:3", floor)).Update("recommended_power", required)
		if result.Error != nil || result.RowsAffected != 1 {
			t.Fatalf("stage %d: %v (%d rows)", floor, result.Error, result.RowsAffected)
		}
	}
	call := func(req proto.Message, stamp int64) proto.Message {
		t.Helper()
		out, handled, err := f.s.DispatchP7(f.ctx, f.id, req, stamp)
		if err != nil || !handled || len(out) == 0 {
			t.Fatalf("%T: handled=%v err=%v", req, handled, err)
		}
		return out[0]
	}
	for floor := int32(1); floor <= 4; floor++ {
		setPower(floor, float64(power)+1)
	}
	before, _ := f.profile(t)
	denied := call(&pb.CSRush3Hit{}, 119001).(*pb.SCRush3Hit)
	after, _ := f.profile(t)
	if denied.GetIsValid() != 0 || len(denied.Floor) != 0 || before.GetCoin() != after.GetCoin() || before.GetPoint() != after.GetPoint() || p1Count(t, &db.P7ReRushPowerProof{}, "player_id=?", f.id) != 0 {
		t.Fatal("under-strength formation received sweep or reward")
	}
	setPower(1, float64(power))
	type response struct {
		out     []proto.Message
		handled bool
		err     error
	}
	var ready sync.WaitGroup
	ready.Add(2)
	start := make(chan struct{})
	responses := make(chan response, 2)
	for i := 0; i < 2; i++ {
		go func() {
			ready.Done()
			<-start
			out, handled, err := f.s.DispatchP7(f.ctx, f.id, &pb.CSRush3Hit{}, 119002)
			responses <- response{out: out, handled: handled, err: err}
		}()
	}
	ready.Wait()
	close(start)
	first, second := <-responses, <-responses
	if first.err != nil || second.err != nil || !first.handled || !second.handled || len(first.out) == 0 || len(second.out) == 0 || !proto.Equal(first.out[0], second.out[0]) {
		t.Fatalf("concurrent idempotent sweep: first=%v second=%v", first.err, second.err)
	}
	one := first.out[0].(*pb.SCRush3Hit)
	if one.GetIsValid() != 1 || len(one.Floor) != 1 || one.Floor[0] != 1 {
		t.Fatalf("equal threshold and next-floor stop: %v", one)
	}
	paid, paidGame := f.profile(t)
	if paid.GetCoin()-before.GetCoin() != 7500 || paid.GetPoint()-before.GetPoint() != 33 {
		t.Fatalf("floor-one rewards coin=%d points=%d", paid.GetCoin()-before.GetCoin(), paid.GetPoint()-before.GetPoint())
	}
	f.restart(t)
	if again := call(&pb.CSRush3Hit{}, 119002); !proto.Equal(one, again) {
		t.Fatal("restart changed idempotent sweep receipt")
	}
	replayPlayer, replayGame := f.profile(t)
	if !proto.Equal(paid, replayPlayer) || !p6CopySeriesJSONEqual(t, paidGame, replayGame) {
		t.Fatal("replayed receipt paid twice")
	}
	setPower(2, float64(power))
	setPower(3, float64(power))
	two := call(&pb.CSRush3Hit{}, 119003).(*pb.SCRush3Hit)
	if two.GetIsValid() != 1 || len(two.Floor) != 2 || two.Floor[0] != 2 || two.Floor[1] != 3 {
		t.Fatalf("two floors and section completion: %v", two)
	}
	state, err := p7State(db.GetIns(), f.id, time.Now())
	if err != nil || state.Floor != 4 || state.HighFloor != 3 || state.TotalStars != 9 {
		t.Fatalf("sweep progress: %+v %v", state, err)
	}
	if p1Count(t, &db.P7ReRushPowerProof{}, "player_id=?", f.id) != 3 || p1Count(t, &db.P7RushFight{}, "player_id=?", f.id) != 0 {
		t.Fatal("power sweep needs three proofs and must not fabricate fights")
	}
	wins, err := p7ReRushRunWins(db.GetIns(), state, 1, 3)
	if err != nil || len(wins) != 3 {
		t.Fatalf("section power proofs: %v %v", wins, err)
	}
	var elite db.P7RushStageRule
	if err = db.GetIns().Where("kind=? AND task_id=? AND source=?", constUtil.P7RushElite, "01", constUtil.P7ReRushSource).First(&elite).Error; err != nil {
		t.Fatal(err)
	}
	passed, err := f.s.p7StagePrerequisite(db.GetIns(), f.id, elite)
	if err != nil || !passed {
		t.Fatalf("earned elite prerequisite: passed=%v err=%v", passed, err)
	}
	call(&pb.CSRushGetSectionAddition{}, 0)
	call(&pb.CSRushAddition{Star: proto.Int32(9), Select: proto.String("perpdef,0.09")}, 119004)
	call(&pb.CSRushResetChallengeCount{}, 119005)
	state, err = p7State(db.GetIns(), f.id, time.Now())
	if err != nil || state.Floor != 1 {
		t.Fatalf("reset state: %+v %v", state, err)
	}
	passed, err = f.s.p7StagePrerequisite(db.GetIns(), f.id, elite)
	if err != nil || !passed {
		t.Fatalf("elite unlocked by completed prior run: passed=%v err=%v", passed, err)
	}
	for floor := int32(1); floor <= 3; floor++ {
		setPower(floor, float64(power)+1)
	}
	top := call(&pb.CSRushToTop{}, 119006).(*pb.SCRushToTop)
	if len(top.SweepInfo) != 3 || top.SweepInfo[2].GetIsSectionEnd() != 1 || p1Count(t, &db.P7ReRushPowerProof{}, "player_id=?", f.id) != 3 {
		t.Fatalf("reset lost accepted history or changed its proof: %v", top)
	}
	if err = db.GetIns().Model(&db.P7ReRushPowerProof{}).Where("player_id=? AND run_id=? AND floor=?", f.id, 1, 2).Update("snapshot_hash", "corrupted").Error; err != nil {
		t.Fatal(err)
	}
	state, err = p7State(db.GetIns(), f.id, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p7ReRushRunWins(db.GetIns(), state, 1, 3); err == nil {
		t.Fatal("corrupted historical proof was accepted")
	}
}

// 当前层前两层已有真实三星战斗时，新增战力扫荡须与原战斗共同证明整段。
func TestP7ReRushPowerSweepCompletesMixedRealSection(t *testing.T) {
	f := p7PartyBusinessFixture(t)
	if err := f.s.PublishP7ReRush(f.ctx); err != nil {
		t.Fatal(err)
	}
	for floor := int32(1); floor <= 2; floor++ {
		request := &pb.CSRushFightStart{RushType: proto.Int32(constUtil.P7RushTrial), TaskId: proto.String(fmt.Sprintf("local_rush_%d", floor)), Star: proto.Int32(3)}
		start, err := f.s.p7Start(f.ctx, f.id, request, 119100+int64(floor), false)
		if err != nil {
			t.Fatal(err)
		}
		series, err := f.s.FreezeP6RushSeriesInput(start, true, false, nil, constUtil.P6OriginalClientHash)
		if err != nil {
			t.Fatal(err)
		}
		input, err := p7GenericMechanismRushInputAt(series, 0)
		if err != nil {
			t.Fatal(err)
		}
		terminal, err := f.s.ComputeOriginalP6ProtocolFinalHP(f.ctx, input, nil)
		if err != nil || terminal.Outcome.Winner != 0 {
			t.Fatalf("real floor %d: %v", floor, err)
		}
		if _, err = f.s.p7End(f.ctx, f.id, &pb.CSRushFightEnd{FightId: start.FightId, Result: proto.Int32(1), OptRecord: terminal.OriginalOperates}); err != nil {
			t.Fatal(err)
		}
	}
	player, game := f.profile(t)
	power, err := f.s.formationFightPower(player, game)
	if err != nil || power <= 0 {
		t.Fatalf("formation power=%d: %v", power, err)
	}
	for floor, required := range map[int32]float64{3: float64(power), 4: float64(power) + 1} {
		if err = db.GetIns().Model(&db.P7RushStageRule{}).Where("id=?", fmt.Sprintf("re-rush:%d:3", floor)).Update("recommended_power", required).Error; err != nil {
			t.Fatal(err)
		}
	}
	out, handled, err := f.s.DispatchP7(f.ctx, f.id, &pb.CSRush3Hit{}, 119103)
	if err != nil || !handled || len(out) == 0 {
		t.Fatalf("mixed sweep: %v", err)
	}
	swept := out[0].(*pb.SCRush3Hit)
	if swept.GetIsValid() != 1 || len(swept.Floor) != 1 || swept.Floor[0] != 3 {
		t.Fatalf("unexpected mixed sweep: %v", swept)
	}
	state, err := p7State(db.GetIns(), f.id, time.Now())
	if err != nil || state.Floor != 4 || state.HighFloor != 3 || state.TotalStars != 9 {
		t.Fatalf("mixed section progress: %+v %v", state, err)
	}
	var section db.P7ReRushSection
	if err = db.GetIns().Where("player_id=? AND run_id=? AND section=1", f.id, state.RunID).First(&section).Error; err != nil || !section.Claimed {
		t.Fatalf("mixed section was not paid exactly once: %+v %v", section, err)
	}
	wins, err := p7ReRushRunWins(db.GetIns(), state, 1, 3)
	if err != nil || len(wins) != 3 || wins[1].Star != 3 || wins[2].Star != 3 || wins[3].Star != 3 {
		t.Fatalf("mixed source proof: %+v %v", wins, err)
	}
}
