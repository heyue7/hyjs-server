package service

import (
	"google.golang.org/protobuf/proto"
	"strconv"
	"strings"

	pb "hyjs-server/model/protocol"
)

// P5BattleSkillTransport preserves the Warrior basic-skill selection and encodes
// the original BattleActor constructor's three base IDs plus one insertion slot.
// Original Warrior.AwakenSkill uses skillId,slot; legacy bare IDs remain readable.
// The three-basic selection determines the slot without duplicating awakening.
func (s *GameService) P5BattleSkillTransport(w *pb.Warrior) (string, string, error) {
	row := s.table("Hero", "id", w.GetId())
	if row == nil {
		return "", "", p5Error()
	}
	// 原绿色伙伴的三个槽均为空，BattleActorData 构造跳过空槽。
	// 保留槽数供客户端读取；无技能战斗能力仍由 P6 独立核验。
	if row["heroClass"] == "0" && row["quality"] == "12" && row["SkillId1"] == "" && row["SkillId2"] == "" && row["UltSkillId"] == "" {
		if w.GetSkillOrder() != "" || w.GetAwakenSkill() != "" || w.GetAwake().GetStage() != 0 {
			return "", "", p5Error()
		}
		return ",,", "", nil
	}
	// 六绿兼容只补一项普通攻击，仍按原三槽传输，两个空槽不生成额外技能。
	if p15GreenHeroSingleNormalSkill(row) {
		if w.GetSkillOrder() != "" || w.GetAwakenSkill() != "" || w.GetAwake().GetStage() != 0 {
			return "", "", p5Error()
		}
		return row["SkillId1"] + ",,", "", nil
	}
	// 十张原蓝卡只有两项普通技能，空绝技槽不能阻断其面板与战斗投影。
	if row["SkillId1"] == "" || row["SkillId2"] == "" || row["UltSkillId"] == "" && (row["heroClass"] != "0" || row["quality"] != "13") {
		return "", "", p5Error()
	}
	awake := ""
	slot := 2
	if w.GetAwakenSkill() != "" {
		parts := strings.Split(w.GetAwakenSkill(), ",")
		if len(parts) == 2 {
			var err error
			slot, err = strconv.Atoi(parts[1])
			if err != nil || slot < 0 || slot > 2 {
				return "", "", p5Error()
			}
		}
		if len(parts) > 2 || w.GetAwake().GetStage() < 2 || row["IsOpenAwake"] != "1" || parts[0] != row["AwakeSkillId"] {
			return "", "", p5Error()
		}
		awake = row["AwakeSkillId"]
	}
	basic := []string{row["SkillId1"], row["SkillId2"]}
	if awake != "" {
		basic = append(basic, "")
		copy(basic[slot+1:], basic[slot:])
		basic[slot] = awake
	}
	if w.GetSkillOrder() != "" {
		basic = strings.Split(w.GetSkillOrder(), ",")
		// A pre-awakening two-skill selection gains the original final slot.
		if awake != "" && len(basic) == 2 {
			basic = append(basic, awake)
		}
	}
	allowed := map[string]bool{row["SkillId1"]: true, row["SkillId2"]: true}
	if awake != "" {
		allowed[awake] = true
	}
	if len(allowed) != len(basic) {
		return "", "", p5Error()
	}
	base := []string{}
	encoded := ""
	for pos, skill := range basic {
		if skill == "" || !allowed[skill] {
			return "", "", p5Error()
		}
		delete(allowed, skill)
		if skill == awake {
			encoded = awake + "," + strconv.Itoa(pos)
		} else {
			base = append(base, skill)
		}
	}
	base = append(base, row["UltSkillId"])
	return strings.Join(base, ","), encoded, nil
}

// P5WarriorProtocol projects legacy bare awakening IDs for the original UIHero
// decoder without changing the owned GUID, growth or investment source.
func (s *GameService) P5WarriorProtocol(w *pb.Warrior) (*pb.Warrior, error) {
	out := proto.Clone(w).(*pb.Warrior)
	if w.GetAwakenSkill() != "" {
		_, encoded, err := s.P5BattleSkillTransport(w)
		if err != nil {
			return nil, err
		}
		out.AwakenSkill = proto.String(encoded)
	}
	return out, nil
}
