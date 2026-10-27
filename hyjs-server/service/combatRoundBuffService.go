package service

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"hyjs-server/constUtil"
)

// combatRoundBuff owns its duration, hit/cast lifetime, stack and consumed shield.
// Property values are applied from the original frozen panel, never written back to it.
type combatRoundBuff struct {
	row                       map[string]string
	origin                    combatRoundBuffOrigin
	loadedRound               int32
	source, target            *combatRoundActor
	timer                     int32
	loaded                    float64
	rounds, attacks, defenses int32
	once, onceHit             bool
	suppressed                bool
	pendingEffect             bool
	originalPercentApplied    bool
	value                     float64
	initialValue              float64
	originalExportGroup       int64
}
type combatRoundBuffOrigin struct {
	talent, skill, cast string
}

type combatRoundTrigger struct {
	origin                           combatRoundBuffOrigin
	buff                             string
	timer, selectStrategy, condition int32
	loadTimer                        int32
	args, group                      string
	chance                           float64
}

func (a *combatRoundActor) attribute(key string) float64 {
	if key == "damageAdd" || key == "damageReduce" || key == "lifestealHp" {
		return a.originalFraction(key) * 100
	}
	if key == "blockDam" {
		return a.blockDam
	}
	base := a.base[key]
	rate, increment := 1.0, 0.0
	for _, b := range a.buffs {
		if b.suppressed || b.pendingEffect {
			continue
		}
		if p6OriginalFactoryNoEffect(b.row) {
			continue
		}
		f, x := b.row["Factor"], tableNumber(b.row, "Args1")
		// permaxHp is an immediate recovery Effect. Its retained identity does
		// not change maxHp's rate merely because the factor shares that prefix.
		if f == "permaxHp" {
			continue
		}
		// Original state effects contribute rates once per active poison/fire stack.
		if (f == "poisonDam" || f == "fireDam") && key == "att" {
			rate += .01
		}
		if f == "fireDam" && (key == "pdef" || key == "mdef") {
			rate -= .03
		}
		if f == "poisonDam" && key == "minusAddperhp" {
			increment -= 5
		}
		// Original plain ice adjusts hit/dodge/critical percentages without a stun.
		if f == "ice" || f == "iceYNT" {
			switch key {
			case "hit":
				increment += tableNumber(b.row, "Args1")
			case "dodge":
				increment += tableNumber(b.row, "Args2")
			case "cri":
				increment += tableNumber(b.row, "Args3")
			}
		}
		if f == "iceYNT" && (key == "damageAdd" || key == "treatAdd") {
			increment += tableNumber(parseP6Condition(b.row["Param"]), key)
		}
		if f == "counterStrikeRealDam" && key == "counterStrikeDam" {
			increment += x
		}
		switch {
		case f == "treatHurtAndAddperAtt" && key == "att":
			increment += b.value
		case f == "damageAddAndReduce" && key == "damageAdd":
			increment += x
		case f == "damageAddAndReduce" && key == "damageReduce":
			increment += tableNumber(b.row, "Args2")
		case f == "damageReduceChange" && key == "damageReduce":
			increment -= x
		case p6PropertyTransferKey(f) == key && key != "":
			increment += b.value
		case p6PropertyTransferKey(f) == "peratt" && key == "att":
			rate += b.value * .01
		case p6PropertyTransferKey(f) == "perdef" && (key == "pdef" || key == "mdef"):
			rate += b.value * .01
		case p6HPTransferKey(f) == key && key != "":
			increment += b.value
		case p6HPTransferKey(f) == "peratt" && key == "att":
			rate += b.value * .01
		case p6HPTransferKey(f) == "perdef" && (key == "pdef" || key == "mdef"):
			rate += b.value * .01
		case f == "blockDam" && key == "block":
			increment += x
		case f == key:
			increment += x
		case f == "def" && (key == "pdef" || key == "mdef"):
			increment += x
		case f == "per"+key:
			rate += x * .01
		case f == "perdef" && (key == "pdef" || key == "mdef"):
			rate += x * .01
		case f == "sourceperattToshield" && key == "shield", f == "sourceperattTopshield" && key == "pshield", f == "sourceperattTomshield" && key == "mshield":
			increment += b.value
		}
	}
	if key == "damageReduce" {
		for _, b := range a.outgoingReduction {
			if !b.suppressed {
				increment += tableNumber(b.row, "Args1")
			}
		}
	}
	return float64(base*rate) + increment
}
func (a *combatRoundActor) hasState(key string) bool {
	for _, b := range a.buffs {
		if !b.suppressed && !b.pendingEffect && (b.row["Factor"] == key || key == "invisible" && b.row["Factor"] == "reliveAndinvisible") {
			return true
		}
	}
	return false
}
func (a *combatRoundActor) shield(kind string) float64 {
	value := 0.0
	for _, b := range a.buffs {
		if !b.suppressed && !b.pendingEffect && b.shieldKind() == kind {
			value += math.Max(0, b.value)
		}
	}
	return value
}
func (b *combatRoundBuff) shieldKind() string {
	if p6OriginalFactoryNoEffect(b.row) {
		return ""
	}
	switch b.row["Factor"] {
	case "shield", "sourceperattToshield":
		return "shield"
	case "pshield", "sourceperattTopshield":
		return "pshield"
	case "mshield", "sourceperattTomshield":
		return "mshield"
	}
	return ""
}
func supportedP6Factor(f string) bool {
	switch f {
	case "additionAttBySelfRound", "MangerToNperhp", "MperhpreduceToNanger", "WeaponPerAtt", "treatHurtAndAddperAtt", "cripple", "damShareByMaxHpTeammate", "reboundPerMaxhp", "changeSkillBuff", "iceYNT", "sameDamForRandom", "dieAfterLastAttack", "damageAddAndReduce", "damageReduceChange", "angerChange", "angerChangeAndAddHp", "angerAndhp", "clearAllDebuff", "clearSomeBuffs", "permaxHpBySelfRound", "blockDam", "sourceNhitToperdef", "angerTocri", "comboNumTocri", "angerToperatt", "criTocriFactor", "sourcepermdefTomdef", "minusMaxHpPercent", "perhpTodamageAdd", "perhpTododge", "perhpTocri", "perhpToPeratt", "perhpToperdef", "counterStrikeDam", "counterStrikeRealDam", "reliveAndinvisible", "bleeding", "additionSourcepermaxHp", "additionAttDam", "additionAttRealDam", "angerRate", "ice", "uneffectBuffOnce", "randomBuff":
		return true
	case "delayCreateBuffs", "hp", "perhp", "permaxHp", "recover", "att", "peratt", "patt", "perpatt", "matt", "permatt", "def", "perdef", "pdef", "perpdef", "mdef", "permdef", "hit", "dodge", "cri", "antiCri", "criFactor", "damageAdd", "damageReduce", "pdamageReduce", "mdamageReduce", "treatAdd", "cure", "heal", "peranger", "anger", "lifestealHp", "minusDebuffOdds", "minusDebuffperOdds", "shield", "pshield", "mshield", "sourceperattToshield", "sourceperattTopshield", "sourceperattTomshield", "recoverBySelfRound", "minusSkillCoolDown", "clearBuff", "clearDebuff", "clearOneBuff", "clearOneDebuff", "invisible", "invincible", "mustHit", "mustCri", "dizzy", "forbidFirstSkill", "forbidSecondSkill", "forbidThirdSkill", "forbidUltimateSkill", "forbidAddSkillAnger", "forbidAddHp", "immuneDef", "immuneDizzy", "immuneDebuff", "immuneFloatBreak", "canNotKill", "poison", "antiPoison", "fire", "antiFire", "poisonDam", "fireDam", "minusAddperhp":
		return true
	}
	return false
}
func supportedP6Timer(timer int32) bool {
	switch timer {
	case 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 26, 27, 28, 34, 35:
		return true
	}
	return false
}
func parseP6Condition(args string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.Split(args, ",") {
		p := strings.SplitN(part, ":", 2)
		if len(p) == 2 {
			out[p[0]] = p[1]
		}
	}
	return out
}
func supportedP6Condition(kind int32, args string) bool {
	if kind == 0 {
		return true
	}
	if kind != 1 && kind != 2 && kind != 3 {
		return false
	}
	r := parseP6Condition(args)
	property := r["type"]
	if kind == 3 {
		// HasActorHpPercent locates the first named hero in the reference actor's camp.
		if !strings.HasPrefix(property, "hero_") {
			return false
		}
		property = r["type2"]
	}
	if (kind == 2 || kind == 3) && property != "hp" && property != "totalLeftHp" {
		return false
	}
	switch property {
	case "doskillNum", "anger", "skillClass", "aliveEnemyNum", "damageReduce", "hp", "maxHp", "att", "pdef", "mdef", "hit", "dodge", "cri", "antiCri", "curSkillType", "space", "row", "comboNum", "selfComboNum", "aliveNum", "selfRoundNum", "totalLeftHp", "totalMaxHp", "debuffNum", "bleeding", "speState":
	case "forbidFirstSkill", "forbidSecondSkill", "forbidThirdSkill", "forbidUltimateSkill", "forbidAddSkillAnger", "forbidAddHp", "fireDam", "poisonDam", "ice", "iceYNT", "invisible", "invincible", "mustHit", "mustCri", "dizzy", "immuneDef", "immuneDizzy", "immuneDebuff", "immuneFloatBreak", "canNotKill":
	default:
		return false
	}
	if kind == 1 && r["type2"] != "" {
		switch r["type2"] {
		case "doskillNum", "anger", "skillClass", "aliveEnemyNum", "damageReduce", "hp", "maxHp", "att", "pdef", "mdef", "hit", "dodge", "cri", "antiCri", "curSkillType", "space", "row", "comboNum", "selfComboNum", "aliveNum", "selfRoundNum", "totalLeftHp", "totalMaxHp", "debuffNum", "bleeding", "speState":
		case "forbidFirstSkill", "forbidSecondSkill", "forbidThirdSkill", "forbidUltimateSkill", "forbidAddSkillAnger", "forbidAddHp", "fireDam", "poisonDam", "ice", "iceYNT", "invisible", "invincible", "mustHit", "mustCri", "dizzy", "immuneDef", "immuneDizzy", "immuneDebuff", "immuneFloatBreak", "canNotKill":
		default:
			return false
		}
	}
	op := int32(tableNumber(r, "op"))
	return op >= 1 && op <= 6 && tableNumber(r, "src1") >= 1 && tableNumber(r, "src1") <= 3 && tableNumber(r, "src2") >= 1 && tableNumber(r, "src2") <= 3 && (op != 6 || tableNumber(r, "src2_val") != 0)
}
func (e *combatRoundEngine) validateDynamicTrigger(t combatRoundTrigger) error {
	return e.validateDynamicTriggerPath(t, map[string]bool{})
}

func (e *combatRoundEngine) validateDynamicTriggerPath(t combatRoundTrigger, path map[string]bool) error {
	if !supportedP6Timer(t.timer) || !supportedP6Condition(t.condition, t.args) || !combatFinite(t.chance) || t.chance < 0 {
		return fmt.Errorf("%s: trigger %s/%d", constUtil.CombatUnsupported, t.buff, t.timer)
	}
	if t.timer == 0 {
		return nil
	} // Forever bonuses have already been expanded by UIHero/P5.
	r := e.table("Buff", "Id", t.buff)
	if r == nil || !supportedP6Factor(r["Factor"]) {
		return fmt.Errorf("%s: buff %s/%s", constUtil.CombatUnsupported, t.buff, r["Factor"])
	}
	c := int32(tableNumber(r, "BuffClass")) % 10
	if c < 0 || c > 5 || (c == 4) != (r["Factor"] == "randomBuff") {
		return errors.New(constUtil.CombatUnsupported)
	}
	for _, k := range []string{"Args1", "Args2", "Args3", "TotalTime"} {
		if !combatFinite(tableNumber(r, k)) {
			return errors.New(constUtil.CombatInvalid)
		}
	}
	if (r["Factor"] == "counterStrikeDam" || r["Factor"] == "counterStrikeRealDam") && tableNumber(r, "Args2") != 2 {
		return errors.New(constUtil.CombatUnsupported)
	}
	if r["Factor"] == "MangerToNperhp" && (c != 1 || tableNumber(r, "Args1") <= 0 || tableNumber(r, "Args2") <= 0 || tableNumber(r, "Args1") > 2147483647) {
		return fmt.Errorf("%s: HP AP transfer factory/args %s", constUtil.CombatUnsupported, r["Id"])
	}
	if r["Factor"] == "MperhpreduceToNanger" && (c != 1 || tableNumber(r, "Args1") >= 0 || tableNumber(r, "Args2") <= 0 || tableNumber(r, "Args2") > 2147483647) {
		return fmt.Errorf("%s: loss AP transfer factory/args %s", constUtil.CombatUnsupported, r["Id"])
	}
	if r["Factor"] == "additionAttBySelfRound" && (c != 0 || tableNumber(r, "Args2") != 6 || math.Ceil(tableNumber(r, "Args3")) > constUtil.P6MaxEvents) {
		return fmt.Errorf("%s: round extra-attack factory/strategy/budget %s", constUtil.CombatUnsupported, r["Id"])
	}
	if r["Factor"] == "WeaponPerAtt" && !p6OriginalFactoryNoEffect(r) {
		return fmt.Errorf("%s: original inert weapon factory %s", constUtil.CombatUnsupported, r["Id"])
	}
	if (r["Factor"] == "treatHurtAndAddperAtt" || r["Factor"] == "cripple") && c != 0 {
		return fmt.Errorf("%s: wound/cripple factory %s", constUtil.CombatUnsupported, r["Id"])
	}
	if r["Factor"] == "treatHurtAndAddperAtt" && (tableNumber(r, "Args1") < 0 || tableNumber(r, "Args2") < 0) {
		return fmt.Errorf("%s: wound sign scope %s", constUtil.CombatUnsupported, r["Id"])
	}
	if r["Factor"] == "damShareByMaxHpTeammate" && c != 0 {
		return fmt.Errorf("%s: share factory %s", constUtil.CombatUnsupported, r["Id"])
	}
	if r["Factor"] == "reboundPerMaxhp" {
		if c != 0 || t.timer != 2 || t.selectStrategy != 130 || r["Param"] == "" || path[r["Id"]] || len(path) >= 64 {
			return fmt.Errorf("%s: teammate rebound factory/trigger %s", constUtil.CombatUnsupported, r["Id"])
		}
		path[r["Id"]] = true
		defer delete(path, r["Id"])
		for _, id := range strings.Split(r["Param"], ",") {
			if id == "" {
				return fmt.Errorf("%s: empty rebound template %s", constUtil.CombatUnsupported, r["Id"])
			}
			child := t
			child.buff, child.selectStrategy = id, 0
			if err := e.validateDynamicTriggerPath(child, path); err != nil {
				return err
			}
		}
	}
	if r["Factor"] == "iceYNT" && c != 2 {
		return fmt.Errorf("%s: iceYNT factory %s", constUtil.CombatUnsupported, r["Id"])
	}
	if r["Factor"] == "changeSkillBuff" {
		parts := strings.Split(r["Param"], ",")
		if c != 0 || len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" || e.table("Skill", "SkillId", parts[0]) == nil || e.table("Buff", "Id", parts[1]) == nil || path[r["Id"]] || len(path) >= 64 {
			return fmt.Errorf("%s: skill replacement %s", constUtil.CombatUnsupported, r["Id"])
		}
		path[r["Id"]] = true
		defer delete(path, r["Id"])
		child := t
		child.buff, child.timer = parts[2], 12
		if err := e.validateDynamicTriggerPath(child, path); err != nil {
			return err
		}
	}
	if r["Factor"] == "sameDamForRandom" && c != 0 {
		return fmt.Errorf("%s: random same-damage factory %s", constUtil.CombatUnsupported, r["Id"])
	}
	if r["Factor"] == "dieAfterLastAttack" && (c != 2 || t.timer != 24 || t.selectStrategy != 0) {
		return fmt.Errorf("%s: last-attack factory/trigger %s", constUtil.CombatUnsupported, r["Id"])
	}
	if r["Factor"] == "reliveAndinvisible" && (t.timer != 16 || t.selectStrategy != 0) {
		return errors.New(constUtil.CombatUnsupported)
	}
	if r["Factor"] == "minusMaxHpPercent" && (c != 0 || tableNumber(r, "Args2") < 0) {
		return fmt.Errorf("%s: max-HP-percent factory/cap %s", constUtil.CombatUnsupported, r["Id"])
	}
	if (r["Factor"] == "sourceperattToshield" || r["Factor"] == "sourceperattTopshield" || r["Factor"] == "sourceperattTomshield") && c != 1 && !p6OriginalFactoryNoEffect(r) {
		return fmt.Errorf("%s: source shield factory %s/%d", constUtil.CombatUnsupported, r["Id"], c)
	}
	if p6HPTransferKey(r["Factor"]) != "" && (c != 1 || tableNumber(r, "Args1") == 0) {
		return fmt.Errorf("%s: HP transfer factory/zero divisor %s", constUtil.CombatUnsupported, r["Id"])
	}
	if (r["Factor"] == "damageAddAndReduce" || r["Factor"] == "damageReduceChange") && c != 0 {
		return fmt.Errorf("%s: damage compound factory %s", constUtil.CombatUnsupported, r["Id"])
	}
	if (r["Factor"] == "angerChange" || r["Factor"] == "angerChangeAndAddHp" || r["Factor"] == "angerAndhp") && c != 0 {
		return fmt.Errorf("%s: anger compound factory %s", constUtil.CombatUnsupported, r["Id"])
	}
	if (r["Factor"] == "clearAllDebuff" || r["Factor"] == "clearSomeBuffs") && c != 3 {
		return fmt.Errorf("%s: dispel factory %s", constUtil.CombatUnsupported, r["Id"])
	}
	if r["Factor"] == "permaxHpBySelfRound" && (c != 0 || tableNumber(r, "Args2") < 0) {
		return fmt.Errorf("%s: round max-HP factory/cap %s", constUtil.CombatUnsupported, r["Id"])
	}
	if r["Factor"] == "blockDam" && c != 0 {
		return fmt.Errorf("%s: block factory %s", constUtil.CombatUnsupported, r["Id"])
	}
	if p6PropertyTransferKey(r["Factor"]) != "" && (c != 1 || p6PropertyTransferDivides(r["Factor"]) && tableNumber(r, "Args1") == 0) {
		return fmt.Errorf("%s: property transfer factory/zero divisor %s", constUtil.CombatUnsupported, r["Id"])
	}
	if r["Factor"] == "randomBuff" {
		if path[r["Id"]] || len(path) >= 64 {
			return fmt.Errorf("%s: cyclic randomBuff %s", constUtil.CombatUnsupported, r["Id"])
		}
		path[r["Id"]] = true
		defer delete(path, r["Id"])
		for _, id := range strings.Split(r["Param"], ",") {
			child := t
			child.buff = id
			if err := e.validateDynamicTriggerPath(child, path); err != nil {
				return err
			}
		}
	}
	return nil
}
func (e *combatRoundEngine) initializeDynamic(a *combatRoundActor) error {
	p := a.hero.GetProp()
	// Original BattleActorData/InActorData/FAttribute consume already-flat
	// attributes. BaseProp percentage fields belong to upstream UI union/rush
	// calculations and have no original engine reader; retain their source
	// metadata without multiplying the frozen panel or rejecting that metadata.
	a.base = map[string]float64{"maxHp": p.GetHp(), "att": p.GetAtt(), "pdef": p.GetPdef(), "mdef": p.GetMdef(), "hit": p.GetHit(), "dodge": p.GetDodge(), "cri": p.GetCri(), "antiCri": p.GetAntiCri(), "criFactor": p.GetCriFactor(), "damageAdd": p.GetDamageAdd(), "damageReduce": p.GetDamageReduce()}
	a.originalPercent = nil
	a.originalPercentBase = nil
	a.flags = map[string]bool{}
	a.immuneBuff = map[string]bool{}
	a.immuneTalent = map[string]bool{}
	for _, id := range a.hero.TalentList {
		r := e.table("Talent", "Id", id)
		if r == nil {
			return errors.New(constUtil.CombatUnsupported)
		}
		for _, group := range strings.Split(r["ImmuneBuffGroup"], ",") {
			if group != "" {
				a.immuneBuff[group] = true
			}
		}
		for _, group := range strings.Split(r["ImmuneTalentGroup"], ",") {
			if group != "" {
				a.immuneTalent[group] = true
			}
		}
		for i := 0; i < 4; i++ {
			key := fmt.Sprintf("Buff%d", i)
			if r[key] == "" {
				continue
			}
			t := combatRoundTrigger{origin: combatRoundBuffOrigin{talent: id}, buff: r[key], timer: int32(tableNumber(r, key+"TriggerTimer")), selectStrategy: int32(tableNumber(r, key+"Select")), condition: int32(tableNumber(r, key+"TriggerCondition")), args: r[key+"ConditionArgs"], chance: tableNumber(r, key+"Percent") * .01, group: r["Group"]}
			if err := e.validateDynamicTrigger(t); err != nil {
				return err
			}
			a.talents = append(a.talents, t)
		}
	}
	return nil
}
func (e *combatRoundEngine) validateCastBuffs(cast map[string]string) error {
	for i := 0; i < 3; i++ {
		n := strconv.Itoa(i)
		if cast["BufferId"+n] == "" {
			continue
		}
		class := int32(tableNumber(cast, "BuffTriggerClass"+n))
		if class < 0 || class > 3 {
			return errors.New(constUtil.CombatUnsupported)
		}
		t := combatRoundTrigger{buff: cast["BufferId"+n], timer: 12, selectStrategy: int32(tableNumber(cast, "Buff"+n+"SelStr")), condition: int32(tableNumber(cast, "Buff"+n+"TriggerCondition")), args: cast["Buff"+n+"ConditionArgs"], chance: tableNumber(cast, "BufferPercent"+n) * .01}
		if err := e.validateDynamicTrigger(t); err != nil {
			return err
		}
	}
	return nil
}
func (e *combatRoundEngine) conditionProperty(a *combatRoundActor, key string) float64 {
	switch key {
	case "doskillNum":
		// Native name means currently casting teammates, including ultimate and weak actors.
		n := 0.0
		for _, actor := range e.players[a.camp].actors {
			if actor.current != nil {
				n++
			}
		}
		return n
	case "anger":
		return e.players[a.camp].anger
	case "skillClass":
		return float64(a.curSkillClass)
	case "aliveEnemyNum":
		n := 0.0
		for _, actor := range e.players[1-a.camp].actors {
			if actor.state != 4 {
				n++
			}
		}
		return n
	case "damageReduce":
		// Original FAttribute stores percent properties as fractions for comparisons.
		return a.originalFraction("damageReduce")
	case "curSkillType":
		if a.current != nil {
			return float64(a.current.attackType)
		}
		return float64(a.attackType)
	case "speState":
		// Original GetValueByType returns FActor current Normal/Float/Break state.
		return float64(a.speState)
	case "bleeding":
		// Native FAttribute stores this percentage as a fraction for conditions.
		return a.attribute(key) * .01
	case "forbidFirstSkill", "forbidSecondSkill", "forbidThirdSkill", "forbidUltimateSkill", "forbidAddSkillAnger", "forbidAddHp", "fireDam", "poisonDam", "ice", "iceYNT", "invisible", "invincible", "mustHit", "mustCri", "dizzy", "immuneDef", "immuneDizzy", "immuneDebuff", "immuneFloatBreak", "canNotKill":
		// Native GetValueByType exposes active state counts, independent of Buff.Args1.
		n := 0.0
		for _, b := range a.buffs {
			if !b.suppressed && (b.row["Factor"] == key || key == "ice" && b.row["Factor"] == "iceYNT" || key == "invisible" && b.row["Factor"] == "reliveAndinvisible") {
				n++
			}
		}
		return n
	case "hp":
		return a.hp
	case "row":
		return float64((a.position()-1)/5 + 1)
	case "space":
		return tableNumber(e.table("Hero", "id", a.hero.GetId()), "space")
	case "comboNum":
		return float64(e.players[a.camp].combo)
	case "selfComboNum":
		return float64(a.selfCombo)
	case "selfRoundNum":
		// Original conditions use the global paired round, including actors yet to act.
		return float64((e.round + 1) / 2)
	case "aliveNum":
		n := 0.0
		for _, x := range e.players[a.camp].actors {
			if x.state != 4 {
				n++
			}
		}
		return n
	case "totalLeftHp":
		return e.totalHP(a.camp)
	case "totalMaxHp":
		v := 0.0
		for _, x := range e.players[a.camp].actors {
			v += x.attribute("maxHp")
		}
		return v
	case "debuffNum":
		n := 0.0
		for _, b := range a.buffs {
			if b.row["IsDebuff"] == "1" {
				n++
			}
		}
		return n
	}
	return a.attribute(key)
}
func (e *combatRoundEngine) conditionTrue(t combatRoundTrigger, owner, target *combatRoundActor) bool {
	if t.condition == 0 {
		return true
	}
	r := parseP6Condition(t.args)
	if t.condition == 3 {
		if target == nil {
			return false
		}
		named := (*combatRoundActor)(nil)
		for _, actor := range e.players[target.camp].actors {
			if actor.hero.GetId() == r["type"] {
				named = actor
				break
			}
		}
		if named == nil {
			return false
		}
		target = named
	}
	value := func(which string) float64 {
		source := int(tableNumber(r, "src"+which))
		if source == 1 {
			v := tableNumber(r, "src"+which+"_val")
			if t.condition == 2 || t.condition == 3 {
				return v * .01
			}
			return v
		}
		a := owner
		if source == 3 {
			a = target
		}
		if a == nil {
			return 0
		}
		key := r["type"]
		if t.condition == 3 || t.condition == 1 && which == "2" && r["type2"] != "" {
			key = r["type2"]
		}
		v := e.conditionProperty(a, key)
		if t.condition == 2 || t.condition == 3 {
			if key == "hp" {
				v /= a.attribute("maxHp")
			} else if key == "totalLeftHp" {
				v /= e.conditionProperty(a, "totalMaxHp")
			}
		}
		return v
	}
	a, b := value("1"), value("2")
	switch int(tableNumber(r, "op")) {
	case 1:
		return a > b
	case 2:
		return a >= b
	case 3:
		return a == b
	case 4, 5:
		return a <= b
	case 6:
		return b != 0 && math.Mod(a, b) == tableNumber(r, "op_val")
	}
	return false
}
func (e *combatRoundEngine) applyTrigger(owner, reference *combatRoundActor, t combatRoundTrigger, targets []*combatRoundActor) error {
	// A reactive talent owns its separate export even during delayed Buff Effect.
	valueParent := e.originalBuffValueDeath
	e.originalBuffValueDeath = nil
	defer func() { e.originalBuffValueDeath = valueParent }()
	roundBuff := e.originalRoundBuffDeath
	e.originalRoundBuffDeath = nil
	defer func() { e.originalRoundBuffDeath = roundBuff }()
	parent := e.originalStandaloneLoadParent
	group := e.originalStandaloneLoadGroup
	e.originalStandaloneLoadParent = nil
	if parent != nil || group == 0 {
		e.originalNewStandaloneLoadGroup()
		ownedGroup := e.originalStandaloneLoadGroup
		defer func() { e.originalCompleteStandaloneLoadGroup(ownedGroup) }()
	}
	defer func() { e.originalStandaloneLoadParent = parent; e.originalStandaloneLoadGroup = group }()
	if t.timer == 0 {
		return nil
	}
	row := e.table("Buff", "Id", t.buff)
	if row == nil {
		return errors.New(constUtil.CombatUnsupported)
	}
	castCollection := targets != nil
	if targets == nil {
		var err error
		// FBuffProc selects with LimitConditions(1,1,!isDebuff).
		targets, err = e.selectTargetsWithConditions(owner, t.selectStrategy, reference, nil, true, true, row["IsDebuff"] != "1")
		if err != nil {
			return err
		}
	}
	for _, a := range targets {
		if castCollection && (a.state == 4 || row["IsDebuff"] == "1" && a.hasState("invincible")) {
			continue
		}
		conditionTarget := a
		if t.selectStrategy == 0 {
			conditionTarget = reference
		}
		if !e.conditionTrue(t, owner, conditionTarget) {
			continue
		}
		if t.group != "" && a.immuneTalent[t.group] {
			continue
		}
		// LoadTalentBuff rejects friendly debuffs before isActive regardless of
		// an empty talent Group. Skill-origin Buffs keep their separate contract.
		if (t.origin.talent != "" || t.group != "") && a.camp == owner.camp && row["IsDebuff"] == "1" {
			continue
		}
		odds := t.chance
		if row["IsDebuff"] == "1" {
			odds = odds*(1+a.attribute("minusDebuffperOdds")*.01) + a.attribute("minusDebuffOdds")*.01
		}
		// Original isActive ALWAYS advances RNG, including 0%/100% and immune cover checks.
		if e.rng.Value() > odds {
			continue
		}
		loadTimer := t.loadTimer
		if loadTimer == 0 {
			loadTimer = t.timer
		}
		if err := e.loadBuffWithOrigin(owner, a, row, loadTimer, t.origin); err != nil {
			return err
		}
	}
	return nil
}
func (e *combatRoundEngine) talentTimer(a, reference *combatRoundActor, timer int32) error {
	return e.talentTimersLoaded(a, reference, timer, timer)
}

// Original trigger filters and loaded Buff.timer are distinct. The merged
// normal/ultimate handlers retain talent/slot order while passing one load timer.
func (e *combatRoundEngine) talentTimersLoaded(a, reference *combatRoundActor, loadTimer int32, timers ...int32) error {
	previousGroup := e.originalStandaloneLoadGroup
	previousParent := e.originalStandaloneLoadParent
	e.originalStandaloneLoadParent = nil
	defer func() { e.originalStandaloneLoadGroup = previousGroup; e.originalStandaloneLoadParent = previousParent }()
	currentTalent := ""
	defer func() {
		if currentTalent != "" {
			e.originalCompleteStandaloneLoadGroup(e.originalStandaloneLoadGroup)
		}
	}()
	for _, t := range a.talents {
		for _, timer := range timers {
			if t.timer == timer {
				if currentTalent != t.origin.talent {
					if currentTalent != "" {
						e.originalCompleteStandaloneLoadGroup(e.originalStandaloneLoadGroup)
					}
					currentTalent = t.origin.talent
					e.originalNewStandaloneLoadGroup()
				}
				t.loadTimer = loadTimer
				if err := e.applyTrigger(a, reference, t, nil); err != nil {
					return err
				}
				break
			}
		}
	}
	return nil
}

// LoadAttBuff handles AllAtt and the matching NormalAtt/UltimateAtt in
// original talent/slot order, with SAME_AS_TARGET spanning castTargets.
func (e *combatRoundEngine) castTalentTimers(skill *combatRoundSkill, reference *combatRoundActor, timers ...int32) error {
	for _, trigger := range skill.actor.talents {
		match := false
		for _, timer := range timers {
			if trigger.timer == timer {
				match = true
				break
			}
		}
		if !match {
			continue
		}
		trigger.loadTimer = 4
		var targets []*combatRoundActor
		if trigger.selectStrategy == 2 {
			targets = append([]*combatRoundActor{}, skill.targets...)
		}
		if err := e.applyTrigger(skill.actor, reference, trigger, targets); err != nil {
			return err
		}
	}
	return nil
}

func (e *combatRoundEngine) castBuffs(skill *combatRoundSkill, cast map[string]string, class, timer int32, reference *combatRoundActor) error {
	for i := 0; i < 3; i++ {
		n := strconv.Itoa(i)
		id := skill.originalCastBuffID(cast, n)
		if id == "" || int32(tableNumber(cast, "BuffTriggerClass"+n)) != class {
			continue
		}
		t := combatRoundTrigger{origin: combatRoundBuffOrigin{skill: skill.id, cast: cast["SkillCastId"]}, buff: id, timer: timer, selectStrategy: int32(tableNumber(cast, "Buff"+n+"SelStr")), condition: int32(tableNumber(cast, "Buff"+n+"TriggerCondition")), args: cast["Buff"+n+"ConditionArgs"], chance: tableNumber(cast, "BufferPercent"+n) * .01}
		var targets []*combatRoundActor
		// Only LoadAttBuff expands SAME_AS_TARGET to the cast target collection.
		// LoadHitBuff, LoadCriBuff and LoadAttEndBuff use their current reference.
		if class == 0 && t.selectStrategy == 2 {
			targets = skill.targets
		}
		if err := e.applyTrigger(skill.actor, reference, t, targets); err != nil {
			return err
		}
	}
	return nil
}
func (e *combatRoundEngine) loadBuff(source, target *combatRoundActor, row map[string]string, timer int32) error {
	return e.loadBuffWithOrigin(source, target, row, timer, combatRoundBuffOrigin{})
}

// Buff.LoadBuff keeps its actual Talent/SkillCast origin and original loaded round.
// Random clones and delayed child templates carry the same immutable origin identity.
func (e *combatRoundEngine) loadBuffWithOrigin(source, target *combatRoundActor, row map[string]string, timer int32, origin combatRoundBuffOrigin) error {
	if row["Factor"] == "reboundPerMaxhp" && row["Param"] == "" {
		return errors.New(constUtil.CombatUnsupported)
	}
	if row["Factor"] == "reboundPerMaxhp" && source.reboundUsed {
		return nil
	}
	e.effectDepth++
	defer func() { e.effectDepth-- }()
	if e.effectDepth > 64 || len(e.events) > constUtil.P6MaxEvents {
		return errors.New(constUtil.CombatInvalid)
	}
	if row["Factor"] == "randomBuff" {
		// Clone calls Range(0,Count-1), whose integer maximum is exclusive.
		// Preserve duplicate weights and the original unreachable final entry.
		choices := strings.Split(row["Param"], ",")
		index := int(e.rng.Value() * float64(len(choices)-1))
		child := e.table("Buff", "Id", choices[index])
		if child == nil {
			return errors.New(constUtil.CombatUnsupported)
		}
		e.trace("random_buff_choice", source, target, "", "", float64(index))
		e.events[len(e.events)-1].Buff = child["Id"]
		// Original Clone returns the selected child clone, never the wrapper.
		return e.loadBuffWithOrigin(source, target, child, timer, origin)
	}
	if target.immuneBuff[row["Group"]] || row["IsDebuff"] == "1" && target.hasState("immuneDebuff") || row["Factor"] == "dizzy" && target.hasState("immuneDizzy") {
		return nil
	}
	if row["Factor"] == "reliveAndinvisible" && target.revived {
		return nil
	}
	count := 0
	for _, b := range append([]*combatRoundBuff(nil), target.buffs...) {
		if row["ShouldCover"] == "1" && row["Group"] != "" && b.row["Group"] == row["Group"] {
			if b.shieldKind() != "" {
				if target.shield(b.shieldKind()) >= b.initialValue {
					return nil
				}
			} else if tableNumber(b.row, "Args1") >= tableNumber(row, "Args1") {
				return nil
			}
			e.unloadBuff(target, b)
			break
		}
		if b.row["Id"] == row["Id"] {
			count++
		}
	}
	if tableNumber(row, "TotalCount") > 0 && count >= int(tableNumber(row, "TotalCount")) {
		return nil
	}
	if int(tableNumber(row, "BuffClass"))/10 == 1 {
		if target.flags[row["Id"]] {
			return nil
		}
		target.flags[row["Id"]] = true
	}
	e.originalBuffSequence++
	b := &combatRoundBuff{originalExportGroup: e.originalBuffSequence, origin: origin, loadedRound: e.round, row: row, source: source, target: target, timer: timer, loaded: e.time, rounds: int32(tableNumber(row, "TotalRound")), attacks: int32(tableNumber(row, "AttCount")), defenses: int32(tableNumber(row, "DefCount"))}
	b.once = row["Factor"] != "delayCreateBuffs" && tableNumber(row, "TotalTime") <= 0 && b.rounds <= 0 && b.attacks <= 0 && b.defenses <= 0
	switch timer {
	case 7, 8, 9, 10, 11, 12, 13, 14, 15, 19, 23, 26, 29, 32, 34:
		b.onceHit = true
	}
	x := tableNumber(row, "Args1")
	b.value = x
	// FBuffManager inserts the identity before LoadDebuff and Effect. Recursive
	// healing/cover checks see it immediately; DataAttribute rates remain unchanged
	// until Effect, so derived property snapshots exclude this pending identity.
	b.pendingEffect = true
	target.buffs = append(target.buffs, b)
	defer func() { b.pendingEffect = false }()
	if row["IsDebuff"] == "1" {
		parentScope := e.originalCastScope
		e.originalCastScope = nil
		err := e.talentTimer(target, nil, 27)
		e.originalCastScope = parentScope
		if err != nil {
			return err
		}
	}
	switch row["Factor"] {
	case "MangerToNperhp", "MperhpreduceToNanger":
		e.originalHPAPEffect(target, row, 1)
	case "WeaponPerAtt":
		b.value = 0
		e.trace("native_buff_no_effect", source, target, "", "", 0)
		e.events[len(e.events)-1].Buff = row["Id"]
	case "treatHurtAndAddperAtt":
		e.originalWoundTreat(b)
	case "damShareByMaxHpTeammate":
		e.originalShareEffect(target, row)
	case "reboundPerMaxhp":
		b.pendingEffect = false
		e.trace("buff_load", source, target, "", "", x)
		e.events[len(e.events)-1].Buff = row["Id"]
		e.originalCastLoadedBuff(b)
		return e.originalReboundEffect(source, target, b)
	case "changeSkillBuff":
		e.originalChangeSkillBuff(target, row)
	case "damageReduceChange":
		e.attachReductionTransfer(b)
	case "angerChange", "angerChangeAndAddHp", "angerAndhp":
		var err error
		b.value, err = e.originalAngerCompoundEffect(source, target, row, b)
		if err != nil {
			return err
		}
	case "clearAllDebuff", "clearSomeBuffs", "clearBuff", "clearDebuff", "clearOneBuff", "clearOneDebuff":
		e.originalDispelEffect(target, row)
	case "permaxHpBySelfRound":
		b.value = e.roundMaxHPValue(source, target, row)
	case "blockDam":
		// Native block chance is additive; its damage modifier overwrites and survives UnEffect.
		target.blockDam = tableNumber(row, "Args2")
	case "sourceNhitToperdef", "angerTocri", "comboNumTocri", "angerToperatt", "criTocriFactor", "sourcepermdefTomdef":
		b.value = e.propertyTransferValue(source, target, row)
	case "minusMaxHpPercent":
		b.value = e.maxHPPercentEffect(source, target, row, b)
	case "perhpTodamageAdd", "perhpTododge", "perhpTocri", "perhpToPeratt", "perhpToperdef":
		// Native Effect freezes this staircase until its own UnEffect/reload.
		b.value = p6HPTransferValue(target, row)
	case "poisonDam", "fireDam":
		// BuffOfState freezes its magical periodic value before adding state rates.
		power, resistance := "poison", "antiPoison"
		if row["Factor"] == "fireDam" {
			power, resistance = "fire", "antiFire"
		}
		b.value = (source.attribute("att")+source.attribute("matt"))*.6 + source.attribute(power)*.01 - target.attribute(resistance)*.01
	case "counterStrikeDam", "counterStrikeRealDam":
		target.counterReal = row["Factor"] == "counterStrikeRealDam"
		target.counterSelect = int32(tableNumber(row, "Args2"))
	case "reliveAndinvisible":
		// DoDead already cleared old effects; original reset restores IDLE once per fight.
		target.state = 0
		target.hpLossAccumulator = 0
		target.hpChangeInRound = 0
		target.revived = true
		target.current = nil
		target.waiting = nil
		target.speState = 1
		target.speUntil = 0
		b.value = target.attribute("maxHp") * x * .01
		e.originalBuffChangeHP(source, target, b.value, "revive", 17, b)
	case "additionAttDam", "additionAttRealDam":
		if err := e.additionalAttackDamage(source, target, row, b); err != nil {
			return err
		}
	case "additionSourcepermaxHp":
		// Native BuffOfProperty applies source max HP immediately with source damage type.
		b.value = source.attribute("maxHp") * x * .01
		if b.value > 0 && target.state < 4 {
			e.originalBuffDamageHP(source, target, b.value, source.attackType, 8, b)
		} else if b.value < 0 {
			e.originalBuffChangeHP(source, target, -b.value, "buff_heal", 8, b)
		}
		e.trace("source_maxhp_damage", source, target, "", "", b.value)
	case "sourceperattToshield", "sourceperattTopshield", "sourceperattTomshield":
		if p6OriginalFactoryNoEffect(row) {
			b.value = 0
			e.trace("native_buff_no_effect", source, target, "", "", 0)
			e.events[len(e.events)-1].Buff = row["Id"]
		} else {
			b.value = source.attribute("att") * x * .01
		}
	case "recoverBySelfRound":
		// Original Effect freezes the source attack based heal once at load time.
		b.value = (source.attribute("att")+source.attribute("patt"))*x*.01*(1+float64(source.attribute("treatAdd")*.01)) + source.attribute("cure") + target.attribute("heal")
	case "hp":
		e.originalBuffChangeHP(source, target, x, "buff_heal", 15, b)
	case "perhp":
		b.value = target.hp * x * .01
		e.originalBuffChangeHP(source, target, b.value, "buff_heal", 15, b)
	case "permaxHp":
		b.value = target.attribute("maxHp") * x * .01
		e.originalBuffChangeHP(source, target, b.value, "buff_heal", 15, b)
	case "recover":
		e.originalRecover(source, target, row, b)
	case "anger":
		e.originalBuffAnger(source, target, x, b)
	case "minusSkillCoolDown":
		// BuffOfProperty.Effect1a30 selects the pending/current ultimate,
		// otherwise current normal, with index0 as the idle fallback. It
		// subtracts Args1 directly and keeps negative cooldown until RoundEnd.
		index := int32(0)
		if e.pendingUltimate != nil && e.pendingUltimate.actor == target {
			index = e.pendingUltimate.index
		} else if target.current != nil {
			index = target.current.index
		}
		for _, skill := range target.skills {
			if skill.index == index {
				skill.cooldown -= int32(x)
			}
		}
	}
	e.originalPercentEffect(b, true)
	b.initialValue = b.value
	e.trace("buff_load", source, target, "", "", x)
	e.events[len(e.events)-1].Buff = row["Id"]
	e.originalCastLoadedBuff(b)
	return nil
}
func (e *combatRoundEngine) unloadBuff(a *combatRoundActor, b *combatRoundBuff) {
	for i, x := range a.buffs {
		if x == b {
			e.originalPercentEffect(b, false)
			if b.row["Factor"] == "MangerToNperhp" || b.row["Factor"] == "MperhpreduceToNanger" {
				e.originalHPAPEffect(a, b.row, -1)
			}
			if b.row["Factor"] == "damShareByMaxHpTeammate" {
				a.shareFactor, a.shareActors = 0, nil
			}
			if b.row["Factor"] == "damageReduceChange" {
				e.detachReductionTransfer(b)
			}
			if kind := b.shieldKind(); kind != "" {
				// Native shield UnEffect clears the whole typed attribute. Other
				// shield Buff identities remain, with zero remaining contribution.
				for _, other := range a.buffs {
					if other.shieldKind() == kind {
						other.value = 0
					}
				}
			}
			a.buffs = append(a.buffs[:i], a.buffs[i+1:]...)
			if b.row["Factor"] == "counterStrikeDam" || b.row["Factor"] == "counterStrikeRealDam" {
				a.counterReal = false
			}
			e.trace("buff_unload", b.source, a, "", "", 0)
			e.events[len(e.events)-1].Buff = b.row["Id"]
			e.originalBuffUnloaded(b)
			return
		}
	}
}
func (e *combatRoundEngine) clearOnce(a *combatRoundActor, hit bool) {
	for i := len(a.buffs) - 1; i >= 0; i-- {
		b := a.buffs[i]
		if b.once && b.onceHit == hit {
			e.unloadBuff(a, b)
		}
	}
}
func (e *combatRoundEngine) updateBuffs(a *combatRoundActor) {
	for i := len(a.buffs) - 1; i >= 0; i-- {
		b := a.buffs[i]
		if b.row["Factor"] == "delayCreateBuffs" && tableNumber(b.row, "Args1") > 0 && e.time-b.loaded >= tableNumber(b.row, "Args1") {
			if err := e.originalDelayBuffEffect(b, false); err != nil {
				e.failure = err
				return
			}
			e.unloadBuff(a, b)
			continue
		}
		if tableNumber(b.row, "TotalTime") > 0 && e.time-b.loaded >= tableNumber(b.row, "TotalTime") {
			e.unloadBuff(a, b)
		}
	}
}

// BuffOfDelayCreate.Effect trims a timed child's remaining rounds when the
// containing round ends early. A round-delayed child keeps its original count.
func (e *combatRoundEngine) originalDelayBuffEffect(b *combatRoundBuff, roundEnd bool) error {
	parent := e.originalStandaloneLoadParent
	group := e.originalStandaloneLoadGroup
	e.originalStandaloneLoadParent = b
	e.originalNewStandaloneLoadGroup()
	startSnapshots := len(e.originalBuffSnapshots)
	defer func() { e.originalStandaloneLoadParent = parent; e.originalStandaloneLoadGroup = group }()
	for _, id := range strings.Split(b.row["Param"], ",") {
		row := e.table("Buff", "Id", id)
		if row == nil {
			return errors.New(constUtil.CombatUnsupported)
		}
		if roundEnd && tableNumber(row, "TotalRound") > 0 {
			remaining := int32(tableNumber(row, "TotalRound")) - 1
			if remaining <= 0 && tableNumber(row, "AttCount") <= 0 && tableNumber(row, "DefCount") <= 0 {
				continue
			}
			copyRow := map[string]string{}
			for key, value := range row {
				copyRow[key] = value
			}
			copyRow["TotalRound"] = strconv.Itoa(int(remaining))
			row = copyRow
		}
		if err := e.loadBuffWithOrigin(b.source, b.target, row, b.timer, b.origin); err != nil {
			return err
		}
	}
	loaded := false
	for _, snapshot := range e.originalBuffSnapshots[startSnapshots:] {
		if snapshot.parent == b {
			loaded = true
			break
		}
	}
	if !loaded {
		e.originalEmptyBuffExport(b)
	} else {
		e.originalCompleteStandaloneLoadGroup(e.originalStandaloneLoadGroup)
	}
	return nil
}

// Original FBuffManager.RoundEnd uses a live forward cursor. A class5 helper
// can unload itself while returning false; only base round/time expiry backs
// the cursor up, preserving the native mutation and newly loaded child order.
func (e *combatRoundEngine) originalBuffRoundEnd(a *combatRoundActor) error {
	for index := 0; index < len(a.buffs); index++ {
		b := a.buffs[index]
		if b.row["Factor"] == "delayCreateBuffs" {
			if tableNumber(b.row, "Args2") > 0 && float64(e.round-b.loadedRound) >= tableNumber(b.row, "Args2") {
				if err := e.originalDelayBuffEffect(b, false); err != nil {
					return err
				}
				e.unloadBuff(a, b)
			}
			if tableNumber(b.row, "Args1") > 0 {
				if err := e.originalDelayBuffEffect(b, true); err != nil {
					return err
				}
				e.unloadBuff(a, b)
			}
		}
		if b.rounds > 0 {
			b.rounds--
			if b.rounds == 0 {
				e.unloadBuff(a, b)
				index--
			}
		} else if tableNumber(b.row, "TotalTime") > 0 {
			e.unloadBuff(a, b)
			index--
		}
	}
	return nil
}
func (e *combatRoundEngine) consumeBuffCounts(a *combatRoundActor, attack bool) {
	for i := len(a.buffs) - 1; i >= 0; i-- {
		b := a.buffs[i]
		if attack && b.attacks > 0 {
			b.attacks--
			if b.attacks == 0 {
				e.unloadBuff(a, b)
			}
		} else if !attack && b.defenses > 0 {
			b.defenses--
			if b.defenses == 0 {
				e.unloadBuff(a, b)
			}
		}
	}
}
func (e *combatRoundEngine) changeHP(source, target *combatRoundActor, change float64, kind string) float64 {
	actual, _, _ := e.changeHPWithOriginalExport(source, target, change, kind)
	return actual
}

// The original HP export keeps nominal post-poison/post-shield change even
// when max HP clamps recovery or a weak actor skips mutation.
func (e *combatRoundEngine) changeHPWithOriginalExport(source, target *combatRoundActor, change float64, kind string) (float64, float64, bool) {
	// Native AddHp skips HP and shield mutation at weak/dead states; revival resets state first.
	if change == 0 || target.state == 4 {
		return 0, 0, false
	}
	if target.state == 3 || target.hasState("dieAfterLastAttack") {
		return 0, change, true
	}
	if change > 0 && target.hasState("forbidAddHp") {
		return 0, 0, true
	}
	if change > 0 {
		// Native AddHp applies poison's recovery penalty, then a minimum positive heal.
		change = math.Max(1, change*(1+target.attribute("minusAddperhp")*.01))
	}
	before := target.hp
	negativeInput := change < 0
	shielded := false
	spent := map[string]bool{}
	if change < 0 {
		// Original FActor.AddHp 012c–014e uses the recipient's enemy camp,
		// including periodic/self-origin damage, before any shield consumption.
		remaining := -change * FrozenP6DamageFactor(e.input, 1-target.camp)
		for _, shieldKind := range []string{"shield", "pshield", "mshield"} {
			if kind != "damage" && shieldKind != "shield" {
				continue
			}
			for _, b := range target.buffs {
				if !b.suppressed && b.shieldKind() == shieldKind && b.value > 0 {
					shielded = true
					used := math.Min(b.value, remaining)
					b.value -= used
					remaining -= used
					if used > 0 {
						spent[shieldKind] = true
					}
				}
			}
			if remaining == 0 {
				break
			}
		}
		change = -remaining
	}
	// An original negative AddHp calls MinusHP even after a zero multiplier,
	// unless an existing matching shield takes the shield-only branch.
	if negativeInput && (change < 0 || !shielded) {
		change = e.originalMinusHP(target, change)
	}
	target.hp = math.Min(target.attribute("maxHp"), target.hp+change)
	if change > 0 || change < 0 && target.hp > 0 {
		e.originalHPChangeInRound(target, change)
	}
	if change < 0 && target.hp > 0 {
		e.originalLossAnger(target, change)
	}
	if target.hp <= 0 && before > 0 {
		e.originalWillDie(target)
	}
	target.hp = math.Max(0, target.hp)
	actual := target.hp - before
	e.trace(kind, source, target, "", "", actual)
	e.originalUnloadSpentShields(target, spent)
	if target.hp <= 0 && before > 0 {
		target.state = 3
		target.deathSource = source
		e.trace("weak_dead", target, nil, "", "", 0)
		if err := e.finishPendingDeaths(); err != nil {
			e.failure = err
		}
	}
	return actual, change, true
}

// dynamicRelease keeps the original hit→hit Buff→critical→variance→block order.
func (e *combatRoundEngine) dynamicRelease(skill *combatRoundSkill, cast map[string]string) error {
	a := skill.actor
	if a.state == 4 {
		return nil
	}
	a.curSkillClass = int32(tableNumber(skill.row, "SkillClass"))
	if skill.target == nil {
		return nil
	}
	previousScope := e.originalCastScope
	e.originalCastScope = &combatRoundHPExportOrigin{source: a, skill: skill.id, cast: cast["SkillCastId"]}
	defer func() { e.originalCastScope = previousScope }()
	ultimate := skill.row["IsUltimate"] == "1"
	firstRelease := !skill.first && cast["SkillCastId"] == skill.firstCast
	if firstRelease {
		skill.first = true
		skill.cooldown += skill.maxCooldown
		if ultimate {
			e.addAnger(a, -e.number("UTIL_TRIGGER_ANGERCOUNT"))
		}
	}
	// FSkill.Do already selected the first castTargets. FRelease.Do 0184/01c8
	// skips reselection for that first cast; 01d9 also retains the complete prior
	// collection for SAME_AS_LAST_RELEASE, including its weak/dead members.
	targets := skill.targets
	var err error
	if !firstRelease && int32(tableNumber(cast, "SelectStrategy")) != 1 {
		targets, err = e.selectTargetsWithWeak(a, int32(tableNumber(cast, "SelectStrategy")), skill.target, skill.targets, true)
		if err != nil {
			return err
		}
	}
	skill.targets = targets
	if err = e.castBuffs(skill, cast, 0, 4, skill.target); err != nil {
		return err
	}
	attackTimer := int32(5)
	if ultimate {
		attackTimer = 6
	}
	if err = e.castTalentTimers(skill, skill.target, 4, attackTimer); err != nil {
		return err
	}
	typ := int32(tableNumber(cast, "ExportType"))
	exportOrigin := combatRoundHPExportOrigin{source: a, skill: skill.id, cast: cast["SkillCastId"], cursor: new(int64)}
	// Native FSkill.init subtracts one without clamping: original level0 contributes -1.
	// FSkillCast stores the scaled base before adding breach growth. Preserve
	// that intermediate rounding on ARM64; cast674 distinguishes FMA by1ULP.
	factor := float64(tableNumber(cast, "ExportFactor")*.01) + float64(a.hero.GetBreachedLevel()-1)*tableNumber(skill.row, "LevelGrowFactor")*.01/float64(len(skill.casts))
	if factor == 0 {
		// The original no-damage branch still emits HP=0 for each retained target.
		// It bypasses hit/critical/property effects and consumes no formula draws.
		for _, target := range targets {
			if target.state == 4 {
				continue
			}
			nominal, hpType := 0.0, int32(0)
			e.trace("hp_export", a, target, skill.id, cast["SkillCastId"], 0)
			e.events[len(e.events)-1].OriginalHPValue = &nominal
			e.events[len(e.events)-1].OriginalHPType = &hpType
			e.events[len(e.events)-1].OriginalValueOrder = exportOrigin.nextOrder()
		}
		return nil
	}
	for _, target := range targets {
		if target.state == 4 {
			continue
		}
		if err = e.talentTimer(target, a, 26); err != nil {
			return err
		}
		hit := a.attribute("hit")*.01 - target.attribute("dodge")*.01
		if target.speState == 2 || target.speState == 3 {
			hit = 1 - (1-hit)*e.number("FloatBreakDodgeFactor")
		}
		if ultimate || typ == 3 || a.hasState("mustHit") {
			hit = 1
		}
		miss := e.rng.Value() > hit
		if miss {
			e.trace("release", a, target, skill.id, cast["SkillCastId"], 0)
			e.events[len(e.events)-1].Miss = true
			nominal, hpType := 0.0, int32(2)
			e.events[len(e.events)-1].OriginalHPValue = &nominal
			e.events[len(e.events)-1].OriginalHPType = &hpType
			e.events[len(e.events)-1].OriginalValueOrder = exportOrigin.nextOrder()
			if err = e.talentTimer(target, a, 11); err != nil {
				return err
			}
			e.clearOnce(a, true)
			e.clearOnce(target, true)
			continue
		}
		if err = e.castBuffs(skill, cast, 1, 12, target); err != nil {
			return err
		}
		if err = e.talentTimer(a, target, 12); err != nil {
			return err
		}
		if typ == 3 {
			if err = e.talentTimer(target, a, 10); err != nil {
				return err
			}
		} else {
			hurtTimer := int32(8)
			if ultimate {
				hurtTimer = 9
			}
			if err = e.talentTimersLoaded(target, a, 7, 7, hurtTimer); err != nil {
				return err
			}
		}
		critical, blocked := false, false
		damage := 0.0
		nominalHP, hpType := 0.0, int32(0)
		hpVisible, shieldExport := false, false
		var primaryOrder int64
		var suppressed []*combatRoundBuff
		if typ == 3 {
			critical = a.hasState("mustCri") || e.rng.Value() <= a.attribute("cri")*.01
			multiplier := 1.0
			if critical {
				multiplier += a.attribute("criFactor") * .01
			}
			heal := (a.attribute("att")*factor*(1+float64(a.attribute("treatAdd")*.01)) + a.attribute("cure") + target.attribute("heal")) * (1 + e.rng.Range(-.1, .1)) * multiplier
			if target.hasState("forbidAddHp") {
				hpType = 6
			}
			_, nominalHP, hpVisible = e.changeHPWithOriginalExport(a, target, heal, "skill_heal")
			if hpVisible {
				primaryOrder = exportOrigin.nextOrder()
			}
		} else if typ == 1 || typ == 2 {
			if !target.hasState("invincible") {
				if target.hasState("uneffectBuffOnce") {
					var suppressErr error
					suppressed, suppressErr = e.suppressBuffsOnce(target)
					if suppressErr != nil {
						return suppressErr
					}
				}
				critical = a.hasState("mustCri") || e.rng.Value() <= a.attribute("cri")*.01-target.attribute("antiCri")*.01
				if critical {
					if err = e.castBuffs(skill, cast, 2, 13, target); err != nil {
						return err
					}
					if err = e.talentTimer(a, target, 13); err != nil {
						return err
					}
					if err = e.talentTimer(target, a, 14); err != nil {
						return err
					}
					for _, mate := range e.players[target.camp].actors {
						if mate != target && mate.hp > 0 {
							if err = e.talentTimer(mate, target, 15); err != nil {
								return err
							}
						}
					}
				}
				// Original noncritical branch jumps to 0787 too, so every typed hit refreshes
				// float/fall state before special-state AP, magic stone and water-ball draws.
				duration := e.specialHit(a, target, int32(tableNumber(cast, "CauseSpeState")))
				state := float64(1)
				if duration > 0 {
					state = float64(target.speState)
				}
				e.originalCastValueExport(target, 4, state, duration, exportOrigin)
				if target.speState == 3 {
					if err = e.magicStone(a, target, combatRoundBuffOrigin{skill: skill.id, cast: cast["SkillCastId"]}, exportOrigin); err != nil {
						return err
					}
				}
				// Floating hits draw before damage variance; broken hits use the magic-stone path.
				if target.speState == 2 && (a.camp == 0 || e.input.Defender != nil) {
					if e.rng.Value() <= 0.15000000596046448 {
						e.trace("water_ball", a, target, skill.id, cast["SkillCastId"], 1)
						e.events[len(e.events)-1].OriginalValueClass = 6
						e.events[len(e.events)-1].OriginalValueOrder = exportOrigin.nextOrder()
						e.events[len(e.events)-1].OriginalValueSource = target.uid
						e.events[len(e.events)-1].OriginalValueTarget = a.uid
					}
				}
				defense, extraAtt, reduce := target.attribute("pdef"), a.attribute("patt"), target.attribute("pdamageReduce")
				if typ == 2 {
					defense, extraAtt, reduce = target.attribute("mdef"), a.attribute("matt"), target.attribute("mdamageReduce")
				}
				if a.hasState("immuneDef") {
					defense, reduce = 0, 0
				}
				combo := 0.0
				if n := e.number("combohitAddNum"); n > 0 {
					combo = math.Floor(float64(e.players[a.camp].combo)/n) * e.number("combohitdamageFactor") * .01
				}
				multiplier := math.Max(0, 1+a.originalFraction("damageAdd")+combo-target.originalFraction("damageReduce")-reduce*.01)
				if a.hasState("immuneDef") {
					multiplier = math.Max(0, 1+a.originalFraction("damageAdd")+combo)
				}
				cri := 1.0
				if critical {
					cri += a.attribute("criFactor") * .01
				}
				power := a.attribute("att") + extraAtt
				share := 1.0
				if len(target.shareActors) > 0 {
					if err := e.originalSharedDamage(skill, target, typ, factor, power, a.originalFraction("damageAdd")+combo, exportOrigin); err != nil {
						return err
					}
					share = 1 - target.shareFactor
				}
				damage = (power - defense) * factor * share * multiplier * (1 + e.rng.Range(-.1, .1)) * cri
				if !a.hasState("mustHit") && target.attribute("dodge") > a.attribute("hit") {
					damage *= 1 - e.rng.Value()*math.Min(.5, (target.attribute("dodge")-a.attribute("hit"))*.01)*2
				}
				damage *= e.specialFactor(int32(tableNumber(cast, "AdvSpeState")), target.speState, "HP")
				if e.rng.Value() <= target.attribute("block")*.01 {
					blocked = true
					damage *= 1 + target.attribute("blockDam")*.01
					e.trace("block", a, target, skill.id, cast["SkillCastId"], damage)
					if err = e.talentTimer(target, a, 23); err != nil {
						return err
					}
				}
				damage = math.Max(1, damage)
				if a.hasState("canNotKill") {
					damage = math.Min(damage, math.Max(0, target.hp-1))
				}
				// Typed shields are applied only to their matching damage kind.
				nominalHP, hpVisible, shieldExport = e.damageHP(a, target, damage, typ)
				if hpVisible {
					primaryOrder = exportOrigin.nextOrder()
				}
				if err := e.originalSameDamage(skill, target, damage, typ, exportOrigin); err != nil {
					return err
				}
				if !ultimate {
					if err := e.counterStrike(target, a, damage, exportOrigin); err != nil {
						return err
					}
					if err := e.bleedingHit(a, target, exportOrigin); err != nil {
						return err
					}
				}
				if a.attribute("lifestealHp") > 0 {
					selectStrategy := int32(0)
					for _, b := range a.buffs {
						if !b.suppressed && b.row["Factor"] == "lifestealHp" {
							selectStrategy = int32(tableNumber(b.row, "Args2"))
						}
					}
					recipients, err := e.selectTargets(a, selectStrategy, skill.target, nil)
					if err != nil {
						return err
					}
					for _, recipient := range recipients {
						kind := int32(13)
						if recipient.hasState("forbidAddHp") {
							kind = 6
						}
						_, nominal, visible := e.changeHPWithOriginalExport(a, recipient, originalLifestealAmount(damage, a.originalFraction("lifestealHp"), a.attribute("treatAdd"), a.attribute("cure"), recipient.attribute("heal")), "lifesteal")
						e.originalExtraHPExport(recipient, nominal, visible, false, kind, []combatRoundHPExportOrigin{exportOrigin})
					}
				}
			}
		}
		if typ == 0 {
			// Original effect-only casts still export an explicit zero HP value.
			hpVisible = true
		}
		if (typ == 1 || typ == 2) && target.hasState("invincible") && !hpVisible {
			hpVisible, hpType = true, 1
		}
		if hpType == 0 {
			if critical {
				hpType = 3
			}
			if blocked {
				hpType = 4
				if critical {
					hpType = 5
				}
			}
		}
		if shieldExport {
			hpType = 16
		}
		if !combatFinite(damage) {
			return errors.New(constUtil.CombatInvalid)
		}
		e.trace("release", a, target, skill.id, cast["SkillCastId"], damage)
		e.events[len(e.events)-1].Critical = critical
		if hpVisible {
			e.events[len(e.events)-1].OriginalHPValue = &nominalHP
			e.events[len(e.events)-1].OriginalHPType = &hpType
			if primaryOrder == 0 {
				primaryOrder = exportOrigin.nextOrder()
			}
			e.events[len(e.events)-1].OriginalValueOrder = primaryOrder
		}

		if !a.hasState("forbidAddSkillAnger") {
			factor := 1.0
			if typ != 3 {
				factor = e.specialFactor(int32(tableNumber(cast, "AdvSpeState")), target.speState, "AP")
			}
			e.originalCastAnger(a, tableNumber(cast, "AddAnger")*(factor+a.attribute("peranger")*.01), exportOrigin)
		}
		if typ != 3 {
			e.originalCastAnger(target, tableNumber(cast, "AddAnger")*(target.attribute("peranger")*.01+e.number("BeHitAddAngerPercent")*.01), exportOrigin)
		}
		if err := e.restoreBuffsOnce(target, suppressed); err != nil {
			return err
		}
		e.consumeBuffCounts(a, false)
		e.consumeBuffCounts(target, false)
		e.clearOnce(a, true)
		e.clearOnce(target, true)
		e.originalAddCombo(a, target, int32(tableNumber(cast, "AdvSpeState")))
	}
	// FRelease.Do finishes attack counts/once-cast effects and cripple before
	// resolving the last cast's weak deaths and loading attack-end triggers.
	e.consumeBuffCounts(a, true)
	e.clearBothCampsOnceCast(a.camp)
	if err := e.originalCrippleCastEnd(a); err != nil {
		return err
	}
	lastRelease := skill.releaseIndex == len(skill.releases)-1
	if lastRelease || a.state == 3 {
		skill.sameDamTargets = nil
		if err := e.finishReleaseDeaths(skill); err != nil {
			return err
		}
	}
	e.originalCastValueExport(a, 3, float64(e.players[a.camp].combo), 0, exportOrigin)
	if a.state == 3 {
		return e.completeActorDeath(a)
	}
	if err = e.castBuffs(skill, cast, 3, 20, skill.target); err != nil {
		return err
	}
	endTimer := int32(21)
	if ultimate {
		endTimer = 22
	}
	if err = e.talentTimersLoaded(a, skill.target, 20, 20, endTimer); err != nil {
		return err
	}
	return nil
}

// The original engine stores lifesteal as an accumulated fraction, including
// Effect/UnEffect rounding residue. Consume that fraction without a percent round trip.
func originalLifestealAmount(damage, fraction, treatAdd, cure, heal float64) float64 {
	return damage*fraction*(1+float64(treatAdd*.01)) + cure + heal
}

func (e *combatRoundEngine) damageHP(source, target *combatRoundActor, damage float64, typ int32) (float64, bool, bool) {
	return e.damageHPKind(source, target, damage, typ, "damage", source)
}

// Effect kind controls the recorded event; AddHp separately consumes shields by export type.
func (e *combatRoundEngine) damageHPKind(source, target *combatRoundActor, damage float64, typ int32, kind string, killer *combatRoundActor) (float64, bool, bool) {
	// Native AddHp leaves weak-dead HP and shields untouched; release triggers still continue.
	if damage == 0 || target.state == 4 {
		return 0, false, false
	}
	if target.state == 3 || target.hasState("dieAfterLastAttack") {
		return -damage, true, false
	}
	remaining := damage * FrozenP6DamageFactor(e.input, 1-target.camp)
	spent := map[string]bool{}
	typed := "pshield"
	if typ == 2 {
		typed = "mshield"
	}
	shielded := target.shield(typed) > 0 || target.shield("shield") > 0
	for _, kind := range []string{typed, "shield"} {
		for _, b := range target.buffs {
			if !b.suppressed && b.shieldKind() == kind && b.value > 0 {
				used := math.Min(b.value, remaining)
				b.value -= used
				remaining -= used
				if used > 0 {
					spent[kind] = true
				}
			}
		}
	}
	before := target.hp
	delta := 0.0
	if remaining > 0 || !shielded {
		delta = e.originalMinusHP(target, -remaining)
	}
	target.hp += delta
	if delta < 0 && target.hp > 0 {
		e.originalHPChangeInRound(target, delta)
	}
	if delta < 0 && target.hp > 0 {
		e.originalLossAnger(target, delta)
	}
	if target.hp <= 0 && before > 0 {
		e.originalWillDie(target)
	}
	target.hp = math.Max(0, target.hp)
	e.trace(kind, source, target, "", "", target.hp-before)
	e.originalUnloadSpentShields(target, spent)
	if target.hp <= 0 && before > 0 {
		target.state = 3
		target.deathSource = killer
		e.trace("weak_dead", target, nil, "", "", 0)
		if err := e.finishPendingDeaths(); err != nil {
			e.failure = err
		}
	}
	return delta, true, shielded && remaining == 0
}

// Native AddHp removes spent typed/common shield identities in every negative
// path, including immediate negative perhp/permaxHp property effects.
func (e *combatRoundEngine) originalUnloadSpentShields(target *combatRoundActor, spent map[string]bool) {
	for _, b := range append([]*combatRoundBuff(nil), target.buffs...) {
		if spent[b.shieldKind()] && target.shield(b.shieldKind()) <= 0 && !b.suppressed {
			e.unloadBuff(target, b)
		}
	}
}

func (e *combatRoundEngine) deathTriggers(source, target *combatRoundActor) {
	for _, pair := range []struct {
		actor, reference *combatRoundActor
		timer            int32
	}{{target, source, 16}, {source, target, 18}} {
		if pair.actor == nil {
			continue
		}
		if err := e.talentTimer(pair.actor, pair.reference, pair.timer); err != nil {
			e.failure = err
			return
		}
	}
	for _, p := range e.players {
		for _, a := range p.actors {
			if a.hp <= 0 {
				continue
			}
			timer := int32(28)
			if a.camp == target.camp {
				timer = 17
			}
			if err := e.talentTimer(a, target, timer); err != nil {
				e.failure = err
				return
			}
		}
	}
}

// These original class1 transfers snapshot target HP only when Effect runs.
func p6HPTransferKey(factor string) string {
	switch factor {
	case "perhpTodamageAdd":
		return "damageAdd"
	case "perhpTododge":
		return "dodge"
	case "perhpTocri":
		return "cri"
	case "perhpToPeratt":
		return "peratt"
	case "perhpToperdef":
		return "perdef"
	}
	return ""
}

func p6HPTransferValue(target *combatRoundActor, row map[string]string) float64 {
	value := math.Floor((target.hp/target.attribute("maxHp")-1)/(tableNumber(row, "Args1")*.01)) * tableNumber(row, "Args2")
	if bound := tableNumber(row, "Args3"); bound != 0 {
		if row["Factor"] == "perhpTodamageAdd" {
			value = math.Max(value, bound)
		} else {
			value = math.Min(value, bound)
		}
	}
	return value
}

// Original minusMaxHpPercent changes HP once, with a current-skill attack cap.
// Its AddHp call has physical type1 even when its cap uses magical attack.
func (e *combatRoundEngine) maxHPPercentEffect(source, target *combatRoundActor, row map[string]string, buffs ...*combatRoundBuff) float64 {
	attackType := source.attackType
	if source.current != nil {
		attackType = source.current.attackType
	}
	power := source.attribute("att")
	if attackType == 1 {
		power += source.attribute("patt")
	} else if attackType == 2 {
		power += source.attribute("matt")
	}
	value := target.attribute("maxHp") * tableNumber(row, "Args1") * .01
	limit := power * tableNumber(row, "Args2") * .01
	if value > 0 {
		value = math.Min(value, limit)
	} else {
		value = math.Max(value, -limit)
	}
	if value < 0 {
		e.originalBuffDamageHP(source, target, -value, 1, 14, buffs...)
	} else if value > 0 {
		e.originalBuffChangeHP(source, target, value, "buff_heal", 14, buffs...)
	}
	e.trace("maxhp_percent_change", source, target, "", "", value)
	return value
}
