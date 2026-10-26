package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// ComputeFrozenBattleFinalHP exposes the exact terminal FActor maxHp/currentHp
// for explicitly requested audit consumers. It never derives maxHP from initial
// panel Prop.Hp or writes participant wallets, FightInfo, HP, scores or winners.
func (s *GameService) ComputeFrozenBattleFinalHP(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate) (*model.CombatFinalHPOutcome, error) {
	return s.computeFrozenBattleFinalHP(ctx, input, supplied, false)
}

// ComputeOriginalP6ProtocolFinalHP explicitly audits the original PVE record
// driver, including its recorded formation change. Source and runtime slots
// remain separate. OriginalOperates comes from the same calculation as the final
// actors; this does not apply HP or promote settlement authority.
func (s *GameService) ComputeOriginalP6ProtocolFinalHP(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate) (*model.CombatFinalHPOutcome, error) {
	if err := validateOriginalP6ProtocolInput(input, supplied); err != nil {
		return nil, err
	}
	return s.computeFrozenBattleFinalHP(ctx, input, supplied, true)
}

func (s *GameService) computeFrozenBattleFinalHP(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate, originalRecordMode bool) (*model.CombatFinalHPOutcome, error) {
	return s.computeFrozenBattleFinalHPControlled(ctx, input, supplied, originalRecordMode, nil)
}

// The explicit formation control is opt-in; all existing callers retain their
// original supplied-record driver and exact source digest with nil control.
func (s *GameService) computeFrozenBattleFinalHPControlled(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate, originalRecordMode bool, formation *pb.BattleTransOperate) (*model.CombatFinalHPOutcome, error) {
	return s.computeFrozenBattleFinalHPRoundControlled(ctx, input, supplied, originalRecordMode, formation, nil)
}

func (s *GameService) computeFrozenBattleFinalHPRoundControlled(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate, originalRecordMode bool, formation *pb.BattleTransOperate, controls []p6RoundAutoControl) (*model.CombatFinalHPOutcome, error) {
	return s.computeFrozenBattleFinalHPLiveControlled(ctx, input, supplied, originalRecordMode, formation, controls, false)
}

func (s *GameService) computeFrozenBattleFinalHPLiveControlled(ctx context.Context, input *model.FrozenBattleInput, supplied *pb.BattleOperate, originalRecordMode bool, formation *pb.BattleTransOperate, controls []p6RoundAutoControl, liveRecorded bool) (*model.CombatFinalHPOutcome, error) {
	if ctx == nil || input == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	frozen, err := cloneP6CalculationInput(input)
	if err != nil {
		return nil, err
	}
	// The checked configuration digest closes all table rows. Hash the remaining
	// exact frozen parameters, actors, and AppliedProperties without serializing
	// a second copy of the multi-megabyte table set for an opt-in audit digest.
	closure := *frozen
	closure.Tables = nil
	encoded, err := json.Marshal(&closure)
	if err != nil {
		return nil, err
	}
	result := &model.CombatFinalHPOutcome{Schema: constUtil.P6FinalActorHPSchema, CalculationSourceSHA256: p6Digest(encoded), ConfigurationHash: frozen.ConfigurationHash, EngineVersion: frozen.EngineVersion, ClientVersion: frozen.ClientVersion}
	if originalRecordMode && supplied != nil {
		// A recorded formation/press sequence is a calculation input. Close its
		// canonical source bytes alongside the frozen participant/table digest.
		record, marshalErr := proto.MarshalOptions{Deterministic: true}.Marshal(supplied)
		if marshalErr != nil {
			return nil, marshalErr
		}
		result.OriginalRecordSHA256 = p6Digest(record)
		bound, marshalErr := json.Marshal(struct {
			FrozenInput    string `json:"frozenInput"`
			OriginalRecord string `json:"originalRecord"`
		}{result.CalculationSourceSHA256, result.OriginalRecordSHA256})
		if marshalErr != nil {
			return nil, marshalErr
		}
		result.CalculationSourceSHA256 = p6Digest(bound)
	}
	formations := [][]*pb.BattleHero{frozen.Attacker.GetFormation()}
	if frozen.Defender != nil {
		formations = append(formations, frozen.Defender.GetFormation())
	} else {
		formations = append(formations, frozen.Monster.GetFormation())
	}
	initial := map[string]string{}
	for camp, actors := range formations {
		for _, actor := range actors {
			raw, marshalErr := proto.MarshalOptions{Deterministic: true}.Marshal(actor)
			if marshalErr != nil {
				return nil, marshalErr
			}
			initial[fmt.Sprintf("%d_%s", camp, actor.GetGuid())] = p6Digest(raw)
		}
	}
	terminal := func(engine *combatRoundEngine) {
		for _, player := range engine.players {
			for _, actor := range player.actors {
				runtimePose := int32(0)
				if actor.position() != actor.hero.GetPose() {
					runtimePose = actor.position()
				}
				result.FinalActors = append(result.FinalActors, model.CombatFinalActorHP{Camp: actor.camp, UID: actor.uid, GUID: actor.hero.GetGuid(), HeroID: actor.hero.GetId(), Pose: actor.hero.GetPose(), RuntimePose: runtimePose, BloodNumber: actor.hero.GetBloodNumber(), CurrentHP: actor.hp, MaxHP: actor.attribute("maxHp"), State: actor.state, InitialActorSHA256: initial[actor.uid]})
			}
		}
	}
	driverRecord := supplied
	if formation != nil {
		// Keep the full expected record bound above; live AI does not consume it.
		driverRecord = nil
	}
	result.Outcome, err = s.computeFrozenBattleRecordLiveControlled(ctx, frozen, driverRecord, nil, terminal, originalRecordMode, formation, controls, liveRecorded)
	if err != nil {
		return nil, err
	}
	if originalRecordMode {
		// Project the actual completed command container before returning its
		// terminal HP snapshot; consumers need no second battle calculation.
		result.OriginalOperates, err = OriginalP6OperateProjection(frozen.BattleType, result.Outcome.Operates)
		if err != nil {
			return nil, err
		}
	}
	for _, actor := range result.FinalActors {
		if actor.InitialActorSHA256 == "" || !combatFinite(actor.CurrentHP) || !combatFinite(actor.MaxHP) {
			return nil, errors.New(constUtil.P6ReplayConflict)
		}
	}
	return result, nil
}
