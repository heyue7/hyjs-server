package service

import (
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"os"
	"testing"
)

func p6ActualSameIDKF(t *testing.T) *pb.KFReplayData {
	t.Helper()
	raw, err := os.ReadFile("../../analysis/p14/final-game-actual-native/actual-Game1-KFReplayData.pb")
	if err != nil || p6Digest(raw) != "18473d1c95d92d51850578f69a8fa6273039a6f77d55a0ebd6f64abf699c502c" {
		t.Fatal("actual Game1 source SHA", err)
	}
	out := &pb.KFReplayData{}
	if err = proto.Unmarshal(raw, out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestP6KFActualSameIDOriginalAndExplicitJointViewer(t *testing.T) {
	s := p6OriginalTables(t)
	raw := p6ActualSameIDKF(t)
	before, _ := proto.MarshalOptions{Deterministic: true}.Marshal(raw)
	original, err := s.FreezeP6KFReplayInput(raw, 1)
	if err != nil || !original.IsAttToDef || !proto.Equal(original.Attacker, raw.PlayerList[0]) || !proto.Equal(original.Defender, raw.PlayerList[1]) {
		t.Fatal("original no-break PID loop first-pair/view mismatch", err)
	}
	if _, err = s.ComputeFrozenBattle(t.Context(), original, nil); err == nil {
		t.Fatal("original duplicate multi-pair source was calculated as a whole single pair")
	}
	for index, source := range raw.PlayerList {
		local, err := s.FreezeP6KFReplayRealmInput(raw, int64(source.GetServerId()), source.GetPlayerId())
		if err != nil || local.IsAttToDef != (index == 1) || !proto.Equal(local.Attacker, source) || !proto.Equal(local.Defender, raw.PlayerList[(index+1)%2]) || local.DefenderFirst != original.DefenderFirst || local.Seed != raw.GetRandSeed() || !proto.Equal(local.KFReplay, raw) {
			t.Fatal("joint source or global initiative changed", index, err)
		}
		if _, err = s.ComputeP6CrossStarterSettlement(t.Context(), local); err == nil {
			t.Fatal("KF borrowed ordinary typed17 certificate")
		}
		clone, err := cloneP6CalculationInput(local)
		if err != nil {
			t.Fatal(err)
		}
		clone.KFReplayViewer.ServerID = 999
		if local.KFReplayViewer.ServerID != int64(source.GetServerId()) {
			t.Fatal("viewer metadata aliased")
		}
		if validateFrozenP6(clone) == nil {
			t.Fatal("wrong joint realm accepted")
		}
	}
	after, _ := proto.MarshalOptions{Deterministic: true}.Marshal(raw)
	if string(before) != string(after) {
		t.Fatal("actual original pair or IDs changed")
	}
}

func TestP6KFJointViewerRejectsAmbiguousAndDanglingMetadata(t *testing.T) {
	s := p6OriginalTables(t)
	raw := p6ActualSameIDKF(t)
	for _, identity := range [][2]int64{{0, 1}, {3, 1}, {1, 999}} {
		if _, err := s.FreezeP6KFReplayRealmInput(raw, identity[0], identity[1]); err == nil {
			t.Fatal("unmatched joint viewer accepted", identity)
		}
	}
	bad := proto.Clone(raw).(*pb.KFReplayData)
	bad.PlayerList[1].ServerId = bad.PlayerList[0].ServerId
	if _, err := s.FreezeP6KFReplayRealmInput(bad, int64(bad.PlayerList[0].GetServerId()), 1); err == nil {
		t.Fatal("same realm collision accepted")
	}
	good, err := s.FreezeP6KFReplayRealmInput(raw, int64(raw.PlayerList[0].GetServerId()), 1)
	if err != nil {
		t.Fatal(err)
	}
	good.KFReplay = nil
	if validateFrozenP6(good) == nil {
		t.Fatal("dangling viewer metadata accepted")
	}
	unknown := &model.FrozenP6KFViewer{PlayerID: 1, ServerID: 1, CompatibilityRule: constUtil.P6KFJointViewerCompatibility + "-unknown"}
	if _, _, err = p6KFViewerProjection(raw, unknown); err == nil {
		t.Fatal("unknown local compatibility accepted")
	}
}
