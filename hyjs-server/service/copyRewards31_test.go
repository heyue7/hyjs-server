package service

import (
	"strconv"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestCopyRewards31FirstMissionProtocolAccounting verifies original protocol reward accounting.
// Winning end requests are explicit unit-test client reports, not original-engine or phone evidence.
func TestCopyRewards31FirstMissionProtocolAccounting(t *testing.T) {
	s, ctx, id, read, _ := copyEncounterOrderSetup(t)
	// Keep the actual CopyExp reward below the original next-level threshold.
	if err := updateProfile(ctx, id, func(p *pb.Player, _ *model.GameState) error {
		p.Level = proto.Int32(14)
		p.Exp = proto.Int64(0)
		p.Coin = proto.Int64(1000)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before, _ := read()
	level := before.GetLevel()
	reward := s.table("CopyExp", "level", strconv.Itoa(int(level)))
	wantCoin := int64(tableNumber(reward, "coin"))
	wantExp := int64(tableNumber(reward, "exp"))
	threshold := int64(tableNumber(s.table("UserExp", "level", strconv.Itoa(int(level))), "exp"))
	if wantCoin <= 0 || wantExp <= 0 || threshold <= before.GetExp()+wantExp {
		t.Fatal("finite level fixture must not cross original threshold", wantCoin, wantExp, threshold)
	}
	stamp := int64(310000)
	dispatch := func(request proto.Message) ([]proto.Message, error) {
		t.Helper()
		raw, err := proto.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		decoded := request.ProtoReflect().Type().New().Interface()
		if err = proto.Unmarshal(raw, decoded); err != nil {
			t.Fatal(err)
		}
		var profile db.Profile
		if err = db.GetIns().First(&profile, id).Error; err != nil {
			t.Fatal(err)
		}
		stamp++
		return s.dispatch(ctx, &model.GamePacket{PlayerId: id, ZoneId: int32(s.OriginalServerID()), Session: profile.Session, Timestamp: stamp, Body: decoded}, "")
	}
	call := func(request proto.Message) []proto.Message {
		t.Helper()
		out, err := dispatch(request)
		if err != nil {
			t.Fatalf("original %T failed: %v", request, err)
		}
		for _, message := range out {
			if failure, ok := message.(*pb.ErrorMessage); ok {
				t.Fatal("original protocol returned error", failure)
			}
		}
		return out
	}
	totals := func(gain *pb.ResultObject) map[string]int64 {
		out := map[string]int64{}
		for _, item := range gain.GetSimps() {
			out[item.GetId()] += int64(item.GetNum())
		}
		return out
	}
	walletUnchanged := func(expected *pb.Player) {
		t.Helper()
		actual, _ := read()
		if actual.GetCoin() != expected.GetCoin() || actual.GetExp() != expected.GetExp() || actual.GetLevel() != expected.GetLevel() || actual.GetStamina() != expected.GetStamina() {
			t.Fatal("unexpected wallet/experience change", actual)
		}
	}
	mission := &pb.CSCopyEnter{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	entered := call(mission)[0].(*pb.SCCopyEnter)
	jobs := map[string]bool{}
	for _, job := range entered.GetJobList() {
		jobs[job.GetTaskId()] = true
	}
	if len(jobs) != 2 || !jobs["1"] || !jobs["4"] {
		t.Fatal("original first mission task contract changed", jobs)
	}
	mapRequest := &pb.CSCopyRunMapInfo{ChapterId: proto.String("1"), MissionId: proto.String("1")}
	call(mapRequest)
	paid, _ := read()
	if paid.GetStamina() != before.GetStamina()-10 {
		t.Fatal("original entry stamina cost changed")
	}
	var finalRequest *pb.CSCopyFightEnd
	var finalReceipt *pb.SCCopyFightEnd
	for index, task := range []string{"1", "4"} {
		start := call(&pb.CSCopyFightStart{ChapterId: proto.String("1"), MissionId: proto.String("1"), TaskId: proto.String(task)})[0].(*pb.SCCopyFightStart)
		request := &pb.CSCopyFightEnd{FightId: start.FightId, BattlePlayer: proto.Clone(start.GetBattlePlayer()).(*pb.BattlePlayer), Result: proto.Int32(1)}
		result := call(request)[0].(*pb.SCCopyFightEnd)
		amounts := totals(result.GetGain())
		if index == 0 {
			if result.GetIsCopyComplete() != 0 || amounts["coin"] != 0 || amounts["exp"] != 0 {
				t.Fatal("intermediate fight awarded whole-mission currency", result)
			}
			walletUnchanged(paid)
			call(mapRequest)
			continue
		}
		finalRequest, finalReceipt = request, result
		currencyRows := map[string]int{}
		for _, item := range result.GetGain().GetSimps() {
			if item.GetId() == "coin" || item.GetId() == "exp" {
				currencyRows[item.GetId()]++
			}
		}
		if currencyRows["coin"] != 1 || currencyRows["exp"] != 1 {
			t.Fatal("original client reads first matching currency row; duplicates are invalid", currencyRows)
		}
		if result.GetIsCopyComplete() != 1 || amounts["coin"] != wantCoin || amounts["exp"] != wantExp {
			t.Fatal("final protocol gain does not match actual-level CopyExp", result, wantCoin, wantExp)
		}
	}
	completed, game := read()
	if completed.GetCoin() != before.GetCoin()+wantCoin || completed.GetLevel() != level || completed.GetExp() != before.GetExp()+wantExp || completed.GetStamina() != paid.GetStamina() {
		t.Fatal("final gain not committed exactly once to player", completed)
	}
	if game.Run == nil || !game.Run.Complete {
		t.Fatal("first mission not complete")
	}
	replay := call(finalRequest)[0].(*pb.SCCopyFightEnd)
	if !proto.Equal(replay, finalReceipt) {
		t.Fatal("same end request did not recover frozen receipt")
	}
	walletUnchanged(completed)
	call(&pb.CSCopyLeave{})
	walletUnchanged(completed)
	_, left := read()
	if left.Run != nil {
		t.Fatal("leave did not clear active run")
	}
	call(&pb.CSCopyLeave{})
	walletUnchanged(completed)
	if _, err := dispatch(finalRequest); err == nil {
		t.Fatal("cleared run accepted an old end request")
	}
	walletUnchanged(completed)
}
