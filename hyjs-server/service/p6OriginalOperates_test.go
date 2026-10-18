package service

import (
	"encoding/json"
	"os"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP6OriginalProtocolRecordedCampMapAndUIDSplit(t *testing.T) {
	recorded := map[int32]bool{1: true, 8: true, 4: true, 9: true, 10: true, 6: true, 12: true, 18: true, 16: true}
	all := &pb.BattleOperate{FinalRoundNum: proto.Int32(5), AttackerTotalLeftHp: proto.Float64(100), DefenderTotalLeftHp: proto.Float64(0), SkillOpts: []*pb.BattleSkillOperate{{RoundNum: proto.Int32(1), RoundTime: proto.Float64(.1), Camp: proto.Int32(0), ActorUid: proto.String("source_guid_suffix"), SkillIndex: proto.Int32(0)}, {RoundNum: proto.Int32(2), Camp: proto.Int32(1), ActorUid: proto.String("enemy"), SkillIndex: proto.Int32(1)}}}
	for kind := int32(0); kind <= 20; kind++ {
		projection, err := OriginalP6OperateProjection(kind, all)
		count := 0
		if recorded[kind] {
			count = 2
		}
		if err != nil || OriginalP6RecordedCamp(kind, 0) != recorded[kind] || OriginalP6RecordedCamp(kind, 1) || len(projection.SkillOpts) != count || projection.GetFinalRoundNum() != 5 || projection.GetAttackerTotalLeftHp() != 100 || projection.GetDefenderTotalLeftHp() != 0 {
			t.Fatal("original record flags/projection", kind, err)
		}
		if count == 2 && projection.SkillOpts[0].GetActorUid() != "source" {
			t.Fatal("original UID.Split1 truncation silently normalized")
		}
	}
	if all.SkillOpts[0].GetActorUid() != "source_guid_suffix" || len(all.SkillOpts) != 2 {
		t.Fatal("projection mutated full diagnostic input")
	}
}

func TestP6OriginalPVERecordReplaysRecordedPlayerAndAutomaticEnemy(t *testing.T) {
	s := p6OriginalTables(t)
	a := p6Actor("hero_047", "record-player", "047_1", 3, 1000)
	d := p6Actor("hero_047", "record-enemy", "047_1", 7, 1000)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{a}, Inst: &pb.BattlePlayerInst{}}, Monster: &pb.BattleMonster{Formation: []*pb.BattleHero{d}, Inst: &pb.BattleMonsterInst{}}, Seed: 42, MaxRound: 8, BattleType: 4})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := json.Marshal(input)
	out, err := s.ComputeOriginalP6ProtocolBattle(t.Context(), input, nil)
	if err != nil || !out.RecordAttacker || out.RecordDefender || len(out.OriginalOperates.SkillOpts) == 0 || out.Outcome.Authoritative {
		t.Fatal("original PVE recorded player result", err)
	}
	var enemyPress int
	for _, op := range out.Outcome.Operates.SkillOpts {
		if op.GetCamp() == 1 {
			enemyPress++
		}
	}
	if enemyPress == 0 || len(out.OriginalOperates.SkillOpts) != len(out.Outcome.Operates.SkillOpts) {
		t.Fatal("original attacker-owned record must retain both camps successful actions")
	}
	replay, err := s.ComputeOriginalP6ProtocolBattle(t.Context(), input, out.OriginalOperates)
	if err != nil || !proto.Equal(replay.SuppliedOriginalOperates, out.OriginalOperates) {
		t.Fatal("original player-only replay did not automatically execute enemy", err)
	}
	// Original automatic and recorded controllers finish at different callbacks.
	// The exact same original0/0 actors/seed in auto-v86 and recorded-v89 prove
	// CopyEnemyAI.Update's finish is two frames after recorded SkillEnd's hint.
	// This pure eight-round fixture has exactly four such enemy RoundEnd events;
	// retain exact equality for every other event field, protocol byte and result.
	wantReplay := *out.Outcome
	wantReplay.Events = append([]model.CombatEvent(nil), out.Outcome.Events...)
	boundaries := 0
	for index := range wantReplay.Events {
		event := &wantReplay.Events[index]
		if event.Kind == "round_end" && event.Camp == 1 {
			event.Time -= .2
			boundaries++
		}
	}
	if boundaries != 4 || !p6CopySeriesJSONEqual(t, replay.Outcome, &wantReplay) {
		t.Fatal("original recorded boundary or complete enemy calculation drift", boundaries)
	}
	missingNPC := proto.Clone(out.OriginalOperates).(*pb.BattleOperate)
	missingNPC.SkillOpts = nil
	for _, op := range out.OriginalOperates.SkillOpts {
		if op.GetCamp() == 0 {
			missingNPC.SkillOpts = append(missingNPC.SkillOpts, proto.Clone(op).(*pb.BattleSkillOperate))
		}
	}
	if partial, computeErr := s.ComputeOriginalP6ProtocolBattle(t.Context(), input, missingNPC); computeErr == nil || partial != nil {
		t.Fatal("source record dropped real NPC entries while preserving terminal HP")
	}
	bad := proto.Clone(out.OriginalOperates).(*pb.BattleOperate)
	bad.SkillOpts = append(bad.SkillOpts, &pb.BattleSkillOperate{Camp: proto.Int32(1)})
	if partial, computeErr := s.ComputeOriginalP6ProtocolBattle(t.Context(), input, bad); computeErr == nil || partial != nil {
		t.Fatal("malformed source commands admitted into original record")
	}
	bad = proto.Clone(out.OriginalOperates).(*pb.BattleOperate)
	bad.AttackerTotalLeftHp = proto.Float64(bad.GetAttackerTotalLeftHp() + 1)
	if partial, computeErr := s.ComputeOriginalP6ProtocolBattle(t.Context(), input, bad); computeErr == nil || partial != nil {
		t.Fatal("wrong original terminal HP manufactured a result")
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("protocol calculation changed frozen actors or rules")
	}
}

func TestP6OriginalUF14ProducesNoRecordedSkillsDespiteActualPresses(t *testing.T) {
	raw, err := os.ReadFile("../../analysis/p6/native-local-uf14-guard-input-v83/000.pb")
	if err != nil || p6Digest(raw) != "3d69ec109c3d37c6817c871c8e83d14b5924ad96eb843163838b56fd75b5aea8" {
		t.Fatal(err)
	}
	wire := &pb.SCFightPlayback{}
	if err = proto.Unmarshal(raw, wire); err != nil {
		t.Fatal(err)
	}
	s := p6OriginalTables(t)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: wire.Attacker, Monster: wire.Defender2, Seed: wire.GetRandSeed(), MaxRound: wire.GetMaxRound(), BattleType: 14, AttackerDamageFactor: wire.AttackerFactor, DefenderDamageFactor: wire.DefenderFactor})
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.ComputeOriginalP6ProtocolBattle(t.Context(), input, nil)
	if err != nil || out.RecordAttacker || out.RecordDefender || len(out.OriginalOperates.SkillOpts) != 0 || len(out.Outcome.Operates.SkillOpts) != 65 || out.OriginalOperates.GetFinalRoundNum() != 41 || out.Outcome.Winner != 1 {
		t.Fatal("original actual UF14 flags distinct from diagnostic callbacks", err)
	}
	verified, err := s.ComputeOriginalP6ProtocolBattle(t.Context(), input, out.OriginalOperates)
	if err != nil || !p6CopySeriesJSONEqual(t, verified.Outcome, out.Outcome) {
		t.Fatal("unrecorded UF14 automatic mode treated empty record as no AI", err)
	}
}

func TestP6ActualPaidChapter3AcceptedOriginalRecordReplay(t *testing.T) {
	s := p6OriginalTables(t)
	for _, task := range []string{"10", "12"} {
		t.Run(task, func(t *testing.T) {
			prefix := "../../analysis/p15/actual-copy-starts-20261015/mission3-task" + task
			raw, err := os.ReadFile(prefix + ".SCCopyFightStart.pb")
			if err != nil {
				t.Fatal(err)
			}
			start := &pb.SCCopyFightStart{}
			if err = proto.Unmarshal(raw, start); err != nil {
				t.Fatal(err)
			}
			raw, err = os.ReadFile(prefix + ".OriginalOperates.pb")
			if err != nil || p6Digest(raw) != "876a8702571500c345fee44104993887c244ab979a776bc69e7c6245eb4cfa12" {
				t.Fatal("actual sameTX observed original record SHA", err)
			}
			record := &pb.BattleOperate{}
			if err = proto.Unmarshal(raw, record); err != nil {
				t.Fatal(err)
			}
			input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: start.BattlePlayer, Monster: start.BattleMonsterList[0], Seed: start.GetRandSeed(), MaxRound: start.GetMaxRound(), BattleType: 1})
			if err != nil {
				t.Fatal(err)
			}
			out, err := s.ComputeOriginalP6ProtocolBattle(t.Context(), input, record)
			if err != nil {
				t.Fatal("actual original accepted player-only protocol replay", err)
			}
			if !proto.Equal(record, out.SuppliedOriginalOperates) || !p6OriginalProtocolRecordEqual(record, out.OriginalOperates) || out.Outcome.Winner != 0 || out.Outcome.Authoritative {
				t.Fatal("actual source record or declared terminal values drift")
			}
			for _, op := range out.OriginalOperates.SkillOpts {
				if op.Camp != nil || op.GetSkillIndex() == 0 && op.SkillIndex != nil || op.GetRoundTime() == 0 && op.RoundTime != nil {
					t.Fatal("original protobuf-net scalar defaults not omitted")
				}
			}
			t.Logf("actual task%s original record4 playerpresses -> strict replay round%d HP%g/%g RNG%d; native original playback comparison pending", task, out.Outcome.FinalRound, out.OriginalOperates.GetAttackerTotalLeftHp(), out.OriginalOperates.GetDefenderTotalLeftHp(), out.Outcome.RandomDraws)
		})
	}
}

func TestP6OriginalProtocolPlaybackQueuesOneSimultaneousMessagePerTick(t *testing.T) {
	s := p6OriginalTables(t)
	a := p6Actor("hero_047", "simultaneous-a", "047_1", 3, 20000)
	b := p6Actor("hero_047", "simultaneous-b", "047_1", 7, 20000)
	d := p6Actor("hero_047", "simultaneous-enemy", "047_1", 3, 20000)
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: &pb.BattlePlayer{PlayerId: proto.Int64(42), Formation: []*pb.BattleHero{a, b}, Inst: &pb.BattlePlayerInst{}}, Monster: &pb.BattleMonster{Formation: []*pb.BattleHero{d}, Inst: &pb.BattleMonsterInst{}}, Seed: 42, MaxRound: 2, BattleType: 4})
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := s.ComputeOriginalP6ProtocolBattle(t.Context(), input, nil)
	if err != nil || len(baseline.OriginalOperates.SkillOpts) < 2 {
		t.Fatal("two-player recorded source probe", err)
	}
	record := proto.Clone(baseline.OriginalOperates).(*pb.BattleOperate)
	playerMessages := 0
	for _, op := range record.SkillOpts {
		if op.GetCamp() == 0 {
			op.RoundTime = nil
			playerMessages++
		}
	}
	if playerMessages != 2 {
		t.Fatal("exact two player messages required", playerMessages)
	}
	out, err := s.ComputeOriginalP6ProtocolBattle(t.Context(), input, record)
	if err != nil {
		t.Fatal("original simultaneous message queue replay", err)
	}
	if len(out.OriginalOperates.SkillOpts) != len(record.SkillOpts) || out.OriginalOperates.SkillOpts[0].GetRoundTime() != 0 || out.OriginalOperates.SkillOpts[1].GetRoundTime() != .1 || !proto.Equal(record, out.SuppliedOriginalOperates) {
		t.Fatal("original Update one-message-per-tick or immutable supplied source", out.OriginalOperates)
	}
}
