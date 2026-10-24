package service

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/reflect/protoreflect"
	"io"
	"math"
	"reflect"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitializeP6 建立公共实例/录像，先算后播默认关闭，部署不改变旧单人结算规则。
func (s *GameService) InitializeP6(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P6BattleRecord{}, &db.P6CombatRule{}); err != nil {
		return err
	}
	rule := db.P6CombatRule{ID: 1, MaxRounds: 40, TimeoutMillis: 900000, ClientHash: constUtil.P6OriginalClientHash, Source: "原APK最大40回合；本地实例15分钟；权威计算需机制与原客户端对照，默认关闭"}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&rule).Error
}

func p6Digest(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

// encodeP6StoredInput keeps the canonical JSON hash while reducing new database blobs.
func encodeP6StoredInput(raw []byte) ([]byte, error) {
	if len(raw) > constUtil.P6StoredInputMaxBytes {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	if len(raw) < constUtil.P6StoredInputCompressionThreshold {
		return raw, nil
	}
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
	if err != nil {
		return nil, err
	}
	if _, err = writer.Write(raw); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	if compressed.Len() >= len(raw) {
		return raw, nil
	}
	return compressed.Bytes(), nil
}

// decodeP6StoredInput accepts historical JSON and one checked gzip member with bounded expansion.
func decodeP6StoredInput(stored []byte) ([]byte, error) {
	if len(stored) > constUtil.P6StoredInputMaxBytes {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	if len(stored) < 2 || stored[0] != 0x1f || stored[1] != 0x8b {
		return stored, nil
	}
	reader := bytes.NewReader(stored)
	compressed, err := gzip.NewReader(reader)
	if err != nil {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	compressed.Multistream(false)
	raw, readErr := io.ReadAll(io.LimitReader(compressed, constUtil.P6StoredInputMaxBytes+1))
	closeErr := compressed.Close()
	if readErr != nil || closeErr != nil || len(raw) > constUtil.P6StoredInputMaxBytes || reader.Len() != 0 {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	return raw, nil
}

// CompactLegacyP6BattleInputs rewrites historical plaintext P6 inputs to the same gzip
// representation used for new records. The primary-key cursor reads at most eight rows per
// batch, and each eligible row is rechecked under a short row lock before only input changes.
// Invalid or unknown payloads stay untouched; repeated calls make no further changes.
func (s *GameService) CompactLegacyP6BattleInputs(ctx context.Context) (*model.P6StorageCompactionResult, error) {
	if ctx == nil {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	const batchSize = 8
	stats := &model.P6StorageCompactionResult{}
	cursor := ""
	for {
		// Stop before issuing the next batch so cancellation does not wait for a full scan.
		if err := ctx.Err(); err != nil {
			return stats, err
		}
		rows := make([]db.P6BattleRecord, 0, batchSize)
		query := db.GetIns().WithContext(ctx).Select("id, input, input_hash").Order("id ASC").Limit(batchSize)
		if cursor != "" {
			query = query.Where("id > ?", cursor)
		}
		if err := query.Find(&rows).Error; err != nil {
			return stats, err
		}
		if len(rows) == 0 {
			return stats, nil
		}
		for i := range rows {
			if err := ctx.Err(); err != nil {
				return stats, err
			}
			stats.Scanned++
			compacted, eligible, invalid := compactP6LegacyInput(&rows[i])
			if invalid {
				stats.SkippedInvalid++
				continue
			}
			if !eligible {
				continue
			}
			changed, err := persistP6CompactedInput(ctx, &rows[i], compacted)
			if err != nil {
				return stats, err
			}
			if !changed {
				continue
			}
			stats.Compacted++
			stats.BytesBefore += int64(len(rows[i].Input))
			stats.BytesAfter += int64(len(compacted))
		}
		cursor = rows[len(rows)-1].ID
		if len(rows) < batchSize {
			return stats, nil
		}
	}
}

// compactP6LegacyInput only accepts a larger plaintext JSON blob whose canonical hash matches
// the stored identity. Existing gzip and small records are never decode candidates.
func compactP6LegacyInput(row *db.P6BattleRecord) ([]byte, bool, bool) {
	if row == nil || len(row.Input) <= constUtil.P6StoredInputCompressionThreshold || (len(row.Input) >= 2 && row.Input[0] == 0x1f && row.Input[1] == 0x8b) {
		return nil, false, false
	}
	raw, err := decodeP6StoredInput(row.Input)
	if err != nil {
		return nil, false, true
	}
	if !json.Valid(raw) || p6Digest(raw) != row.InputHash {
		return nil, false, true
	}
	stored, err := encodeP6StoredInput(raw)
	if err != nil || len(stored) >= len(row.Input) {
		return nil, false, false
	}
	// Verify the exact stored bytes decode back to the original canonical JSON before writing.
	restored, err := decodeP6StoredInput(stored)
	if err != nil || !bytes.Equal(restored, raw) {
		return nil, false, true
	}
	return stored, true, false
}

// persistP6CompactedInput locks one primary key and confirms the scanned hash and input bytes
// still match before updating only the input column. Concurrent changes leave the row untouched.
func persistP6CompactedInput(ctx context.Context, row *db.P6BattleRecord, compacted []byte) (bool, error) {
	changed := false
	// Suppress the per-row SQL echo because the bound input blob is large and must stay out of logs.
	err := db.GetIns().WithContext(ctx).Session(&gorm.Session{Logger: logger.Discard}).Transaction(func(tx *gorm.DB) error {
		var current db.P6BattleRecord
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id, input, input_hash").Where("id=?", row.ID).First(&current).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if current.InputHash != row.InputHash || !bytes.Equal(current.Input, row.Input) {
			return nil
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		result := tx.Model(&db.P6BattleRecord{}).Where("id=? AND input_hash=?", row.ID, row.InputHash).Update("input", compacted)
		if result.Error != nil {
			return result.Error
		}
		changed = result.RowsAffected == 1
		return nil
	})
	return changed, err
}

// decodeP6Record verifies both immutable content and its persisted identity before reuse.
func decodeP6Record(record *db.P6BattleRecord) (*model.FrozenBattleInput, error) {
	var input model.FrozenBattleInput
	if record == nil {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	raw, err := decodeP6StoredInput(record.Input)
	if err != nil || p6Digest(raw) != record.InputHash || json.Unmarshal(raw, &input) != nil || input.Attacker.GetPlayerId() != record.PlayerID || input.Seed != record.Seed || input.ConfigurationHash != record.ConfigurationHash || input.EngineVersion != record.EngineVersion || input.ClientVersion != record.ClientVersion {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	tables, err := json.Marshal(input.Tables)
	if err != nil || p6Digest(tables) != input.ConfigurationHash || validateFrozenP6(&input) != nil {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	return &input, nil
}

// FreezeBattleInput 复制双方实例及战斗相关原表，配置修改不能改变已创建战斗。
func (s *GameService) FreezeBattleInput(input model.FrozenBattleInput) (*model.FrozenBattleInput, error) {
	if input.Attacker == nil || input.Attacker.GetPlayerId() <= 0 || input.MaxRound < 1 || input.MaxRound > 40 || input.BattleType < 0 || input.BattleType > 20 || (input.Defender == nil) == (input.Monster == nil) {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	out := input
	if input.KFReplayViewer != nil {
		viewer := *input.KFReplayViewer
		out.KFReplayViewer = &viewer
	}
	if input.DiagnosticManualAutoStartRoundTime != nil {
		out.DiagnosticManualAutoStartRoundTime = proto.Float64(*input.DiagnosticManualAutoStartRoundTime)
	}
	if input.AttackerDamageFactor != nil {
		out.AttackerDamageFactor = proto.Float64(*input.AttackerDamageFactor)
	}
	if input.DefenderDamageFactor != nil {
		out.DefenderDamageFactor = proto.Float64(*input.DefenderDamageFactor)
	}
	out.Attacker = proto.Clone(input.Attacker).(*pb.BattlePlayer)
	if input.Defender != nil {
		out.Defender = proto.Clone(input.Defender).(*pb.BattlePlayer)
	}
	if input.Monster != nil {
		out.Monster = proto.Clone(input.Monster).(*pb.BattleMonster)
	}
	if input.KFReplay != nil {
		out.KFReplay = proto.Clone(input.KFReplay).(*pb.KFReplayData)
	}
	out.WinConditions = nil
	for _, condition := range input.WinConditions {
		if condition == nil {
			return nil, errors.New(constUtil.CombatInvalid)
		}
		out.WinConditions = append(out.WinConditions, proto.Clone(condition).(*pb.BattleWinCondition))
	}
	out.Tables = map[string][]map[string]string{}
	// 原表顺序参与hash与同时点的释放排序；只冻结引擎输入，不带账号或背包。
	names := []string{"Hero", "Skill", "SkillCast", "Buff", "Talent", "GameConfigure", "CopyEnemy", "BuffInfExtend", "HeroAwakenAdd", "BootHero", "RecruitShowHero"}
	// WantedEnemy supplies the exact original type8 alternate skill triples.
	// Other modes retain their established frozen table set and digest.
	if input.BattleType == 8 {
		names = append(names, "WantedEnemy")
	}
	for _, name := range names {
		for _, row := range s.Tables[name] {
			clone := map[string]string{}
			for key, value := range row {
				clone[key] = value
			}
			out.Tables[name] = append(out.Tables[name], clone)
		}
	}
	for _, name := range []string{"Hero", "Skill", "SkillCast", "Buff", "Talent", "GameConfigure"} {
		if len(out.Tables[name]) == 0 {
			return nil, fmt.Errorf("%s: %s", constUtil.CombatInvalid, name)
		}
	}
	// Preserve reviewed combat sources when unrelated startup catalog rows differ.
	// 摘要只在本次Freeze内复用；任一normalizer实际赋值后立即失效重算。
	encoded, err := json.Marshal(out.Tables)
	if err != nil {
		return nil, err
	}
	tableHash := p6Digest(encoded)
	if normalizeUnusedThreeOrangeCombatRowsWithHash(&out, input.ConfigurationHash, tableHash) {
		encoded, err = json.Marshal(out.Tables)
		if err != nil {
			return nil, err
		}
		tableHash = p6Digest(encoded)
	}
	if p15NormalizeUnusedGreenCombatRowsWithHash(&out, input.ConfigurationHash, tableHash) {
		encoded, err = json.Marshal(out.Tables)
		if err != nil {
			return nil, err
		}
		tableHash = p6Digest(encoded)
	}
	out.AppliedProperties = map[string][]string{}
	for key, ids := range input.AppliedProperties {
		out.AppliedProperties[key] = append([]string(nil), ids...)
	}
	out.ConfigurationHash = tableHash
	out.EngineVersion = constUtil.P6EngineVersion
	if out.ClientVersion == "" {
		out.ClientVersion = constUtil.P6OriginalClientHash
	}
	if err = validateFrozenP6(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

func validateFrozenP6(input *model.FrozenBattleInput) error {
	if input != nil && input.KFReplay == nil && input.KFReplayViewer != nil {
		return errors.New(constUtil.CombatInvalid)
	}
	if input == nil || input.Attacker == nil || input.Attacker.GetPlayerId() <= 0 || input.MaxRound < 1 || input.MaxRound > 40 || input.BattleType < 0 || input.BattleType > 20 || (input.Defender == nil) == (input.Monster == nil) {
		return errors.New(constUtil.CombatInvalid)
	}
	// 当前版本校验原先手位；历史冻结输入保持原字节并由版本门禁阻止重算。
	if input.EngineVersion == constUtil.P6EngineVersion {
		if err := ValidateP6AIDriverPolicy(input); err != nil {
			return err
		}
		if err := ValidateP6PVECompatibility(*input); err != nil {
			return err
		}
		if err := ValidateP6DamageFactors(input); err != nil {
			return err
		}
		if err := ValidateP6FirstOrder(input); err != nil {
			return err
		}
	}
	camps := [][]*pb.BattleHero{input.Attacker.GetFormation()}
	if input.Defender != nil {
		camps = append(camps, input.Defender.GetFormation())
	} else if input.Monster != nil {
		camps = append(camps, input.Monster.GetFormation())
	} else {
		return errors.New(constUtil.CombatInvalid)
	}
	for camp, actors := range camps {
		if len(actors) < 1 || len(actors) > constUtil.P6MaxActors {
			return errors.New(constUtil.CombatInvalid)
		}
		guids, positions := map[string]bool{}, map[int32]bool{}
		for _, actor := range actors {
			if actor == nil || actor.GetGuid() == "" || actor.GetId() == "" || actor.GetLevel() < 1 || actor.GetPose() < 1 || actor.GetPose() > 15 || actor.Prop == nil || actor.Inst == nil || guids[actor.GetGuid()] || positions[actor.GetPose()] {
				return errors.New(constUtil.CombatInvalid)
			}
			// 初始零HP保留源阵容及录像；原引擎不把此实例准入运行阵容。
			p := actor.GetProp()
			for _, value := range []float64{p.GetHp(), p.GetAtt(), p.GetPdef(), p.GetMdef(), p.GetCri(), p.GetAntiCri(), p.GetHit(), p.GetDodge(), p.GetCriFactor(), p.GetDamageAdd(), p.GetDamageReduce(), actor.GetInst().GetCurHp()} {
				if !combatFinite(value) || math.Abs(value) > 1e15 {
					return errors.New(constUtil.CombatInvalid)
				}
			}
			valid := true
			p.ProtoReflect().Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
				if f.Kind() == protoreflect.DoubleKind && (!combatFinite(v.Float()) || math.Abs(v.Float()) > 1e15) {
					valid = false
				}
				return valid
			})
			if !valid {
				return errors.New(constUtil.CombatInvalid)
			}
			if p.GetHp() <= 0 || actor.GetInst().GetCurHp() < 0 || actor.GetInst().GetCurHp() > p.GetHp() {
				return errors.New(constUtil.CombatInvalid)
			}
			guids[actor.GetGuid()] = true
			positions[actor.GetPose()] = true
		}
		anger := 0.0
		if camp == 0 {
			anger = input.Attacker.GetInst().GetAnger()
		} else if input.Defender != nil {
			anger = input.Defender.GetInst().GetAnger()
		} else {
			anger = input.Monster.GetInst().GetAnger()
		}
		if !combatFinite(anger) || anger < 0 || anger > 300 {
			return errors.New(constUtil.CombatInvalid)
		}
	}
	return nil
}

// CaptureBattleReplay 由已锁Profile的原战斗开始事务调用，冻结输入并保留来源回执。
func (s *GameService) CaptureBattleReplay(ctx context.Context, tx *gorm.DB, playerID int64, source, fightID string, input model.FrozenBattleInput) (string, error) {
	if tx == nil || ctx == nil || playerID <= 0 || input.Attacker.GetPlayerId() != playerID || source == "" || len(source) > 32 || fightID == "" || len(fightID) > 128 {
		return "", errors.New(constUtil.CombatInvalid)
	}
	key := fmt.Sprintf("%d/%s/%s", playerID, source, fightID)
	id := p6Digest([]byte(key))
	var previous db.P6BattleRecord
	err := tx.WithContext(ctx).Where("player_id=? AND source=? AND source_fight=?", playerID, source, fightID).First(&previous).Error
	if err == nil {
		saved, decodeErr := decodeP6Record(&previous)
		if decodeErr != nil || saved.Seed != input.Seed || saved.MaxRound != input.MaxRound || saved.BattleType != input.BattleType || saved.PVECompatibilityRule != input.PVECompatibilityRule || !proto.Equal(saved.Attacker, input.Attacker) || !proto.Equal(saved.Defender, input.Defender) || !proto.Equal(saved.Monster, input.Monster) || !proto.Equal(saved.KFReplay, input.KFReplay) || saved.DefenderFirst != input.DefenderFirst || saved.IsAttToDef != input.IsAttToDef || !reflect.DeepEqual(saved.AttackerDamageFactor, input.AttackerDamageFactor) || !reflect.DeepEqual(saved.DefenderDamageFactor, input.DefenderDamageFactor) || !reflect.DeepEqual(saved.AppliedProperties, input.AppliedProperties) && !(len(saved.AppliedProperties) == 0 && len(input.AppliedProperties) == 0) || !p6ConditionsEqual(saved.WinConditions, input.WinConditions) {
			return "", errors.New(constUtil.P6ReplayConflict)
		}
		// 重试仅核对原双方/seed，不用最新技能表替换该场冻结版本。
		return previous.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}
	frozen, err := s.FreezeBattleInput(input)
	if err != nil {
		return "", err
	}
	encoded, err := json.Marshal(frozen)
	if err != nil {
		return "", err
	}
	digest := p6Digest(encoded)
	stored, err := encodeP6StoredInput(encoded)
	if err != nil {
		return "", err
	}
	var rule db.P6CombatRule
	if err = tx.WithContext(ctx).First(&rule, 1).Error; err != nil {
		return "", err
	}
	if rule.TimeoutMillis < 1000 || rule.TimeoutMillis > 86400000 || rule.MaxRounds < 1 || rule.MaxRounds > 40 {
		return "", errors.New(constUtil.CombatInvalid)
	}
	now := time.Now().UnixMilli()
	record := db.P6BattleRecord{ID: id, PlayerID: playerID, Source: source, SourceFight: fightID, Status: constUtil.P6BattleCaptured, Seed: frozen.Seed, ConfigurationHash: frozen.ConfigurationHash, EngineVersion: frozen.EngineVersion, ClientVersion: frozen.ClientVersion, Input: stored, InputHash: digest, CreatedAt: now, ExpiresAt: now + rule.TimeoutMillis}
	return id, tx.WithContext(ctx).Create(&record).Error
}

// FinishObservedBattleReplay 只记录原单人路径已接受的结果，不改变旧奖励或冒充权威计算。
// 与原结算同事务写录像，失败不留下完成状态；同源重复必须返回相同原记录。
func (s *GameService) FinishObservedBattleReplay(ctx context.Context, tx *gorm.DB, playerID int64, source, fightID string, operate *pb.BattleOperate, clientWin bool) error {
	if tx == nil || ctx == nil || operate == nil || !combatFinite(operate.GetAttackerTotalLeftHp()) || !combatFinite(operate.GetDefenderTotalLeftHp()) || operate.GetAttackerTotalLeftHp() < 0 || operate.GetDefenderTotalLeftHp() < 0 {
		return errors.New(constUtil.CombatInvalid)
	}
	var record db.P6BattleRecord
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND source=? AND source_fight=?", playerID, source, fightID).First(&record).Error; err != nil {
		return err
	}
	frozen, err := decodeP6Record(&record)
	if err != nil {
		return err
	}
	if operate.GetFinalRoundNum() < 0 || operate.GetFinalRoundNum() > frozen.MaxRound+1 || len(operate.GetSkillOpts()) > 10000 {
		return errors.New(constUtil.CombatInvalid)
	}
	// The original exhausted-round result counts the next round without executing skills in it.
	for _, operation := range operate.GetSkillOpts() {
		if operation == nil || operation.GetRoundNum() < 1 || operation.GetRoundNum() > frozen.MaxRound {
			return errors.New(constUtil.CombatInvalid)
		}
	}
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(operate)
	if err != nil {
		return err
	}
	result := int32(1)
	if clientWin {
		result = 0
	}
	hash := p6Digest(append(append([]byte(nil), encoded...), byte(result)))
	if record.Status == constUtil.P6BattleObserved {
		if record.ReplayHash != hash || record.ClientResult != result {
			return errors.New(constUtil.P6ReplayConflict)
		}
		return nil
	}
	if record.Status != constUtil.P6BattleCaptured {
		return errors.New(constUtil.P6ReplayUnavailable)
	}
	record.Status = constUtil.P6BattleObserved
	record.Operates = encoded
	record.ReplayHash = hash
	record.ClientResult = result
	record.ServerResult = result
	record.Authoritative = false
	record.FinishedAt = time.Now().UnixMilli()
	return tx.WithContext(ctx).Save(&record).Error
}

// fightPlayback 返回冻结原快照与操作，必须完成且归属一致；查询不重新计算或发资源。
func (s *GameService) fightPlayback(ctx context.Context, id int64, request *pb.CSFightPlayback) ([]proto.Message, error) {
	if request == nil || strings.TrimSpace(request.GetFightId()) == "" || len(request.GetFightId()) > 128 || request.GetServerId() != 0 && request.GetServerId() != int32(s.OriginalServerID()) {
		return nil, errors.New(constUtil.P6ReplayUnavailable)
	}
	var row db.P6BattleRecord
	// 原回放ID优先走主键，未找到才查原战斗别名，避免OR扫入全部长快照。
	err := db.GetIns().WithContext(ctx).Where("id=? AND player_id=?", request.GetFightId(), id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		err = db.GetIns().WithContext(ctx).Where("player_id=? AND source_fight=?", id, request.GetFightId()).Order("id ASC").First(&row).Error
	}
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return s.dailyPlayback(ctx, id, request.GetFightId())
		}
		return nil, errors.New(constUtil.P6ReplayUnavailable)
	}
	if row.Status != constUtil.P6BattleObserved && row.Status != constUtil.P6BattleComputed {
		return nil, errors.New(constUtil.P6ReplayUnavailable)
	}
	input, err := decodeP6Record(&row)
	// Original SetPlaybackData has no type11 defender branch. Friend duels
	// recover through their original SCFriendFight receipt; returning generic
	// SCFightPlayback here would expose an unplayable diagnostic command list.
	if err == nil && input.BattleType == 11 {
		return nil, errors.New(constUtil.P6ReplayUnavailable)
	}
	operate := &pb.BattleOperate{}
	if err != nil || proto.Unmarshal(row.Operates, operate) != nil || p6Digest(append(append([]byte(nil), row.Operates...), byte(row.ClientResult))) != row.ReplayHash {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	if row.Status == constUtil.P6BattleComputed {
		var outcome model.CombatOutcome
		if p6Digest(row.Outcome) != row.OutcomeHash || json.Unmarshal(row.Outcome, &outcome) != nil || !proto.Equal(operate, outcome.Operates) || outcome.Authoritative != row.Authoritative || row.ServerResult != row.ClientResult || row.ServerResult != p6PlaybackResult(outcome.Winner) {
			return nil, errors.New(constUtil.P6ReplayConflict)
		}
	} else if row.Authoritative || row.ServerResult != row.ClientResult {
		return nil, errors.New(constUtil.P6ReplayConflict)
	}
	result := &pb.SCFightPlayback{Fightid: proto.String(row.ID), RandSeed: proto.Int64(input.Seed), BattleType: proto.Int32(input.BattleType), Attacker: input.Attacker, Defender1: input.Defender, Defender2: input.Monster, WinConditions: input.WinConditions, OptRecord: operate, ServerResult: proto.Int32(row.ServerResult), ClientResult: proto.Int32(row.ClientResult), MaxRound: proto.Int32(input.MaxRound), AttackerFactor: proto.Float64(FrozenP6DamageFactor(input, 0)), DefenderFactor: proto.Float64(FrozenP6DamageFactor(input, 1))}
	return []proto.Message{result}, nil
}

// DispatchP6 只接原录像查询。开始/结算复用业务所属期的资格和原资源事务。
func (s *GameService) DispatchP6(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch b := request.(type) {
	case *pb.CSFightPlayback:
		r, e := s.fightPlayback(ctx, id, b)
		return r, true, e
	}
	return nil, false, nil
}

func p6ConditionsEqual(a, b []*pb.BattleWinCondition) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !proto.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// CancelBattleReplay closes an uncompleted instance in the same owning Profile transaction.
func (s *GameService) CancelBattleReplay(ctx context.Context, tx *gorm.DB, playerID int64, source, fightID string) error {
	if tx == nil || ctx == nil || playerID <= 0 {
		return errors.New(constUtil.CombatInvalid)
	}
	var record db.P6BattleRecord
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND source=? AND source_fight=?", playerID, source, fightID).First(&record).Error; err != nil {
		return err
	}
	if record.Status == constUtil.P6BattleCancelled || record.Status == constUtil.P6BattleExpired {
		return nil
	}
	if record.Status != constUtil.P6BattleCaptured {
		return errors.New(constUtil.P6ReplayConflict)
	}
	record.Status = constUtil.P6BattleCancelled
	if time.Now().UnixMilli() > record.ExpiresAt {
		record.Status = constUtil.P6BattleExpired
	}
	record.FinishedAt = time.Now().UnixMilli()
	return tx.WithContext(ctx).Save(&record).Error
}

// ComputeBattleReplay executes actual rounds against the already frozen record and stores
// the replay atomically. Its result remains non-authoritative until native comparison gates pass.
func (s *GameService) ComputeBattleReplay(ctx context.Context, tx *gorm.DB, playerID int64, source, fightID string, supplied *pb.BattleOperate) (*model.CombatOutcome, error) {
	if tx == nil || ctx == nil || playerID <= 0 {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	var record db.P6BattleRecord
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND source=? AND source_fight=?", playerID, source, fightID).First(&record).Error; err != nil {
		return nil, err
	}
	input, err := decodeP6Record(&record)
	if err != nil {
		return nil, err
	}
	if record.Status == constUtil.P6BattleComputed {
		var previous model.CombatOutcome
		if p6Digest(record.Outcome) != record.OutcomeHash || json.Unmarshal(record.Outcome, &previous) != nil {
			return nil, errors.New(constUtil.P6ReplayConflict)
		}
		encoded, marshalErr := proto.MarshalOptions{Deterministic: true}.Marshal(previous.Operates)
		if marshalErr != nil || p6Digest(append(append([]byte(nil), encoded...), byte(record.ClientResult))) != record.ReplayHash || !reflect.DeepEqual(encoded, record.Operates) || previous.Authoritative != record.Authoritative || record.ServerResult != record.ClientResult || record.ServerResult != p6PlaybackResult(previous.Winner) {
			return nil, errors.New(constUtil.P6ReplayConflict)
		}
		// Original protobuf-net omits zero fields on retries; compare semantic operations.
		if supplied != nil && !p6OriginalOperateEqual(previous.Operates, supplied) {
			return nil, errors.New(constUtil.P6ReplayConflict)
		}
		return &previous, nil
	}
	if record.Status != constUtil.P6BattleCaptured || time.Now().UnixMilli() > record.ExpiresAt {
		return nil, errors.New(constUtil.P6ReplayUnavailable)
	}
	outcome, err := s.ComputeFrozenBattle(ctx, input, supplied)
	if err != nil {
		return nil, err
	}
	outcome.CapabilityHash = P6CapabilityHash()
	outcome.Authoritative = false
	encoded, err := proto.MarshalOptions{Deterministic: true}.Marshal(outcome.Operates)
	if err != nil {
		return nil, err
	}
	result, err := json.Marshal(outcome)
	if err != nil {
		return nil, err
	}
	record.Status = constUtil.P6BattleComputed
	record.Operates = encoded
	record.Outcome = result
	record.OutcomeHash = p6Digest(result)
	record.ClientResult = 1
	record.ServerResult = 1
	if outcome.Winner == 0 {
		record.ClientResult = 0
		record.ServerResult = 0
	}
	record.ReplayHash = p6Digest(append(append([]byte(nil), encoded...), byte(record.ClientResult)))
	record.Authoritative = false
	record.FinishedAt = time.Now().UnixMilli()
	return outcome, tx.WithContext(ctx).Save(&record).Error
}

func p6PlaybackResult(winner int32) int32 {
	if winner == 0 {
		return 0
	}
	return 1
}

// RecoverFrozenBattle resumes a captured instance after reconnect without new seed or tables.
// Completed/cancelled instances are handled by their owning business receipt.
func (s *GameService) RecoverFrozenBattle(ctx context.Context, tx *gorm.DB, playerID int64, source, fightID string) (*model.FrozenBattleInput, error) {
	if ctx == nil || tx == nil || playerID <= 0 {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	var record db.P6BattleRecord
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("player_id=? AND source=? AND source_fight=?", playerID, source, fightID).First(&record).Error; err != nil {
		return nil, err
	}
	if record.Status != constUtil.P6BattleCaptured || time.Now().UnixMilli() > record.ExpiresAt {
		return nil, errors.New(constUtil.P6ReplayUnavailable)
	}
	return decodeP6Record(&record)
}

// OriginalCopyReplay only captures original single-group playback, whose original response
// has exactly one defender2. Multi-group playback requires separate native protocol audit.
func (s *GameService) CaptureOriginalCopyReplay(ctx context.Context, tx *gorm.DB, id int64, start *pb.SCCopyFightStart) error {
	if !tx.Migrator().HasTable(&db.P6BattleRecord{}) || start == nil || len(start.GetBattleMonsterList()) != 1 {
		return nil
	}
	_, err := s.CaptureBattleReplay(ctx, tx, id, "copy", start.GetFightId(), model.FrozenBattleInput{Attacker: start.GetBattlePlayer(), Monster: start.GetBattleMonsterList()[0], Seed: start.GetRandSeed(), MaxRound: start.GetMaxRound(), BattleType: 1})
	return err
}
func (s *GameService) FinishOriginalCopyReplay(ctx context.Context, tx *gorm.DB, id int64, start *pb.SCCopyFightStart, end *pb.CSCopyFightEnd) error {
	if !tx.Migrator().HasTable(&db.P6BattleRecord{}) || start == nil || len(start.GetBattleMonsterList()) != 1 || end.GetOptRecord() == nil {
		return nil
	}
	// A pre-P6 saved fight may survive the server restart; keep its original P0 settlement.
	var count int64
	if err := tx.WithContext(ctx).Model(&db.P6BattleRecord{}).Where("player_id=? AND source=? AND source_fight=?", id, "copy", start.GetFightId()).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	return s.FinishObservedBattleReplay(ctx, tx, id, "copy", start.GetFightId(), end.GetOptRecord(), end.GetResult() == 1)
}
func (s *GameService) CancelOriginalCopyReplay(ctx context.Context, tx *gorm.DB, id int64, start *pb.SCCopyFightStart) error {
	if !tx.Migrator().HasTable(&db.P6BattleRecord{}) || start == nil || len(start.GetBattleMonsterList()) != 1 {
		return nil
	}
	var count int64
	if err := tx.WithContext(ctx).Model(&db.P6BattleRecord{}).Where("player_id=? AND source=? AND source_fight=?", id, "copy", start.GetFightId()).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	return s.CancelBattleReplay(ctx, tx, id, "copy", start.GetFightId())
}
