package service

import (
	"context"
	"encoding/json"
	"errors"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// FreezeP6CopySeriesInput closes the original SCCopyFightStart monster list and
// automatic-player state explicitly. Manual multiwave records require earlier
// operations absent from the original final-only protocol and remain unavailable.
func (s *GameService) FreezeP6CopySeriesInput(start *pb.SCCopyFightStart, automatic bool, properties map[string][]string, clientVersion, compatibilityRule string) (*model.FrozenP6CopySeries, error) {
	if !automatic {
		return nil, errors.New(constUtil.P6CopyManualSeriesUnavailable)
	}
	if start == nil || start.GetBattlePlayer() == nil || len(start.GetBattleMonsterList()) == 0 {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	maximum := start.GetMaxRound()
	if maximum <= 0 {
		maximum = 40
	}
	source := proto.Clone(start).(*pb.SCCopyFightStart)
	first, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: source.BattlePlayer, Monster: source.BattleMonsterList[0], Seed: source.GetRandSeed(), MaxRound: maximum, BattleType: 1, AppliedProperties: properties, ClientVersion: clientVersion, PVECompatibilityRule: compatibilityRule})
	if err != nil {
		return nil, err
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(source)
	if err != nil || len(raw) > constUtil.P6StoredInputMaxBytes {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	series := &model.FrozenP6CopySeries{Schema: constUtil.P6CopySeriesSchema, Start: source, StartSHA256: p6Digest(raw), PlayerAutomatic: true, Template: first}
	// Validate every frozen wave before any calculation. A bad later monster
	// must fail the whole source instead of being hidden behind an early loss.
	if err = validateP6CopySeries(series); err != nil {
		return nil, err
	}
	return series, nil
}

func validateP6CopySeries(input *model.FrozenP6CopySeries) error {
	if input == nil || input.Schema != constUtil.P6CopySeriesSchema || input.Start == nil || input.Template == nil || !input.PlayerAutomatic || len(input.Start.GetBattleMonsterList()) == 0 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(input.Start)
	if err != nil || len(raw) > constUtil.P6StoredInputMaxBytes || p6Digest(raw) != input.StartSHA256 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	base := input.Template
	maximum := input.Start.GetMaxRound()
	if maximum <= 0 {
		maximum = 40
	}
	if base.BattleType != 1 || base.Defender != nil || base.KFReplay != nil || base.DefenderFirst || base.IsAttToDef || len(base.WinConditions) != 0 || base.AttackerDamageFactor != nil || base.DefenderDamageFactor != nil || base.Seed != input.Start.GetRandSeed() || base.MaxRound != maximum || !proto.Equal(base.Attacker, input.Start.BattlePlayer) || !proto.Equal(base.Monster, input.Start.BattleMonsterList[0]) {
		return errors.New(constUtil.P6ReplayConflict)
	}
	for _, monster := range input.Start.BattleMonsterList {
		if monster == nil {
			return errors.New(constUtil.CombatInvalid)
		}
		wave := *base
		wave.Monster = monster
		if err = validateFrozenP6(&wave); err != nil {
			return err
		}
	}
	return nil
}

// ComputeP6CopySeries follows BattleManager.FightOver: a camp0 win advances the
// defender wave; a loss exhausts the single attacker. EngineDefault.SetFightData
// creates a new ComputeCore with the original static fighter and same global seed.
// FinalOperates is the last ComputeCore only, as original BattleNetOver reads it.
func (s *GameService) ComputeP6CopySeries(ctx context.Context, input *model.FrozenP6CopySeries, finalRecord *pb.BattleOperate) (*model.P6CopySeriesOutcome, error) {
	if ctx == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if err := validateP6CopySeries(input); err != nil {
		return nil, err
	}
	// Clone before running any fight so all wave sources and output metadata are
	// isolated from caller mutation and previous computed fighter HP/AP/Buffs.
	base, err := cloneP6CalculationInput(input.Template)
	if err != nil {
		return nil, err
	}
	start := proto.Clone(input.Start).(*pb.SCCopyFightStart)
	result := &model.P6CopySeriesOutcome{Schema: input.Schema, StartSHA256: input.StartSHA256, Winner: 1}
	for index, monster := range start.BattleMonsterList {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		wave := *base
		wave.Monster = monster
		encoded, encodeErr := json.Marshal(&wave)
		if encodeErr != nil {
			return nil, encodeErr
		}
		// The original last-only record cannot drive earlier wave skills. Derive
		// all automatic waves, then strictly validate its terminal identity.
		outcome, computeErr := s.ComputeFrozenBattle(ctx, &wave, nil)
		if computeErr != nil {
			return nil, computeErr
		}
		result.Fights = append(result.Fights, model.P6CopyFightOutcome{AttackerWave: 0, DefenderWave: int64(index), InputSHA256: p6Digest(encoded), Outcome: outcome})
		result.FinalOperates, err = OriginalP6OperateProjection(wave.BattleType, outcome.Operates)
		if err != nil {
			return nil, err
		}
		result.Winner = outcome.Winner
		if outcome.Winner != 0 {
			result.AttackerWave = 1
			result.DefenderWave = int64(index)
			break
		}
		result.DefenderWave = int64(index + 1)
	}
	if finalRecord != nil && !p6OriginalOperateEqual(finalRecord, result.FinalOperates) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	return result, nil
}
