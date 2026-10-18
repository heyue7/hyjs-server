package service

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

// CombatRandom 恢复APK的JRandom 48位状态；每场实例独立，禁止多协程共享。
// JRandom.initialScramble、next、nextDouble分别见IL 0001、001a、0001。
type CombatRandom struct {
	state uint64
	draws int64
}

// NewCombatRandom 使用原协议randSeed建立该次计算的独立状态。
func NewCombatRandom(seed int64) *CombatRandom {
	return &CombatRandom{state: (uint64(seed) ^ constUtil.CombatRandomMultiplier) & constUtil.CombatRandomMask}
}

func (r *CombatRandom) next(bits uint) uint64 {
	r.state = (r.state*constUtil.CombatRandomMultiplier + constUtil.CombatRandomAddend) & constUtil.CombatRandomMask
	return r.state >> (48 - bits)
}

// Value 恢复RandomUtil.value，使用26+27位生成同一条原APK随机序列。
func (r *CombatRandom) Value() float64 {
	r.draws++
	return float64((r.next(26)<<27)+r.next(27)) / constUtil.CombatRandomDoubleDivisor
}

// Range preserves original RandomUtil.Range's multiplication round before
// adding min. The explicit conversion prevents ARM64 FMA from fusing these
// original IL operations, including after this small function is inlined.
func (r *CombatRandom) Range(min, max float64) float64 {
	return float64(r.Value()*float64(max-min)) + min
}

// Draws 返回nextDouble的抽取次数，用于检查命中分支和失败原子性。
func (r *CombatRandom) Draws() int64 { return r.draws }

// CombatReleaseResult 仅表示一次已经选定目标的原始出手，不能替代整场胜败。
type CombatReleaseResult struct {
	Miss        bool
	Critical    bool
	Damage      float64
	TargetHP    float64
	TargetDead  bool
	RandomDraws int64
}

func combatFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func combatActorValid(actor *pb.BattleHero) bool {
	if actor == nil || actor.GetGuid() == "" || actor.GetId() == "" || actor.GetLevel() < 1 || actor.GetProp() == nil || actor.GetInst() == nil {
		return false
	}
	p := actor.GetProp()
	values := []float64{p.GetHp(), p.GetAtt(), p.GetPdef(), p.GetMdef(), p.GetCri(), p.GetAntiCri(), p.GetHit(), p.GetDodge(), p.GetCriFactor(), p.GetDamageAdd(), p.GetDamageReduce(), actor.GetInst().GetCurHp()}
	for _, v := range values {
		if !combatFinite(v) || v < 0 {
			return false
		}
	}
	return p.GetHp() > 0 && actor.GetInst().GetCurHp() > 0 && actor.GetInst().GetCurHp() <= p.GetHp()
}

// EvaluateUnbuffedRelease 复原FRelease.Do的无Buff、无连击、正常特殊态单次伤害分支。
// 输入必须来自服务端冻结的战斗快照；该函数不读取客户端声称的攻击值或伤害。
// 未实现目标选择、AI、回合、怒气、技能冷却、Buff、盾与特殊态，因此不能用于Boss/PK结算。
func (s *GameService) EvaluateUnbuffedRelease(attacker, target *pb.BattleHero, castID string, rng *CombatRandom) (*CombatReleaseResult, error) {
	if rng == nil || !combatActorValid(attacker) || !combatActorValid(target) || attacker.GetGuid() == target.GetGuid() {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	// BaseProp已经是面板终值。存在Buff来源或未展开的百分比时拒绝，避免静默忽略机制。
	for _, actor := range []*pb.BattleHero{attacker, target} {
		p := actor.GetProp()
		if len(actor.GetTalentList()) != 0 || actor.GetAwakeStage() != 0 || actor.GetAwakenSkill() != "" || actor.GetBloodNumber() > 1 || p.GetPeratt() != 0 || p.GetPerhp() != 0 || p.GetPerpdef() != 0 || p.GetPermdef() != 0 {
			return nil, errors.New(constUtil.CombatUnsupported)
		}
		hero := s.table("Hero", "id", actor.GetId())
		if hero == nil || hero["DefaultState"] != "1" {
			return nil, errors.New(constUtil.CombatUnsupported)
		}
	}
	cast := s.table("SkillCast", "SkillCastId", castID)
	skill := s.table("Skill", "SkillId", cast["SkillId"])
	if cast == nil || skill == nil {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	owned := false
	hero := s.table("Hero", "id", attacker.GetId())
	for _, id := range strings.Split(attacker.GetSkillOrder(), ",") {
		if id == cast["SkillId"] && (id == hero["SkillId1"] || id == hero["SkillId2"] || id == hero["UltSkillId"]) {
			owned = true
		}
	}
	if !owned {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	exportType, err := strconv.Atoi(cast["ExportType"])
	if err != nil || (exportType != 1 && exportType != 2) {
		return nil, errors.New(constUtil.CombatUnsupported)
	}
	for _, key := range []string{"CauseSpeState", "AdvSpeState", "ProjectId", "BufferId0", "BufferId1", "BufferId2"} {
		if cast[key] != "" && cast[key] != "0" {
			return nil, errors.New(constUtil.CombatUnsupported)
		}
	}
	factor, err := strconv.ParseFloat(cast["ExportFactor"], 64)
	if err != nil || !combatFinite(factor) || factor <= 0 {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	ultimate, err := strconv.Atoi(skill["IsUltimate"])
	if err != nil || (ultimate != 0 && ultimate != 1) {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	// 先验证所有输入，再消费随机序列；失败调用不改变rng或战斗快照。
	startDraws := rng.Draws()
	// 复制随机状态计算，数值溢出时保持调用前的种子和抽取次数。
	originalRNG := rng
	workingRNG := *rng
	rng = &workingRNG
	a := attacker.GetProp()
	t := target.GetProp()
	hitChance := a.GetHit()/100 - t.GetDodge()/100
	if ultimate == 1 {
		hitChance = 1
	}
	result := &CombatReleaseResult{TargetHP: target.GetInst().GetCurHp()}
	// FRelease.Do 03ea总是抽命中，包含必命中分支；命中失败立即结束本次出手。
	if rng.Value() > hitChance {
		result.Miss = true
		result.RandomDraws = rng.Draws() - startDraws
		*originalRNG = workingRNG
		return result, nil
	}
	critFactor := 1.0
	// FRelease.Do 070f/0739：暴击率相减；不对原始率做人为钳位。
	if rng.Value() <= a.GetCri()/100-t.GetAntiCri()/100 {
		result.Critical = true
		critFactor += a.GetCriFactor() / 100
	}
	defense := t.GetPdef()
	if exportType == 2 {
		defense = t.GetMdef()
	}
	multiplier := math.Max(0, 1+a.GetDamageAdd()/100-t.GetDamageReduce()/100)
	// FSkillCast.init 0065把ExportFactor乘0.01；FRelease.Do 0d10使用减法防御和±10%扰动。
	damage := (a.GetAtt() - defense) * (factor / 100) * multiplier * (1 + rng.Range(-.1, .1)) * critFactor
	// FRelease.Do 0d6f允许高闪避在已经命中的情况下再削减输出。
	if t.GetDodge() > a.GetHit() {
		attenuation := math.Min(0.5, (t.GetDodge()-a.GetHit())/100)
		damage *= 1 - rng.Value()*attenuation*2
	}
	// FRelease.Do 0e1c仍消费格挡随机数。无Buff快照的block为0，不进入格挡分支。
	rng.Value()
	result.Damage = math.Max(1, damage)
	if !combatFinite(result.Damage) {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	result.TargetHP = math.Max(0, result.TargetHP-result.Damage)
	result.TargetDead = result.TargetHP == 0
	result.RandomDraws = rng.Draws() - startDraws
	*originalRNG = workingRNG
	return result, nil
}
