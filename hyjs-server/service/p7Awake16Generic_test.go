package service

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// 前段代表仅准备隔离第三个原角色，再经原上阵/探索/Begin/End与冷回执；无手机通过声称。
func TestP7AwakeGenericThreeActorOriginalEntryEndAndCold(t *testing.T) {
	f := p7AwakeBusinessFixture(t)
	var third *pb.Warrior
	if err := updateProfile(f.ctx, f.id, func(p *pb.Player, g *model.GameState) error {
		// 这是有限角色前提，不增加生产获取渠道、普通币钻或已赢结果。
		if _, err := f.s.GrantFixedReward(p, g, "hero_011,1"); err != nil {
			return err
		}
		for _, hero := range g.Warriors {
			if hero.GetId() == "hero_011" {
				third = proto.Clone(hero).(*pb.Warrior)
			}
		}
		return nil
	}); err != nil || third == nil {
		t.Fatal("finite third original actor", err)
	}
	if _, err := f.s.formationOn(f.ctx, f.id, &pb.CSFormationOn{Id: third.Id, Guid: third.Guid, FormatPos: proto.Int32(3)}); err != nil {
		t.Fatal("original third formation", err)
	}
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.FixedZone("UTC+8", 8*3600))
	enter := p7AwakeExploreCall(t, f, &pb.CSEnterAwakeCopy{CopyId: proto.String("501")}, now).(*pb.SCEnterAwakeCopy)
	begin, err := f.s.p7AwakeFightBegin(f.ctx, f.id, &pb.CSAwakeCopyFightBegin{MonGuid: enter.Monsters[0].Guid}, now)
	if err != nil {
		t.Fatal("generic original Begin", err)
	}
	start := begin[0].(*pb.SCAwakeCopyFightBegin)
	if len(start.BattlePlayer.Formation) != 3 {
		t.Fatal("public Begin removed actual actors", len(start.BattlePlayer.Formation))
	}
	var fight db.P7AwakeFight
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil || fight.ScopeProof != constUtil.P7Awake16GenericCompatibilityProof {
		t.Fatal("generic scope not frozen", err)
	}
	input, _, err := p7AwakeFightInput(db.GetIns().WithContext(f.ctx), &fight)
	if err != nil || validateP7Awake16Scope(input) == nil {
		t.Fatal("new source still depended on old two-actor scope", err)
	}
	// 测试客户端独立使用原自动驱动生成完整记录，不调用生产结算给出预设赢家。
	client, err := f.s.computeFrozenBattleFinalHP(f.ctx, input, nil, true)
	if err != nil || client.Outcome.Authoritative {
		t.Fatal("independent source record", err)
	}
	reported := int32(0)
	if client.Outcome.Winner == 0 {
		reported = 1
	}
	end := &pb.CSAwakeCopyFightEnd{FightId: start.FightId, IsWinClient: proto.Int32(reported), OptRecord: client.OriginalOperates}
	reply, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now.Add(time.Second))
	if err != nil || reply[0].(*pb.SCAwakeCopyFightEnd).GetIsWinServer() != reported {
		t.Fatal("generic original End", err)
	}
	if err = db.GetIns().First(&fight, "id=?", start.GetFightId()).Error; err != nil {
		t.Fatal(err)
	}
	var terminal model.CombatFinalHPOutcome
	if json.Unmarshal(fight.Terminal, &terminal) != nil || len(terminal.FinalActors) != 5 || terminal.Outcome.CapabilityHash != constUtil.P7Awake16GenericCompatibilityProof || terminal.OriginalRecordSHA256 == "" {
		t.Fatal("generic full actors/source proof omitted")
	}
	f.restart(t)
	recovered, err := f.s.p7AwakeFightEnd(f.ctx, f.id, end, now.Add(2*time.Second))
	if err != nil || !proto.Equal(reply[0], recovered[0]) {
		t.Fatal("generic cold receipt", err)
	}
	bad := proto.Clone(end).(*pb.CSAwakeCopyFightEnd)
	bad.OptRecord.AttackerTotalLeftHp = proto.Float64(bad.OptRecord.GetAttackerTotalLeftHp() + 1)
	if _, err = f.s.p7AwakeFightEnd(f.ctx, f.id, bad, now.Add(2*time.Second)); err == nil {
		t.Fatal("cold terminal mutation accepted")
	}
	missing, err := cloneP6CalculationInput(input)
	if err != nil {
		t.Fatal(err)
	}
	missing.Attacker.Formation[0].TalentList = append(missing.Attacker.Formation[0].TalentList, "tal_unknown_awake_generic")
	if _, err = f.s.p7Awake16GenericProof(f.ctx, missing); err == nil || !strings.Contains(err.Error(), "tal_unknown_awake_generic") {
		t.Fatal("missing original source was not named", err)
	}
}

func TestP7AwakeGenericAutoClockKeepsOriginalTypeAndShape(t *testing.T) {
	input := &model.FrozenBattleInput{BattleType: 16, Attacker: &pb.BattlePlayer{}, Monster: &pb.BattleMonster{}, DiagnosticAIDriverPolicy: constUtil.P6DiagnosticManualAutoStart, DiagnosticManualAutoStartRoundTime: proto.Float64(30.1)}
	if err := ValidateP6AIDriverPolicy(input); err != nil {
		t.Fatal("original type16 PVE clock", err)
	}
	for _, variant := range []string{"negative", "late", "defender", "first", "compatibility", "foreign-mode"} {
		copy := *input
		switch variant {
		case "negative":
			copy.DiagnosticManualAutoStartRoundTime = proto.Float64(-0.1)
		case "late":
			copy.DiagnosticManualAutoStartRoundTime = proto.Float64(110.1)
		case "defender":
			copy.Defender = &pb.BattlePlayer{}
		case "first":
			copy.DefenderFirst = true
		case "compatibility":
			copy.PVECompatibilityRule = "foreign"
		case "foreign-mode":
			copy.BattleType = 18
		}
		if err := ValidateP6AIDriverPolicy(&copy); err == nil {
			t.Fatal("clock scope expanded", variant)
		}
	}
}
