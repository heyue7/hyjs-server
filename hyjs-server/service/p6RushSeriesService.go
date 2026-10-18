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

// FreezeP6RushSeriesInput follows BattleData.SetData(SCRushFightStart,bool):
// true9/false10, attackerFirst, Clear factors1/1 and every monster in source order.
// Nonzero business win criteria need separate original rule certification.
func (s *GameService) FreezeP6RushSeriesInput(start *pb.SCRushFightStart, trial, automatic bool, properties map[string][]string, clientVersion string) (*model.FrozenP6RushSeries, error) {
	if start == nil || start.BattlePlayer == nil || start.BattlePlayer.Inst == nil || len(start.BattleMonsterList) == 0 {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if start.GetWinKey() != 0 || start.GetWinValue() != 0 {
		return nil, errors.New(constUtil.P6RushWinConditionUnavailable)
	}
	if !automatic && len(start.BattleMonsterList) > 1 {
		return nil, errors.New(constUtil.P6RushManualSeriesUnavailable)
	}
	source := proto.Clone(start).(*pb.SCRushFightStart)
	maximum := source.GetMaxRound()
	if maximum <= 0 {
		maximum = 40
	}
	mode := int32(10)
	if trial {
		mode = 9
	}
	input, err := s.FreezeBattleInput(model.FrozenBattleInput{Attacker: source.BattlePlayer, Monster: source.BattleMonsterList[0], Seed: source.GetRandSeed(), MaxRound: maximum, BattleType: mode, AppliedProperties: properties, ClientVersion: clientVersion})
	if err != nil {
		return nil, err
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(source)
	if err != nil || len(raw) > constUtil.P6StoredInputMaxBytes {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	out := &model.FrozenP6RushSeries{Schema: constUtil.P6RushSeriesSchema, Start: source, StartSHA256: p6Digest(raw), Trial: trial, PlayerAutomatic: automatic, Template: input}
	if err = validateP6RushSeries(out); err != nil {
		return nil, err
	}
	return out, nil
}

func validateP6RushSeries(input *model.FrozenP6RushSeries) error {
	if input == nil || input.Schema != constUtil.P6RushSeriesSchema || input.Start == nil || input.Template == nil || input.Start.BattlePlayer == nil || input.Start.BattlePlayer.Inst == nil || len(input.Start.BattleMonsterList) == 0 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	source := input.Start
	if source.GetWinKey() != 0 || source.GetWinValue() != 0 {
		return errors.New(constUtil.P6RushWinConditionUnavailable)
	}
	if !input.PlayerAutomatic && len(source.BattleMonsterList) > 1 {
		return errors.New(constUtil.P6RushManualSeriesUnavailable)
	}
	raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(source)
	if err != nil || len(raw) > constUtil.P6StoredInputMaxBytes || p6Digest(raw) != input.StartSHA256 {
		return errors.New(constUtil.P6ReplayConflict)
	}
	maximum := source.GetMaxRound()
	if maximum <= 0 {
		maximum = 40
	}
	mode := int32(10)
	if input.Trial {
		mode = 9
	}
	base := input.Template
	if base.BattleType != mode || base.Defender != nil || base.KFReplay != nil || base.DefenderFirst || base.IsAttToDef || len(base.WinConditions) != 0 || base.AttackerDamageFactor != nil || base.DefenderDamageFactor != nil || base.Seed != source.GetRandSeed() || base.MaxRound != maximum || !proto.Equal(base.Attacker, source.BattlePlayer) || !proto.Equal(base.Monster, source.BattleMonsterList[0]) {
		return errors.New(constUtil.P6ReplayConflict)
	}
	// Validate every later wave now, including one hidden behind an early loss.
	for _, monster := range source.BattleMonsterList {
		// Original BattleFighterData directly dereferences Monster.Inst. A
		// missing message must fail here instead of Go silently defaulting anger.
		if monster == nil || monster.Inst == nil {
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

// ComputeP6RushSeries reinitializes original static fighters and the same global
// seed per wave. Final-only protocol records cannot supply missing manual waves.
// This API never settles P7 points, stars, budgets or authoritative winner facts.
func (s *GameService) ComputeP6RushSeries(ctx context.Context, input *model.FrozenP6RushSeries, finalRecord *pb.BattleOperate) (*model.P6RushSeriesOutcome, error) {
	if ctx == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	if err := validateP6RushSeries(input); err != nil {
		return nil, err
	}
	if !input.PlayerAutomatic && finalRecord == nil {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	base, err := cloneP6CalculationInput(input.Template)
	if err != nil {
		return nil, err
	}
	start := proto.Clone(input.Start).(*pb.SCRushFightStart)
	result := &model.P6RushSeriesOutcome{Schema: input.Schema, StartSHA256: input.StartSHA256, Winner: 1}
	for index, monster := range start.BattleMonsterList {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		wave := *base
		wave.Monster = monster
		raw, encodeErr := json.Marshal(&wave)
		if encodeErr != nil {
			return nil, encodeErr
		}
		var sourceRecord *pb.BattleOperate
		if !input.PlayerAutomatic {
			sourceRecord = finalRecord
		}
		computed, computeErr := s.ComputeOriginalP6ProtocolBattle(ctx, &wave, sourceRecord)
		if computeErr != nil {
			return nil, computeErr
		}
		result.Fights = append(result.Fights, model.P6CopyFightOutcome{AttackerWave: 0, DefenderWave: int64(index), InputSHA256: p6Digest(raw), Outcome: computed.Outcome})
		result.FinalOperates = computed.OriginalOperates
		result.Winner = computed.Outcome.Winner
		if result.Winner != 0 {
			result.AttackerWave = 1
			result.DefenderWave = int64(index)
			break
		}
		result.DefenderWave = int64(index + 1)
	}
	if finalRecord != nil && input.PlayerAutomatic && !p6OriginalOperateEqual(finalRecord, result.FinalOperates) {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	return result, nil
}
