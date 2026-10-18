package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// combatRoundSkill 保存每个技能自己的回合CD、等待/施法/尾段状态与稳定出手顺序。
type combatRoundSkill struct {
	actor          *combatRoundActor
	id             string
	index          int32
	row            map[string]string
	casts          []map[string]string
	releases       []combatRoundRelease
	state          int32
	cooldown       int32
	maxCooldown    int32
	stage          int32
	releaseIndex   int
	begin, end     float64
	move           float64
	beginNotified  bool
	target         *combatRoundActor
	targets        []*combatRoundActor
	sameDamTargets []*combatRoundActor
	buffOverrides  map[string]string
	first          bool
	attackType     int32
	firstCast      string
}
type combatRoundRelease struct {
	cast   map[string]string
	time   float64
	isCast bool
}
type combatRoundActor struct {
	runtimePose         int32
	hero                *pb.BattleHero
	uid                 string
	camp                int32
	heroClass           int32
	aiProperty          int32
	attackType          int32
	curSkillClass       int32
	counterReal         bool
	counterSelect       int32
	blockDam            float64
	shareFactor         float64
	shareActors         []*combatRoundActor
	angerToHPAP         int32
	angerToHPAmount     float64
	lossToAPHP          float64
	lossToAPAP          int32
	hpLossAccumulator   float64
	hpChangeInRound     float64
	state               int32
	skills              []*combatRoundSkill
	current             *combatRoundSkill
	waiting             []*combatRoundSkill
	movedTarget         string
	movedRule           int32
	waitUntil           float64
	hp                  float64
	base                map[string]float64
	originalPercent     map[string]float64
	originalPercentBase map[string]float64
	buffs               []*combatRoundBuff
	outgoingReduction   []*combatRoundBuff
	talents             []combatRoundTrigger
	flags               map[string]bool
	immuneBuff          map[string]bool
	immuneTalent        map[string]bool
	speState            int32
	unableStates        map[int32]bool
	speUntil            float64
	magicStones         int32
	selfRounds          int32
	selfCombo           int32
	deathSource         *combatRoundActor
	revived             bool
	reboundUsed         bool
	rebounded           bool
}
type combatRoundPlayer struct {
	actors     []*combatRoundActor
	deadActors []*combatRoundActor
	anger      float64
	combo      int32
	lastCombo  float64
}
type combatRoundCommand struct {
	actor  *combatRoundActor
	skills []*combatRoundSkill
}
type combatRoundTableIndexKey struct {
	name, key string
}

type combatRoundTableIndex struct {
	rows   []map[string]string
	values map[string]map[string]string
}

type combatRoundEngine struct {
	originalFormationApplied       bool
	roundOneLiveAutoFormation      *pb.BattleTransOperate
	roundOneLiveAutoStarted        bool
	originalRecordMode             bool
	tableIndexes                   map[combatRoundTableIndexKey]combatRoundTableIndex
	service                        *GameService
	input                          *model.FrozenBattleInput
	rng                            *CombatRandom
	players                        [2]*combatRoundPlayer
	round                          int32
	camp                           int32
	time                           float64
	commands                       []combatRoundCommand
	commandIndex                   int
	aiDelay                        float64
	pendingUltimate                *combatRoundSkill
	ultimateRunning                bool
	aiUltimateCleanup              bool
	aiCleanupReturned              bool
	aiDealingNormals               []*combatRoundSkill
	copyEnemyDealt                 []*combatRoundSkill
	copyEnemyUltimatePhase         bool
	copyEnemyUltimateQueue         []*combatRoundSkill
	copyEnemyCurrentUltimate       *combatRoundSkill
	copyEnemyFinishNotified        bool
	originalPlaybackFinishHint     bool
	pveNoTargetPresses             map[*combatRoundSkill]combatRoundNoTargetPress
	captureTicks                   bool
	diagnosticCasts                []map[string]any
	originalExportsStarted         bool
	originalStandaloneLoadGroup    int64
	originalStandaloneLoadSequence int64
	originalRoundBuffDeath         *combatRoundBuff
	originalBuffValueDeath         *combatRoundBuff
	originalStandaloneLoadParent   *combatRoundBuff
	originalBuffSequence           int64
	originalBuffSnapshots          []combatRoundPendingBuffSnapshot
	originalCastScope              *combatRoundHPExportOrigin
	events                         []model.CombatEvent
	operate                        *pb.BattleOperate
	supplied                       *pb.BattleOperate
	suppliedIndex                  int
	failure                        error
	effectDepth                    int
}

// Each calculation already owns a deeply cloned frozen table collection. Index
// original rows lazily within that calculation and preserve the first duplicate,
// including a nil first row for the empty key, exactly like GameService.table.
func (e *combatRoundEngine) table(name, key, value string) map[string]string {
	if e.tableIndexes == nil {
		e.tableIndexes = map[combatRoundTableIndexKey]combatRoundTableIndex{}
	}
	indexKey := combatRoundTableIndexKey{name: name, key: key}
	index, found := e.tableIndexes[indexKey]
	rows := e.service.Tables[name]
	// Frozen production tables remain immutable. Explicit mechanism probes may
	// append or replace a local table after engine creation; discard that old
	// index without making the ordinary per-hit lookup scan all original rows.
	if !found || len(index.rows) != len(rows) || len(rows) > 0 && &index.rows[0] != &rows[0] {
		index = combatRoundTableIndex{rows: rows, values: make(map[string]map[string]string, len(rows))}
		for _, row := range rows {
			id := row[key]
			if _, exists := index.values[id]; !exists {
				index.values[id] = row
			}
		}
		e.tableIndexes[indexKey] = index
	}
	return index.values[value]
}

func (e *combatRoundEngine) number(key string) float64 {
	return tableNumber(e.table("GameConfigure", "StaticName", key), "NameValue")
}
func (a *combatRoundActor) skill(index int32) *combatRoundSkill {
	for _, skill := range a.skills {
		if skill.index == index {
			return skill
		}
	}
	return nil
}
func (a *combatRoundActor) ready(skill *combatRoundSkill) bool {
	return skill != nil && a.hp > 0 && a.state < 3 && skill.state == 0 && skill.cooldown <= 0
}
func (e *combatRoundEngine) canSkill(skill *combatRoundSkill) bool {
	if skill == nil || !skill.actor.ready(skill) || skill.actor.hasState("dizzy") {
		return false
	}
	// Original FActor.isCanDoSkill applies state prohibitions before AI builds
	// normal commands or chooses an ultimate actor. Rejected actors cannot hide
	// the next eligible ultimate in GetUltimateReadyActor's position order.
	a := skill.actor
	if skill.row["IsUltimate"] == "1" && (a.hasState("forbidUltimateSkill") || e.players[a.camp].anger < e.number("UTIL_TRIGGER_ANGERCOUNT")) {
		return false
	}
	return !(skill.index == 0 && a.hasState("forbidFirstSkill") || skill.index == 1 && a.hasState("forbidSecondSkill") || skill.index == 3 && a.hasState("forbidThirdSkill"))
}
func (e *combatRoundEngine) trace(kind string, actor, target *combatRoundActor, skill, cast string, value float64) {
	event := model.CombatEvent{Sequence: int64(len(e.events) + 1), Round: e.round, Time: e.time, Camp: e.camp, Kind: kind, Skill: skill, Cast: cast, Value: value, RandomDraws: e.rng.Draws()}
	if actor != nil {
		event.Actor = actor.uid
		event.Camp = actor.camp
	}
	if target != nil {
		event.Target = target.uid
		event.TargetHP = target.hp
	}
	e.events = append(e.events, event)
}
func (e *combatRoundEngine) addAnger(actor *combatRoundActor, value float64) {
	// FPlayer.AddAnger scales only positive gains; suppression never turns gain into loss.
	if value > 0 {
		value *= 1 + actor.attribute("angerRate")*.01
		if value <= 0 {
			return
		}
	}
	p := e.players[actor.camp]
	before := p.anger
	p.anger = math.Max(0, math.Min(e.number("UTIL_MAXANGER"), p.anger+value))
	if before != p.anger {
		e.trace("anger", actor, nil, "", "", p.anger-before)
	}
}
func (e *combatRoundEngine) alive(camp int32) bool {
	for _, actor := range e.players[camp].actors {
		if actor.hp > 0 || actor.state == 3 {
			return true
		}
	}
	return false
}
func (e *combatRoundEngine) anyRunning() bool {
	for _, p := range e.players {
		for _, a := range p.actors {
			if a.current != nil {
				return true
			}
		}
	}
	return false
}
func (e *combatRoundEngine) find(uid string, camp int32) *combatRoundActor {
	for _, a := range e.players[camp].actors {
		if a.uid == uid {
			return a
		}
	}
	return nil
}

// ComputeFrozenBattle 用原0.1秒逻辑步长推进整场，输入/配置/seed不从客户端即时属性读回。
// supplied非nil时重演原操作；奖励开放另受能力矩阵和原客户端对照门禁约束。
func (s *GameService) ComputeFrozenBattle(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate) (*model.CombatOutcome, error) {
	return s.computeFrozenBattleObserved(ctx, input, supplied, nil)
}

// The optional observer reads completed logic ticks for original-engine
// diagnostics. Ordinary simulation and replay retain their existing outputs.
func (s *GameService) computeFrozenBattleObserved(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate, observer func(*combatRoundEngine)) (*model.CombatOutcome, error) {
	return s.computeFrozenBattleLifecycleObserved(ctx, input, supplied, observer, nil)
}

// Terminal audit observes the completed original runtime before output actor
// transport. Base calculations have no terminal observer or extra audit hashing.
func (s *GameService) computeFrozenBattleLifecycleObserved(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate, observer, terminalObserver func(*combatRoundEngine)) (*model.CombatOutcome, error) {
	return s.computeFrozenBattleRecordObserved(ctx, input, supplied, observer, terminalObserver, false)
}

// Original PVE protocol playback drives camp0 while the enemy remains automatic.
// Its attacker-owned record container can include both camps' actions. The default
// entry retains the established complete diagnostic operation replay semantics.
func (s *GameService) computeFrozenBattleRecordObserved(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate, observer, terminalObserver func(*combatRoundEngine), originalRecordMode bool) (*model.CombatOutcome, error) {
	return s.computeFrozenBattleRecordControlled(ctx, input, supplied, observer, terminalObserver, originalRecordMode, nil)
}

// Only the new round1/time0 live driver supplies formation here. Existing public
// and original-record entrypoints pass nil and preserve their execution order.
func (s *GameService) computeFrozenBattleRecordControlled(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate, observer, terminalObserver func(*combatRoundEngine), originalRecordMode bool, formation *pb.BattleTransOperate) (*model.CombatOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozenInput, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	frozen := *frozenInput
	if err = ValidateP6AIDriverPolicy(&frozen); err != nil {
		return nil, err
	}
	if err = ValidateP6DamageFactors(&frozen); err != nil {
		return nil, err
	}
	if err = validateFrozenP6(&frozen); err != nil {
		return nil, err
	}
	if err = ValidateP6FirstOrder(&frozen); err != nil {
		return nil, err
	}
	if err = validateP6KFCalculationBoundary(&frozen); err != nil {
		return nil, err
	}
	// The original playback field has no Engine/BuildInFightData consumer. Keep
	// its original protobuf transport without inventing victory-condition rules.
	for _, condition := range frozen.WinConditions {
		if condition == nil {
			return nil, errors.New(constUtil.CombatInvalid)
		}
	}
	tables, err := json.Marshal(frozen.Tables)
	if err != nil || p6Digest(tables) != frozen.ConfigurationHash || frozen.EngineVersion != constUtil.P6EngineVersion {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	if err = ValidateP6PVECompatibility(frozen); err != nil {
		return nil, err
	}
	e := &combatRoundEngine{service: &GameService{Tables: frozen.Tables}, input: &frozen, rng: NewCombatRandom(frozen.Seed), operate: &pb.BattleOperate{}, supplied: supplied, captureTicks: observer != nil, round: 1, originalRecordMode: originalRecordMode}
	if formation != nil {
		e.roundOneLiveAutoFormation = proto.Clone(formation).(*pb.BattleTransOperate)
	}
	if frozen.DefenderFirst {
		e.camp = 1
	}
	// ComputeCore.Start0013 establishes round1 before BattleBegin loads timer1 talents.
	if err = e.initialize(); err != nil {
		return nil, err
	}
	for _, p := range e.players {
		for _, a := range p.actors {
			// Original BattleBegin skips DEAD actors, including initial0HP
			// actors retained by FPlayer.init in its runtime actor collection.
			if a.state == 4 {
				continue
			}
			if err = e.talentTimer(a, nil, 1); err != nil {
				return nil, err
			}
		}
	}
	if supplied != nil {
		if err = e.validateOperates(supplied); err != nil {
			return nil, err
		}
		if originalRecordMode {
			// Original PlaybackAI.NextRound selects only messages for its own
			// camp. Preserve the caller's full record and build a private camp0
			// driver queue; NPC source entries do not suppress its original AI.
			e.supplied = proto.Clone(supplied).(*pb.BattleOperate)
			e.supplied.SkillOpts = nil
			if OriginalP6RecordedCamp(frozen.BattleType, 0) {
				for _, op := range supplied.SkillOpts {
					if op.GetCamp() == 0 {
						e.supplied.SkillOpts = append(e.supplied.SkillOpts, proto.Clone(op).(*pb.BattleSkillOperate))
					}
				}
			}
		}
	}
	if formation != nil {
		if supplied != nil || !originalRecordMode || formation.GetRoundNum() != 1 {
			return nil, errors.New(constUtil.P6ReplayConflict)
		}
		if err = e.validateOriginalFormationRecord(&pb.BattleOperate{AttackerTransOpt: e.roundOneLiveAutoFormation}); err != nil {
			return nil, err
		}
	}
	e.originalFinalizeBuffSnapshots()
	// Original EngineMsgCallback always evaluates subject camp 0. Its enemy-dead
	// win condition is checked before own-dead or the round-limit loss condition.
	e.originalExportsStarted = true
	winner := int32(1)
	// Independently admitted KF/Grab wrappers use the original caller's deferred FinishRound.
	// ComputeCore.Update consumes every due0.1 substep before that caller acts.
	// Its two global double clocks persist across rounds and can group two
	// substeps in one Update when accumulated subtraction crosses0.1.
	var (
		originalRealTime    float64
		originalCurrentTime float64
	)
	originalDeferredUpdate := frozen.BattleType == 14 || frozen.BattleType == 19 || frozen.BattleType == 20 || (frozen.KFReplay != nil && frozen.KFReplayViewer != nil && frozen.KFReplayViewer.CompatibilityRule == constUtil.P6KFJointViewerCompatibility)
	for e.round = 1; e.round <= frozen.MaxRound; {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		e.camp = (e.round - 1) % 2
		if frozen.DefenderFirst {
			e.camp = 1 - e.camp
		}
		if err = e.beginRound(); err != nil {
			return nil, err
		}
		e.originalFinalizeBuffSnapshots()
		// Original RoundBegin checks both sides after round triggers and calls
		// BattleEnd before AI runs when one side already has no living actor.
		if !e.alive(0) || !e.alive(1) {
			if !e.alive(1) {
				winner = 0
			}
			break
		}
		// Original PlaybackAI.Run follows RoundBegin triggers/callbacks and
		// applies its formation change before reading this round's messages.
		if err = e.applyOriginalFormationRecord(); err != nil {
			return nil, err
		}
		if err = e.applyRoundOneLiveAutoFormation(); err != nil {
			return nil, err
		}
		originalFinishPending := false
		for ticks := 0; ticks < 1200; ticks++ {
			if ticks%100 == 0 {
				if err = ctx.Err(); err != nil {
					return nil, err
				}
			}
			if originalDeferredUpdate {
				for !(originalRealTime-originalCurrentTime > 0.1) {
					originalRealTime += 0.1
				}
				originalCurrentTime += 0.1
			}
			if err = e.step(); err != nil {
				return nil, err
			}
			if observer != nil {
				observer(e)
			}
			if len(e.events) > constUtil.P6MaxEvents {
				return nil, errors.New(constUtil.CombatInvalid)
			}
			if !e.alive(0) || !e.alive(1) {
				break
			}
			if e.canFinishRound() {
				if err = e.validateOriginalCopyEnemyRoundFinish(); err != nil {
					return nil, err
				}
				originalFinishPending = true
			}
			// A callback requests FinishRound; it cannot interrupt DoUpdate's
			// current outer Update batch. Preserve any remaining actor/Buff tick.
			if originalFinishPending && (!originalDeferredUpdate || !(originalRealTime-originalCurrentTime > 0.1)) {
				break
			}
			// ComputeCore.DoUpdate adds delta after both player updates; repeated double
			// addition matches its threshold ordering instead of rounding each frame.
			e.time += 0.1
			if ticks == 1199 {
				return nil, errors.New("战斗逻辑回合超过原有界运行范围")
			}
		}
		// ActorDead ends a release immediately, without calling FinishRound.
		// A normal FinishRound increments roundNum before RoundEnd effects;
		// forced deaths during ProcMyRoundEnd therefore carry that next number.
		if !e.alive(0) || !e.alive(1) {
			if !e.alive(1) {
				winner = 0
			}
			break
		}
		// The original caller handles the pending FinishRound after DoUpdate
		// has advanced currentTime by delta. End-round callbacks use that clock.
		e.time += 0.1
		e.round++
		if err = e.endRound(); err != nil {
			return nil, err
		}
		if !e.alive(1) {
			winner = 0
			break
		}
		if !e.alive(0) {
			winner = 1
			break
		}
	}
	if e.roundOneLiveAutoFormation != nil && !e.originalFormationApplied {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	// Original RoundEnd advances roundNum before the max-round BattleEnd check.
	// A naturally exhausted input reports MaxRound+1; death endings keep their round.
	finalRound := e.round
	if e.supplied != nil && e.suppliedIndex != len(e.supplied.SkillOpts) {
		return nil, errors.New("录像含有结束后或不可执行的操作")
	}
	if e.supplied != nil && e.supplied.AttackerTransOpt != nil && !e.originalFormationApplied {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	e.operate.FinalRoundNum = proto.Int32(finalRound)
	e.operate.AttackerTotalLeftHp = proto.Float64(e.totalHP(0))
	e.operate.DefenderTotalLeftHp = proto.Float64(e.totalHP(1))
	comparison := e.operate
	if originalRecordMode {
		comparison, err = OriginalP6OperateProjection(frozen.BattleType, e.operate)
		if err != nil {
			return nil, err
		}
	}
	recordMatches := supplied == nil
	if supplied != nil {
		if originalRecordMode {
			recordMatches = p6OriginalProtocolRecordEqual(supplied, comparison)
		} else {
			recordMatches = p6OriginalOperateEqual(supplied, comparison)
		}
	}
	if !recordMatches {
		return nil, fmt.Errorf("%s: 原录像终局或完整技能记录不符 round=%d hp=%g/%g，计算 round=%d hp=%g/%g", constUtil.P6ReplayConflict, supplied.GetFinalRoundNum(), supplied.GetAttackerTotalLeftHp(), supplied.GetDefenderTotalLeftHp(), e.operate.GetFinalRoundNum(), e.operate.GetAttackerTotalLeftHp(), e.operate.GetDefenderTotalLeftHp())
	}
	if terminalObserver != nil {
		terminalObserver(e)
	}
	output := &model.CombatOutcome{Winner: winner, FinalRound: finalRound, Operates: e.operate, Attacker: frozen.Attacker, Defender: frozen.Defender, Monster: frozen.Monster, Events: e.events, RandomDraws: e.rng.Draws(), Authoritative: false, PVECompatibilityRule: frozen.PVECompatibilityRule}
	output.Attacker.Inst.Anger = proto.Float64(e.players[0].anger)
	if output.Defender != nil {
		output.Defender.Inst.Anger = proto.Float64(e.players[1].anger)
	} else {
		output.Monster.Inst.Anger = proto.Float64(e.players[1].anger)
	}
	for _, p := range e.players {
		for _, actor := range p.actors {
			actor.hero.Inst.CurHp = proto.Float64(actor.hp)
		}
	}
	return output, nil
}

// cloneP6CalculationInput isolates every mutable input before simulation. Table
// strings are immutable; cloning their maps avoids encoding and parsing the
// entire original table collection for every automatic fight and replay.
func cloneP6CalculationInput(input *model.FrozenBattleInput) (*model.FrozenBattleInput, error) {
	if input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen := *input
	if input.KFReplayViewer != nil {
		viewer := *input.KFReplayViewer
		frozen.KFReplayViewer = &viewer
	}
	if input.DiagnosticManualAutoStartRoundTime != nil {
		frozen.DiagnosticManualAutoStartRoundTime = proto.Float64(*input.DiagnosticManualAutoStartRoundTime)
	}
	if input.AttackerDamageFactor != nil {
		frozen.AttackerDamageFactor = proto.Float64(*input.AttackerDamageFactor)
	}
	if input.DefenderDamageFactor != nil {
		frozen.DefenderDamageFactor = proto.Float64(*input.DefenderDamageFactor)
	}
	if input.Attacker != nil {
		frozen.Attacker = proto.Clone(input.Attacker).(*pb.BattlePlayer)
	}
	if input.Defender != nil {
		frozen.Defender = proto.Clone(input.Defender).(*pb.BattlePlayer)
	}
	if input.Monster != nil {
		frozen.Monster = proto.Clone(input.Monster).(*pb.BattleMonster)
	}
	if input.KFReplay != nil {
		frozen.KFReplay = proto.Clone(input.KFReplay).(*pb.KFReplayData)
	}
	if input.WinConditions != nil {
		frozen.WinConditions = make([]*pb.BattleWinCondition, len(input.WinConditions))
		for i, condition := range input.WinConditions {
			if condition != nil {
				frozen.WinConditions[i] = proto.Clone(condition).(*pb.BattleWinCondition)
			}
		}
	}
	if input.AppliedProperties != nil {
		frozen.AppliedProperties = make(map[string][]string, len(input.AppliedProperties))
		for key, values := range input.AppliedProperties {
			if values == nil {
				frozen.AppliedProperties[key] = nil
			} else {
				frozen.AppliedProperties[key] = append(make([]string, 0, len(values)), values...)
			}
		}
	}
	// Retain the previous JSON finite-number gate for protobuf panels, including
	// nested optional KF players. Tables contain strings only and are hashed once
	// by the caller after cloning; omitting them here removes the redundant pass.
	metadata := frozen
	metadata.Tables = nil
	if _, err := json.Marshal(metadata); err != nil {
		return nil, err
	}
	if input.Tables != nil {
		frozen.Tables = make(map[string][]map[string]string, len(input.Tables))
		for name, rows := range input.Tables {
			if rows == nil {
				frozen.Tables[name] = nil
				continue
			}
			clonedRows := make([]map[string]string, len(rows))
			for i, row := range rows {
				if row == nil {
					continue
				}
				clonedRows[i] = make(map[string]string, len(row))
				for key, value := range row {
					clonedRows[i][key] = value
				}
			}
			frozen.Tables[name] = clonedRows
		}
	}
	return &frozen, nil
}

// FrozenP6DamageFactor reads a camp's original multiplier. Old records omitted
// this field when their producers used one; explicit zero remains zero.
func FrozenP6DamageFactor(input *model.FrozenBattleInput, camp int32) float64 {
	if input != nil {
		factor := input.AttackerDamageFactor
		if camp == 1 {
			factor = input.DefenderDamageFactor
		}
		if factor != nil {
			return *factor
		}
	}
	return 1
}

// ValidateP6DamageFactors bounds the currently supported original nonnegative
// multiplier path; unsupported signed inversion cannot silently change shields.
func ValidateP6DamageFactors(input *model.FrozenBattleInput) error {
	for _, camp := range []int32{0, 1} {
		factor := FrozenP6DamageFactor(input, camp)
		if !combatFinite(factor) || math.Abs(factor) > 1e15 {
			return errors.New(constUtil.CombatInvalid)
		}
		if factor < 0 {
			return fmt.Errorf("%s: negative damageFactor signed inversion", constUtil.CombatUnsupported)
		}
	}
	return nil
}

func (e *combatRoundEngine) initialize() error {
	if e.input.MaxRound < 1 || e.input.MaxRound > 40 {
		return errors.New(constUtil.CombatInvalid)
	}
	for _, key := range []string{"UTIL_TRIGGER_ANGERCOUNT", "UTIL_MAXANGER", "SKILL_Round_CoolDown", "UTL_Round_CoolDown", "moveToSkillTime", "monsterMoveSkillTime", "ultMoveToSkillTime", "PlayerAIDeltaTime", "CopyEnemyAIDeltaTime"} {
		if e.number(key) <= 0 {
			return fmt.Errorf("%s: %s", constUtil.CombatInvalid, key)
		}
	}
	formations := [][]*pb.BattleHero{e.input.Attacker.Formation}
	anger := []float64{e.input.Attacker.GetInst().GetAnger()}
	if e.input.Attacker.Inst == nil {
		e.input.Attacker.Inst = &pb.BattlePlayerInst{}
	}
	if e.input.Defender != nil {
		formations = append(formations, e.input.Defender.Formation)
		anger = append(anger, e.input.Defender.GetInst().GetAnger())
		if e.input.Defender.Inst == nil {
			e.input.Defender.Inst = &pb.BattlePlayerInst{}
		}
	} else {
		formations = append(formations, e.input.Monster.Formation)
		anger = append(anger, e.input.Monster.GetInst().GetAnger())
		if e.input.Monster.Inst == nil {
			e.input.Monster.Inst = &pb.BattleMonsterInst{}
		}
	}
	for camp, formation := range formations {
		e.players[camp] = &combatRoundPlayer{anger: anger[camp]}
		for _, hero := range formation {
			row := e.table("Hero", "id", hero.GetId())
			if row == nil {
				return errors.New(constUtil.CombatUnsupported)
			}
			a := &combatRoundActor{hero: hero, runtimePose: hero.GetPose(), uid: fmt.Sprintf("%d_%s", camp, hero.GetGuid()), camp: int32(camp), heroClass: int32(tableNumber(row, "heroClass")), aiProperty: int32(tableNumber(row, "aiProperty")), hp: hero.GetInst().GetCurHp(), movedRule: -1, curSkillClass: -1}
			if a.hp == 0 {
				a.state = 4
			}
			// Dynamic mechanisms are accepted only after their original handlers exist.
			// Native bloodNumber only chooses 1..5 visual bars; total HP drives the engine.
			if row["DefaultState"] != "1" {
				return errors.New(constUtil.CombatUnsupported)
			}
			// FActor.init applies its default special state before fight-start talents.
			a.speState = 1
			a.unableStates = map[int32]bool{}
			for _, value := range strings.Split(row["ImposState"], ",") {
				state, parseErr := strconv.ParseInt(strings.TrimSpace(value), 10, 32)
				if parseErr != nil || state < 0 || state > 3 {
					return fmt.Errorf("%s: ImposState %q", constUtil.CombatUnsupported, row["ImposState"])
				}
				a.unableStates[int32(state)] = true
			}
			slots, awakenID, err := e.p6AwakenSkillSlots(hero, row)
			if err != nil {
				return err
			}
			if err := e.initializeDynamic(a); err != nil {
				return err
			}
			for index, key := range slots {
				if key == "" {
					continue
				}
				skillRow := e.table("Skill", "SkillId", key)
				if skillRow == nil || index > 3 {
					return errors.New(constUtil.CombatUnsupported)
				}
				if key != row["SkillId1"] && key != row["SkillId2"] && key != row["UltSkillId"] && key != awakenID && !e.originalScriptedSkillSequence(hero) {
					return errors.New(constUtil.CombatInvalid)
				}
				skill := &combatRoundSkill{actor: a, id: key, index: int32(index), row: skillRow, maxCooldown: int32(e.number("SKILL_Round_CoolDown"))}
				class := int32(tableNumber(skillRow, "SkillClass"))
				if class < 0 || class > 1 {
					return errors.New(constUtil.CombatUnsupported)
				}
				if skillRow["IsUltimate"] == "1" {
					skill.maxCooldown = int32(e.number("UTL_Round_CoolDown"))
				}
				for _, cast := range e.service.Tables["SkillCast"] {
					if cast["SkillId"] == key {
						for _, column := range []string{"CauseSpeState", "AdvSpeState"} {
							if v := tableNumber(cast, column); v < 0 || v > 3 {
								return errors.New(constUtil.CombatInvalid)
							}
						}
						typ := tableNumber(cast, "ExportType")
						if typ < 0 || typ > 3 {
							return errors.New(constUtil.CombatUnsupported)
						}
						if tableNumber(cast, "ExportFactor") < 0 || tableNumber(cast, "CastTime") < 0 || !combatFinite(tableNumber(cast, "FlyTime")) || tableNumber(cast, "FlyTime") < 0 {
							return errors.New(constUtil.CombatInvalid)
						}
						if err := e.validateCastBuffs(cast); err != nil {
							return err
						}
						if skill.firstCast == "" {
							skill.firstCast = cast["SkillCastId"]
						}
						skill.casts = append(skill.casts, cast)
						// IMMEDIATE_PROJECT emits a view callback, then its actual cast after FlyTime.
						castTime := tableNumber(cast, "CastTime")
						if class == 1 {
							skill.releases = append(skill.releases, combatRoundRelease{cast: cast, time: castTime, isCast: false})
							castTime += tableNumber(cast, "FlyTime")
						}
						skill.releases = append(skill.releases, combatRoundRelease{cast: cast, time: castTime, isCast: true})
					}
				}
				if len(skill.casts) == 0 || tableNumber(skillRow, "Duration") <= 0 {
					return errors.New(constUtil.CombatUnsupported)
				}
				// FSkill stores the last original table cast's type before sorting timings.
				lastType := int32(tableNumber(skill.casts[len(skill.casts)-1], "ExportType"))
				skill.attackType = lastType
				if a.attackType == 0 && skill.index != 2 && lastType >= 1 && lastType <= 2 {
					a.attackType = lastType
				}
				sort.SliceStable(skill.casts, func(i, j int) bool {
					return tableNumber(skill.casts[i], "CastTime") < tableNumber(skill.casts[j], "CastTime")
				})
				sort.SliceStable(skill.releases, func(i, j int) bool { return skill.releases[i].time < skill.releases[j].time })
				a.skills = append(a.skills, skill)
			}
			// Native empty original slots create an idle actor and retain physical fallback.
			// A defined combat hero must still provide a verified original skill sequence.
			if len(a.skills) == 0 && (row["SkillId1"] != "" || row["SkillId2"] != "" || row["UltSkillId"] != "") {
				return errors.New(constUtil.CombatUnsupported)
			}
			if a.attackType == 0 {
				a.attackType = 1
			}
			// FPlayer.init branches to0x133 for nonpositiveHP, skipping only
			// totalHP accumulation. Actor Add is at0x13a and retains the actor;
			// only actual deaths during this battle enter deadActors.
			e.players[camp].actors = append(e.players[camp].actors, a)
		}
	}
	return nil
}

func (e *combatRoundEngine) beginRound() error {
	e.time = 0
	e.aiDelay = 0
	e.commandIndex = 0
	e.commands = nil
	e.pendingUltimate = nil
	e.ultimateRunning = false
	e.aiUltimateCleanup = false
	e.aiDealingNormals = nil
	e.copyEnemyDealt = nil
	e.copyEnemyUltimatePhase = false
	e.copyEnemyUltimateQueue = nil
	e.copyEnemyCurrentUltimate = nil
	e.copyEnemyFinishNotified = false
	e.originalPlaybackFinishHint = false
	e.pveNoTargetPresses = nil
	for _, p := range e.players {
		p.combo = 0
		p.lastCombo = 0
		for _, a := range p.actors {
			a.current = nil
			a.selfCombo = 0
			if a.state < 3 {
				a.state = 0
			}
			a.waiting = nil
			a.hpLossAccumulator = 0
			a.hpChangeInRound = 0
			a.movedTarget = ""
			a.movedRule = -1
			a.waitUntil = 0
			a.speState = 1
			a.speUntil = 0
			for _, skill := range a.skills {
				skill.state = 0
				skill.stage = 0
				skill.releaseIndex = 0
				skill.first = false
				skill.target = nil
				skill.targets = nil
				skill.sameDamTargets = nil
			}
		}
	}
	e.trace("round_begin", nil, nil, "", "", 0)
	for _, camp := range []int32{e.camp, 1 - e.camp} {
		for _, a := range e.players[camp].actors {
			if a.state == 4 {
				continue
			}
			timer := int32(3)
			if a.camp == e.camp {
				a.selfRounds++
				timer = 2
			}
			if err := e.talentTimer(a, nil, timer); err != nil {
				return err
			}
			if a.camp == e.camp {
				if err := e.talentTimer(a, nil, 35); err != nil {
					return err
				}
				for _, b := range append([]*combatRoundBuff(nil), a.buffs...) {
					beforeRevived := a.revived
					switch b.row["Factor"] {
					case "additionAttBySelfRound":
						if err := e.originalRoundExtraAttacks(b); err != nil {
							return err
						}
					case "permaxHpBySelfRound":
						previous := e.originalRoundBuffDeath
						e.originalRoundBuffDeath = b
						e.applyRoundMaxHP(b)
						e.originalRoundBuffDeath = previous
					case "recoverBySelfRound":
						e.originalBuffChangeHP(b.source, a, b.value, "round_heal", 11, b)
					case "poisonDam", "fireDam":
						if a.state >= 3 {
							continue
						}
						previous := e.originalRoundBuffDeath
						e.originalRoundBuffDeath = b
						if b.value > 0 {
							hpType := int32(9)
							if b.row["Factor"] == "fireDam" {
								hpType = 10
							}
							e.originalBuffDamageHP(b.source, a, b.value, 2, hpType, b)
						} else if b.value < 0 {
							e.originalBuffChangeHP(b.source, a, -b.value, "periodic_heal", 11, b)
						}
						e.originalRoundBuffDeath = previous
						e.trace("periodic_damage", b.source, a, "", "", b.value)
						e.events[len(e.events)-1].Buff = b.row["Id"]
					}
					if a.state >= 3 || !beforeRevived && a.revived {
						break
					}
				}
			}
			// Original RoundBegin resets both living camps' per-actor stone
			// counts after their own/enemy round-start talent processing.
			a.magicStones = 0
		}
	}
	// Original live Auto is enabled after the explicit round1 DoTrans. Its Run
	// must build the queue from the changed runtime positions, not old slots.
	if e.roundOneLiveAutoFormation != nil && e.round == 1 && e.camp == 0 {
		return nil
	}
	return e.buildOriginalRoundAICommands()
}

func (e *combatRoundEngine) buildOriginalRoundAICommands() error {
	actors := append([]*combatRoundActor(nil), e.players[e.camp].actors...)
	sort.SliceStable(actors, func(i, j int) bool {
		// Original CopyEnemyAI groups descending aiProperty, then ascending FPID.
		// PlayerAI keeps its separate formation-order contract.
		if e.usesCopyEnemyAI(e.camp) && actors[i].aiProperty != actors[j].aiProperty {
			return actors[i].aiProperty > actors[j].aiProperty
		}
		return actors[i].position() < actors[j].position()
	})
	for _, a := range actors {
		var skills []*combatRoundSkill
		for _, index := range []int32{0, 1, 3} {
			if skill := a.skill(index); e.canSkill(skill) {
				skills = append(skills, skill)
			}
		}
		if a.heroClass == 1 && e.usesCopyEnemyAI(e.camp) {
			if e.canSkill(a.skill(2)) {
				e.copyEnemyUltimateQueue = append(e.copyEnemyUltimateQueue, a.skill(2))
				continue
			}
			if len(skills) >= 2 {
				chance := a.hero.GetSkillCastChance() * 0.009999999776482582
				if chance <= 0 {
					chance = 0.5
				}
				chosen := 0
				if e.rng.Value() <= chance {
					chosen = 1
				}
				skills = []*combatRoundSkill{skills[chosen]}
			}
		}
		if e.usesCopyEnemyAI(e.camp) {
			for _, skill := range skills {
				e.commands = append(e.commands, combatRoundCommand{a, []*combatRoundSkill{skill}})
			}
		} else if len(skills) > 0 {
			e.commands = append(e.commands, combatRoundCommand{a, skills})
		}
	}
	return nil
}

func (e *combatRoundEngine) issue(skill *combatRoundSkill) bool {
	if !e.canSkill(skill) || e.pendingUltimate != nil {
		return false
	}
	a := skill.actor
	if skill.row["IsUltimate"] == "1" {
		skill.state = 1
		e.pendingUltimate = skill
	} else if a.current != nil {
		skill.state = 1
		a.waiting = append(a.waiting, skill)
	} else {
		e.beginSkill(skill)
	}
	e.operate.SkillOpts = append(e.operate.SkillOpts, &pb.BattleSkillOperate{RoundNum: proto.Int32(e.round), RoundTime: proto.Float64(e.time), Camp: proto.Int32(a.camp), ActorUid: proto.String(a.hero.GetGuid()), SkillIndex: proto.Int32(skill.index)})
	e.trace("skill_press", a, nil, skill.id, "", float64(skill.index))
	e.recordPVENoTargetPress(skill)
	return true
}

func (e *combatRoundEngine) beginSkill(skill *combatRoundSkill) {
	a := skill.actor
	strategy := int32(3)
	if tableNumber(skill.casts[0], "ExportType") == 3 {
		strategy = 103
	}
	targets, err := e.selectTargets(a, strategy, nil, nil)
	if err != nil || len(targets) == 0 {
		// FSkill.Do 007a–0085 writes a nil initial target and returns before
		// changing the skill state. FActor.SkillBegin also returns before taking
		// an active skill slot; an unused normal remains Ready at battle end.
		skill.target = nil
		return
	}
	targets, err = e.selectTargets(a, int32(tableNumber(skill.casts[0], "SelectStrategy")), targets[0], nil)
	if err != nil || len(targets) == 0 {
		skill.state = 4
		if skill == e.pendingUltimate {
			e.pendingUltimate = nil
		}
		return
	}
	skill.target = targets[0]
	delete(e.pveNoTargetPresses, skill)
	skill.targets = targets
	skill.sameDamTargets = nil
	skill.state = 2
	skill.stage = 0
	skill.releaseIndex = 0
	skill.first = false
	move := 0.0
	rule := int32(tableNumber(skill.row, "SkillCastPointSelectRule"))
	if skill.row["IsUltimate"] == "1" {
		move = e.number("ultMoveToSkillTime")
	} else if rule != 4 && (a.movedTarget != skill.target.uid || a.movedRule != rule) {
		move = e.number("moveToSkillTime")
		if a.heroClass == 1 {
			move = e.number("monsterMoveSkillTime")
		}
	}
	skill.begin = e.time + move
	skill.move = move
	skill.beginNotified = false
	skill.end = skill.begin + tableNumber(skill.row, "Duration")
	if skill.row["IsUltimate"] == "1" {
		skill.end += e.number("UltimateEndCameraTime")
		e.ultimateRunning = true
	}
	a.current = skill
	a.state = 1
	// FActor.SkillBegin changes Wait to Skilling. A preceding skill's wait
	// deadline cannot reset position during this newly active skill.
	a.waitUntil = 0
	if skill.row["IsUltimate"] == "1" {
		// FSkill.Do resets the actor's position for an ultimate. Its target is
		// never cached as the location of a completed normal-skill movement.
		a.movedTarget = ""
		a.movedRule = -1
	} else {
		a.movedTarget = skill.target.uid
		a.movedRule = rule
	}
}

func (e *combatRoundEngine) step() error {
	if e.roundOneLiveAutoFormation != nil && !e.roundOneLiveAutoStarted {
		if e.round != 1 || e.camp != 0 || e.time != 0 || !e.originalFormationApplied {
			return errors.New(constUtil.P6ReplayConflict)
		}
		if err := e.buildOriginalRoundAICommands(); err != nil {
			return err
		}
		e.roundOneLiveAutoStarted = true
		e.trace("round1_live_auto_start", nil, nil, "", "", 0)
	}
	e.aiCleanupReturned = false
	// EnginePlayerAI removes Done normal entries before updating actors. A
	// normal completed later in this Tick remains in that queue until next Tick.
	for i := 0; i < len(e.aiDealingNormals); {
		if e.aiDealingNormals[i].state == 4 {
			e.aiDealingNormals = append(e.aiDealingNormals[:i], e.aiDealingNormals[i+1:]...)
		} else {
			i++
		}
	}
	if e.aiUltimateCleanup && len(e.aiDealingNormals) == 0 {
		e.aiUltimateCleanup = false
		e.aiCleanupReturned = true
		e.aiDelay = 0
	}
	if e.captureTicks {
		e.diagnosticCasts = nil
	}
	if e.failure != nil {
		return e.failure
	}
	if e.pendingUltimate != nil && e.pendingUltimate.actor.hp <= 0 {
		e.pendingUltimate.state = 4
		e.pendingUltimate = nil
		e.ultimateRunning = false
	}
	copyNormalIssued := false
	if e.originalManualAutoWaiting() {
		// The original manual player has not run its PlayerAI. Actor/state/buff
		// updates below still execute; time advances through the existing0.1 loop.
	} else if e.originalUsesSuppliedCamp() {
		for e.suppliedIndex < len(e.supplied.SkillOpts) {
			op := e.supplied.SkillOpts[e.suppliedIndex]
			if op.GetRoundNum() != e.round || op.GetRoundTime() > e.time {
				break
			}
			// Native wire stores guid; EnginePlaybackAI reconstructs camp_guid.
			actor := e.find(fmt.Sprintf("%d_%s", op.GetCamp(), op.GetActorUid()), op.GetCamp())
			if actor == nil || op.GetCamp() != e.camp {
				return fmt.Errorf("录像操作不符合原技能时序: op=%d round=%d time=%g actor=%s camp=%d/%d", e.suppliedIndex, e.round, e.time, op.GetActorUid(), op.GetCamp(), e.camp)
			}
			skill := actor.skill(op.GetSkillIndex())
			if !e.issue(skill) {
				skillState, cooldown := int32(-1), int32(-1)
				if skill := actor.skill(op.GetSkillIndex()); skill != nil {
					skillState, cooldown = skill.state, skill.cooldown
				}
				return fmt.Errorf("录像操作不符合原技能时序: op=%d round=%d time=%g actor=%s hp=%g skill=%d state=%d cooldown=%d anger=%g need=%g pending=%t", e.suppliedIndex, e.round, e.time, op.GetActorUid(), actor.hp, op.GetSkillIndex(), skillState, cooldown, e.players[actor.camp].anger, e.number("UTIL_TRIGGER_ANGERCOUNT"), e.pendingUltimate != nil)
			}
			if e.usesCopyEnemyAI(e.camp) {
				if skill.row["IsUltimate"] != "1" {
					e.copyEnemyDealt = append(e.copyEnemyDealt, skill)
					e.commandIndex++
					e.aiDelay = 1
					copyNormalIssued = true
				} else {
					e.copyEnemyCurrentUltimate = skill
				}
			}
			// Original PlaybackAI tracks every issued skill, including ultimates,
			// in one dealingSkills list until its next Update removes Done entries.
			if (skill.row["IsUltimate"] != "1" || e.originalRecordMode) && !e.usesCopyEnemyAI(e.camp) {
				e.aiDealingNormals = append(e.aiDealingNormals, skill)
			}
			e.suppliedIndex++
			if e.originalRecordMode {
				// Original PlaybackAI.Update executes one queued message, clears
				// curMsg and returns; same-time source messages wait one more Tick.
				break
			}
		}
		if e.usesCopyEnemyAI(e.camp) {
			e.stepOriginalCopyEnemyReplayPhase(copyNormalIssued)
		}
	} else if e.usesCopyEnemyAI(e.camp) {
		e.stepOriginalCopyEnemyAI()
	} else if e.aiUltimateCleanup || e.aiCleanupReturned {
		// PlayerAI's cleanup Update returns before the next actor command.
	} else {
		if e.pendingUltimate == nil {
			ready := append([]*combatRoundActor(nil), e.players[e.camp].actors...)
			sort.SliceStable(ready, func(i, j int) bool { return ready[i].position() < ready[j].position() })
			for _, a := range ready {
				if e.canSkill(a.skill(2)) && !(e.usesCopyEnemyAI(e.camp) && a.heroClass == 1) {
					if e.issue(a.skill(2)) {
						break
					}
				}
			}
		}
		if e.pendingUltimate == nil {
			if e.aiDelay > 0 {
				e.aiDelay -= 0.1
			} else if e.commandIndex < len(e.commands) {
				command := e.commands[e.commandIndex]
				e.commandIndex++
				for _, skill := range command.skills {
					copyDealt := e.usesCopyEnemyAI(e.camp) && skill.state == 0
					e.issue(skill)
					// Original CopyEnemyAI adds a state0 message after DoSkill even
					// when it never attached an active skill or produced SkillBegin.
					if copyDealt {
						e.copyEnemyDealt = append(e.copyEnemyDealt, skill)
					}
					if !e.usesCopyEnemyAI(e.camp) {
						e.aiDealingNormals = append(e.aiDealingNormals, skill)
					}
				}
				e.aiDelay = e.number("PlayerAIDeltaTime")
				if e.usesCopyEnemyAI(e.camp) {
					e.aiDelay = e.number("CopyEnemyAIDeltaTime")
				}
			}
		}
	}
	// Original ComputeCore reads BattleData.isAttToDef independently of initiative.
	// Standard playback Clear uses false; native KF local-index1 swaps use true.
	order := []int32{0, 1}
	if e.input.IsAttToDef {
		order = []int32{1, 0}
	}
	for _, camp := range order {
		for _, a := range e.players[camp].actors {
			e.updateBuffs(a)
			if a.speUntil > 0 && e.time >= a.speUntil {
				a.speState = 1
				a.speUntil = 0
				e.trace("special_end", a, nil, "", "", 0)
			}
			if a.hp <= 0 {
				if a.current != nil && !a.current.first {
					a.current.state = 4
					a.current = nil
				}
				if a.current == nil {
					continue
				}
			}
			if a.waitUntil > 0 && e.time >= a.waitUntil {
				a.movedTarget = ""
				a.movedRule = -1
				a.waitUntil = 0
				if a.state < 3 {
					a.state = 0
				}
			}
			// FActor.Update starts its own pending ultimate after earlier actors
			// have updated in this tick. Their final normal skill may unblock it
			// immediately; a check before the player loops adds an extra 0.1s.
			if e.pendingUltimate != nil && e.pendingUltimate.actor == a && !e.ultimateRunning && !e.anyRunning() {
				e.beginSkill(e.pendingUltimate)
				// Original FActor.Update entered with no doUltimateSkill. It starts
				// the ultimate here and updates it on the following actor tick.
				continue
			}
			if a.current == nil && e.pendingUltimate == nil && len(a.waiting) > 0 {
				skill := a.waiting[0]
				a.waiting = a.waiting[1:]
				e.beginSkill(skill)
			}
			skill := a.current
			if skill == nil {
				continue
			}
			if !skill.beginNotified {
				// FSkill.UpdateSkillBegin emits the original callback on its first
				// Update, before the separately timed movement/release offset.
				skill.beginNotified = true
				e.trace("skill_begin", a, skill.target, skill.id, "", skill.move)
			}
			if skill.stage == 0 && e.time >= skill.begin {
				skill.stage = 1
			}
			if skill.stage == 1 {
				for skill.releaseIndex < len(skill.releases) && e.time >= skill.begin+skill.releases[skill.releaseIndex].time {
					release := skill.releases[skill.releaseIndex]
					if release.isCast {
						callback := a.state != 4 && skill.target != nil
						if err := e.release(skill, release.cast); err != nil {
							return err
						}
						// Original FRelease still calls SkillCast for visual-only
						// casts. They have no primary-damage "release" audit event.
						if e.captureTicks && callback {
							e.diagnosticCasts = append(e.diagnosticCasts, map[string]any{"round": e.round, "time": e.time, "skill": skill.id, "cast": release.cast["SkillCastId"]})
						}
					} else {
						// FRelease sets curSkillClass before both projectile launch and actual hit.
						a.curSkillClass = int32(tableNumber(skill.row, "SkillClass"))
						e.trace("projectile_launch", a, skill.target, skill.id, release.cast["SkillCastId"], tableNumber(release.cast, "FlyTime"))
					}
					skill.releaseIndex++
				}
				if skill.releaseIndex == len(skill.releases) {
					skill.stage = 2
					// A release may complete its actor death and finish this skill.
					// Preserve that state instead of overwriting it with the tail stage.
					if skill.state != 4 {
						skill.state = 3
					}
				}
			}
			if skill.stage == 2 && e.time >= skill.end {
				skill.state = 4
				a.current = nil
				e.finishOriginalSkillActorState(a)
				e.trace("skill_end", a, nil, skill.id, "", 0)
				if e.originalRecordMode && e.supplied != nil && !e.originalHasActiveNormalSkill() {
					e.originalPlaybackFinishHint = true
				}
				if skill == e.pendingUltimate {
					e.pendingUltimate = nil
					e.ultimateRunning = false
					e.aiDelay = 0
					if !e.usesCopyEnemyAI(e.camp) && !(e.originalRecordMode && e.originalUsesSuppliedCamp()) {
						e.aiUltimateCleanup = true
					}
				}
				if skill.row["IsUltimate"] == "1" {
					e.originalBreakCombo(e.players[a.camp])
				}
			}
		}
		e.originalMaybeBreakCombo(e.players[camp])
	}
	if err := e.finishPendingDeaths(); err != nil {
		return err
	}
	e.originalFinalizeBuffSnapshots()
	return e.failure
}

// FActor.SkillEnd checks remaining ready normals/queued skills, then DoWait
// preserves a moved position. DoWait at the home position and DoIdle are Idle.
func (e *combatRoundEngine) finishOriginalSkillActorState(a *combatRoundActor) {
	if a.state >= 3 {
		return
	}
	needsAnother := len(a.waiting) > 0
	for _, skill := range a.skills {
		if skill.row["IsUltimate"] != "1" && e.canSkill(skill) {
			needsAnother = true
			break
		}
	}
	if needsAnother && a.movedTarget != "" && a.movedRule != 4 {
		a.state = 2
		a.waitUntil = e.time + e.number("ContinueAttackWaitTimeout")
		return
	}
	a.state = 0
	a.movedTarget = ""
	a.movedRule = -1
	a.waitUntil = 0
}

func (e *combatRoundEngine) release(skill *combatRoundSkill, cast map[string]string) error {
	if err := e.dynamicRelease(skill, cast); err != nil {
		return err
	}
	// Original FRelease.Do02b5/18b6 inserts its completed data at index0.
	// Retain even a cast with empty values/loads/dead, so callback collection is observable.
	e.trace("cast_export", skill.actor, nil, skill.id, cast["SkillCastId"], 0)
	return nil
}

func (e *combatRoundEngine) canFinishRound() bool {
	if e.originalManualAutoWaiting() {
		return false
	}
	// EnginePlayerAI.Update returns at 00ab after ultimate cleanup; FinishRound
	// becomes reachable on its next Update, allowing that Tick's delayed Buffs.
	if e.aiUltimateCleanup || e.aiCleanupReturned || len(e.aiDealingNormals) > 0 {
		return false
	}
	if e.anyRunning() || e.pendingUltimate != nil {
		return false
	}
	for _, a := range e.players[e.camp].actors {
		if a.hp > 0 && len(a.waiting) > 0 {
			return false
		}
	}
	// Original CopyEnemyAI.Update0124..0136 switches an empty normal queue
	// to its ultimate step and returns. With no SkillEnd callback, its empty
	// ultimate step notifies round finish on the next Update, at time0.1;
	// ComputeCore then advances the callback clock to0.2. Preserve both ticks.
	if e.usesCopyEnemyAI(e.camp) && len(e.commands) == 0 && e.time < 0.1 {
		return false
	}
	if e.usesCopyEnemyAI(e.camp) {
		if !e.copyEnemyFinishNotified && !(e.originalRecordMode && e.supplied != nil && e.originalPlaybackFinishHint) {
			// An original no-target normal never reaches the ultimate phase.
			// Its separately checked stall/compatibility action stays explicit.
			return !e.copyEnemyUltimatePhase && e.commandIndex >= len(e.commands) && !e.originalCopyNormalDone()
		}
	}
	if e.originalUsesSuppliedCamp() {
		return e.suppliedIndex >= len(e.supplied.SkillOpts) || e.supplied.SkillOpts[e.suppliedIndex].GetRoundNum() > e.round
	}
	return e.commandIndex >= len(e.commands)
}
func (e *combatRoundEngine) endRound() error {
	for _, p := range e.players {
		for _, a := range p.actors {
			for _, skill := range a.skills {
				skill.cooldown = max(0, skill.cooldown-1)
			}
			if a.state >= 3 {
				a.state = 4
				continue
			}
			if err := e.originalLastAttackRoundEnd(a); err != nil {
				return err
			}
			if err := e.originalBuffRoundEnd(a); err != nil {
				return err
			}
		}
	}
	e.originalFinalizeBuffSnapshots()
	e.trace("round_end", nil, nil, "", "", 0)
	return nil
}

func (e *combatRoundEngine) totalHP(camp int32) float64 {
	total := 0.0
	for _, a := range e.players[camp].actors {
		total += a.hp
	}
	return total
}
func (e *combatRoundEngine) validateOperates(operate *pb.BattleOperate) error {
	lastRound, lastTime := int32(0), 0.0
	for _, op := range operate.SkillOpts {
		if op == nil || op.GetRoundNum() < 1 || op.GetRoundNum() > e.input.MaxRound || op.GetCamp() < 0 || op.GetCamp() > 1 || op.GetSkillIndex() < 0 || op.GetSkillIndex() > 3 || !combatFinite(op.GetRoundTime()) || op.GetRoundTime() < 0 || op.GetRoundTime() > constUtil.P6RoundTimeLimit {
			return errors.New(constUtil.CombatInvalid)
		}
		if op.GetRoundNum() < lastRound || op.GetRoundNum() == lastRound && op.GetRoundTime() < lastTime {
			return errors.New(constUtil.CombatInvalid)
		}
		lastRound, lastTime = op.GetRoundNum(), op.GetRoundTime()
	}
	return e.validateOriginalFormationRecord(operate)
}

// selectTargets restores the original strategy enum and >100 friendly-camp offset.
// Geometry is based on the frozen 5-column FPID, independent of visual transforms.
func (e *combatRoundEngine) selectTargets(source *combatRoundActor, strategy int32, target *combatRoundActor, last []*combatRoundActor) ([]*combatRoundActor, error) {
	return e.selectTargetsWithWeak(source, strategy, target, last, false)
}

// FSkill.Do excludes weak death, while FRelease LimitConditions(1,0,1) includes it.
func (e *combatRoundEngine) selectTargetsWithWeak(source *combatRoundActor, strategy int32, target *combatRoundActor, last []*combatRoundActor, includeWeak bool) ([]*combatRoundActor, error) {
	return e.selectTargetsWithConditions(source, strategy, target, last, includeWeak, false, true)
}

// LimitConditions independently controls weak death, invisibility and invincibility.
func (e *combatRoundEngine) selectTargetsWithConditions(source *combatRoundActor, strategy int32, target *combatRoundActor, last []*combatRoundActor, includeWeak, includeInvisible, includeInvincible bool) ([]*combatRoundActor, error) {
	camp := 1 - source.camp
	if strategy > 100 {
		strategy -= 100
		camp = source.camp
	}
	if strategy == 30 {
		for _, actor := range e.players[camp].deadActors {
			if !actor.rebounded {
				return []*combatRoundActor{actor}, nil
			}
		}
		return nil, nil
	}
	var actors []*combatRoundActor
	for _, a := range e.players[camp].actors {
		if (a.hp > 0 || includeWeak && a.state == 3) && a.state != 4 && (includeInvisible || camp == source.camp || !a.hasState("invisible") || a == target) && (includeInvincible || !a.hasState("invincible")) {
			actors = append(actors, a)
		}
	}
	sort.SliceStable(actors, func(i, j int) bool { return actors[i].position() < actors[j].position() })
	if strategy == 0 {
		return []*combatRoundActor{source}, nil
	}
	// SAME_AS_TARGET selects one held target rather than the previous area target set.
	if includeWeak && strategy == 2 {
		if target != nil && target.state != 4 && (includeInvincible || !target.hasState("invincible")) {
			return []*combatRoundActor{target}, nil
		}
		return nil, nil
	}
	if strategy == 1 || strategy == 2 {
		if len(last) > 0 {
			return last, nil
		}
		if target != nil && (target.hp > 0 || includeWeak && target.state == 3) {
			return []*combatRoundActor{target}, nil
		}
		return nil, nil
	}
	// Native FIRST keeps the selected skill target, even at weak death during its later casts.
	if includeWeak && strategy == 3 && target != nil {
		return []*combatRoundActor{target}, nil
	}
	if len(actors) == 0 {
		// Original GetFirstActor retries only invisibility when its first pass
		// finds nobody. Final/weak death and the invincibility limit stay intact.
		if strategy == 3 && !includeInvisible {
			retryStrategy := strategy
			if camp == source.camp {
				retryStrategy += 100
			}
			return e.selectTargetsWithConditions(source, retryStrategy, target, last, includeWeak, true, includeInvincible)
		}
		return nil, nil
	}
	first := actors[0]
	pick := func(n int, skip map[string]bool) []*combatRoundActor {
		var pool, result []*combatRoundActor
		for _, a := range actors {
			if !skip[a.uid] {
				pool = append(pool, a)
			}
		}
		for n > 0 && len(pool) > 0 {
			index := int(e.rng.Value() * float64(len(pool)))
			result = append(result, pool[index])
			pool = append(pool[:index], pool[index+1:]...)
			n--
		}
		return result
	}
	switch strategy {
	case 3:
		return []*combatRoundActor{first}, nil
	case 4, 5:
		result := []*combatRoundActor{first}
		return append(result, pick(int(strategy-3), map[string]bool{first.uid: true})...), nil
	case 6, 7, 8:
		return pick(int(strategy-5), map[string]bool{}), nil
	case 9:
		return []*combatRoundActor{actors[len(actors)-1]}, nil
	case 10:
		return actors, nil
	case 11, 12, 13:
		// GetRow1ActorsByRow probes the preferred row, then its original
		// fallback order. Empty middle/back rows still select a live front row.
		rows := []int32{1, 2, 3}
		if strategy == 12 {
			rows = []int32{2, 1, 3}
		} else if strategy == 13 {
			rows = []int32{3, 2, 1}
		}
		for _, wantedRow := range rows {
			var result []*combatRoundActor
			for _, a := range actors {
				if (a.position()-1)/5+1 == wantedRow {
					result = append(result, a)
				}
			}
			if len(result) > 0 {
				return result, nil
			}
		}
		return nil, nil
	case 14, 15:
		var result []*combatRoundActor
		for _, a := range actors {
			row := (a.position()-1)/5 + 1
			if strategy == 14 && row <= 2 || strategy == 15 && row >= 2 {
				result = append(result, a)
			}
		}
		return result, nil
	case 25:
		// Original MORE_HP_THAN_SELF returns every eligible actor above the owner HP.
		var result []*combatRoundActor
		for _, a := range actors {
			if a.hp > source.hp {
				result = append(result, a)
			}
		}
		return result, nil
	case 16, 17, 26, 27, 28, 31:
		selected := actors[0]
		score := func(a *combatRoundActor) float64 {
			switch strategy {
			case 16, 17:
				return a.hp
			case 26:
				return a.attribute("att")
			case 27:
				return -a.attribute("pdef")
			case 28:
				return -a.attribute("mdef")
			default:
				return a.attribute("maxHp") - a.hp
			}
		}
		for _, a := range actors[1:] {
			if strategy == 16 && score(a) < score(selected) || strategy != 16 && score(a) > score(selected) {
				selected = a
			}
		}
		return []*combatRoundActor{selected}, nil
	case 18:
		column := (source.position() - 1) % 5
		for _, a := range actors {
			if (a.position()-1)%5 == column {
				return []*combatRoundActor{a}, nil
			}
		}
		return []*combatRoundActor{first}, nil
	case 19, 20, 21, 22, 23, 32, 33:
		center := first
		if strategy == 23 {
			center = actors[len(actors)-1]
		}
		if strategy == 22 || strategy == 32 {
			column := (source.position() - 1) % 5
			for _, a := range actors {
				if (a.position()-1)%5 == column {
					center = a
					break
				}
			}
		}
		var result []*combatRoundActor
		cx, cy := (center.position()-1)%5, (center.position()-1)/5
		for _, a := range actors {
			x, y := (a.position()-1)%5, (a.position()-1)/5
			dx, dy := int32(math.Abs(float64(x-cx))), int32(math.Abs(float64(y-cy)))
			if strategy == 19 && x == cx || strategy == 20 && x == cx && dy <= 1 || strategy >= 21 && strategy <= 23 && dx <= 1 && dy <= 1 || strategy == 32 && x == cx && y <= cy || strategy == 33 && dx+dy <= 1 {
				result = append(result, a)
			}
		}
		return result, nil
	case 24:
		return pick(1, map[string]bool{}), nil
	default:
		return nil, fmt.Errorf("%s: target strategy %s", constUtil.CombatUnsupported, strconv.Itoa(int(strategy)))
	}
}
