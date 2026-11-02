package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func copySweepNumber(row map[string]string, key string) (int64, error) {
	value, err := strconv.ParseInt(row[key], 10, 32)
	if err != nil || value < 0 {
		return 0, errors.New(constUtil.CopySweepConfigError)
	}
	return value, nil
}

func (s *GameService) copySweep(ctx context.Context, id int64, request *pb.CSCopySweep, stamp int64) ([]proto.Message, error) {
	return s.copySweepAt(ctx, id, request, stamp, time.Now())
}

// InitializeCopySweepWitness runs before serving traffic and fails closed if
// the table cannot be created. One row stores each role's latest sweep response.
func (s *GameService) InitializeCopySweepWitness(ctx context.Context) error {
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.CopySweepWitness{}); err != nil {
		return err
	}
	s.CopySweepWitnessReady = true
	return nil
}

func copySweepReceiptBytes(receipt *model.CopySweepReceipt) ([]byte, []byte, []byte, error) {
	if receipt == nil || receipt.Timestamp <= 0 || receipt.Request == nil || receipt.Request.GetChapterId() == "" || receipt.Request.GetMissionId() == "" || receipt.Response == nil {
		return nil, nil, nil, errors.New(constUtil.CopySweepHistoryUnavailable)
	}
	marshal := proto.MarshalOptions{Deterministic: true}
	request, err := marshal.Marshal(receipt.Request)
	if err != nil {
		return nil, nil, nil, err
	}
	response, err := marshal.Marshal(receipt.Response)
	if err != nil {
		return nil, nil, nil, err
	}
	var header [16]byte
	binary.BigEndian.PutUint64(header[:8], uint64(receipt.Timestamp))
	binary.BigEndian.PutUint64(header[8:], uint64(len(request)))
	hash := sha256.New()
	hash.Write(header[:])
	hash.Write(request)
	hash.Write(response)
	return request, response, hash.Sum(nil), nil
}

// The legacy profile stores protobuf values through encoding/json, which does
// not persist protobuf unknown fields. Match that durable shape for its digest;
// the witness retains the complete original protobuf separately.
func copySweepProfileDigest(receipt *model.CopySweepReceipt) ([]byte, error) {
	raw, err := json.Marshal(receipt)
	if err != nil {
		return nil, err
	}
	var stored model.CopySweepReceipt
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, err
	}
	_, _, digest, err := copySweepReceiptBytes(&stored)
	return digest, err
}

func copySweepSaveWitness(tx *gorm.DB, witness *db.CopySweepWitness, maximum, last *model.CopySweepReceipt) error {
	maxDigest, err := copySweepProfileDigest(maximum)
	if err != nil {
		return err
	}
	request, response, lastDigest, err := copySweepReceiptBytes(last)
	if err != nil || last.Timestamp > maximum.Timestamp {
		return errors.New(constUtil.CopySweepHistoryUnavailable)
	}
	witness.MaxStamp, witness.MaxDigest = maximum.Timestamp, maxDigest
	witness.LastStamp, witness.LastDigest = last.Timestamp, lastDigest
	witness.LastRequest, witness.LastResponse = request, response
	return tx.Save(witness).Error
}

// copySweepLoadWitness follows the profile lock. An old binary can only move
// the legacy watermark forward; an equal watermark must match its saved digest.
func copySweepLoadWitness(tx *gorm.DB, id int64, maximum *model.CopySweepReceipt, completeHistory, initialized bool) (*db.CopySweepWitness, *model.CopySweepReceipt, bool, error) {
	var witness db.CopySweepWitness
	err := tx.First(&witness, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if initialized {
			return nil, nil, false, errors.New(constUtil.CopySweepHistoryUnavailable)
		}
		return &db.CopySweepWitness{PlayerID: id}, maximum, maximum != nil, nil
	}
	if err != nil || maximum == nil {
		return nil, nil, false, errors.New(constUtil.CopySweepHistoryUnavailable)
	}
	digest, err := copySweepProfileDigest(maximum)
	if err != nil || witness.MaxStamp <= 0 || len(witness.MaxDigest) != sha256.Size || maximum.Timestamp < witness.MaxStamp {
		return nil, nil, false, errors.New(constUtil.CopySweepHistoryUnavailable)
	}
	if witness.LastStamp <= 0 || witness.LastStamp > witness.MaxStamp || len(witness.LastDigest) != sha256.Size {
		return nil, nil, false, errors.New(constUtil.CopySweepHistoryUnavailable)
	}
	last := &model.CopySweepReceipt{Timestamp: witness.LastStamp, Request: &pb.CSCopySweep{}, Response: &pb.SCCopySweep{}}
	if proto.Unmarshal(witness.LastRequest, last.Request) != nil || proto.Unmarshal(witness.LastResponse, last.Response) != nil {
		return nil, nil, false, errors.New(constUtil.CopySweepHistoryUnavailable)
	}
	_, _, lastDigest, err := copySweepReceiptBytes(last)
	if err != nil || !bytes.Equal(lastDigest, witness.LastDigest) {
		return nil, nil, false, errors.New(constUtil.CopySweepHistoryUnavailable)
	}
	if last.Timestamp == witness.MaxStamp {
		lastProfileDigest, err := copySweepProfileDigest(last)
		if err != nil || !bytes.Equal(lastProfileDigest, witness.MaxDigest) {
			return nil, nil, false, errors.New(constUtil.CopySweepHistoryUnavailable)
		}
	}
	if maximum.Timestamp > witness.MaxStamp {
		if completeHistory {
			if err := copySweepCheckKnownGrants(tx, id, last); err != nil {
				return nil, nil, false, err
			}
			found, err := copySweepHasGrant(tx, id, witness.MaxStamp)
			if err != nil || !found {
				return nil, nil, false, errors.New(constUtil.CopySweepHistoryUnavailable)
			}
		}
		return &witness, maximum, true, nil
	}
	if !bytes.Equal(digest, witness.MaxDigest) {
		return nil, nil, false, errors.New(constUtil.CopySweepHistoryUnavailable)
	}
	return &witness, last, false, nil
}

// copySweepHasGrant looks up every mission at this timestamp using the P9
// primary-key prefix. The caller holds the profile lock until settlement commits.
func copySweepHasGrant(tx *gorm.DB, id, stamp int64) (bool, error) {
	var grant db.P9PetCopyGrant
	prefix := "sweep:" + strconv.FormatInt(stamp, 10) + ":%"
	err := tx.Select("source_key").Where("player_id=? AND source_key LIKE ?", id, prefix).Take(&grant).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// copySweepCheckKnownGrants checks both retained receipts against their exact
// P9 source. An absent or malformed known row contradicts complete history.
func copySweepCheckKnownGrants(tx *gorm.DB, id int64, receipts ...*model.CopySweepReceipt) error {
	keys := make(map[string]bool, len(receipts))
	for _, receipt := range receipts {
		if receipt == nil {
			continue
		}
		key := "sweep:" + strconv.FormatInt(receipt.Timestamp, 10) + ":" + receipt.Request.GetChapterId() + "/" + receipt.Request.GetMissionId()
		keys[key] = true
	}
	if len(keys) == 0 {
		return nil
	}
	wanted := make([]string, 0, len(keys))
	for key := range keys {
		wanted = append(wanted, key)
	}
	var grants []db.P9PetCopyGrant
	if err := tx.Select("source_key", "completed_count", "source").Where("player_id=? AND source_key IN ?", id, wanted).Find(&grants).Error; err != nil {
		return errors.New(constUtil.CopySweepHistoryUnavailable)
	}
	for _, grant := range grants {
		if !keys[grant.SourceKey] || grant.CompletedCount != 1 || grant.Source == "" {
			return errors.New(constUtil.CopySweepHistoryUnavailable)
		}
		delete(keys, grant.SourceKey)
	}
	if len(keys) != 0 {
		return errors.New(constUtil.CopySweepHistoryUnavailable)
	}
	return nil
}

// copySweepCommittedSince detects the transaction-local settlement marker.
// The timestamp fallback preserves old callers that still write legacy receipts.
func copySweepCommittedSince(before, after *model.GameState) (bool, error) {
	if before == nil || after == nil {
		return false, errors.New(constUtil.CopySweepHistoryUnavailable)
	}
	if after.CopySweepCommitted {
		return true, nil
	}
	current, prior := after.CopySweepReceipt, before.CopySweepReceipt
	return current != nil && current.Response != nil && (prior == nil || current.Timestamp > prior.Timestamp), nil
}

// addCopySweepExperience 校验原等级表并逐级检查资源上限，任一步失败均回滚整包扫荡。
func (s *GameService) addCopySweepExperience(player *pb.Player, exp int64) error {
	player.Exp = proto.Int64(player.GetExp() + exp)
	for {
		var (
			level = s.table("UserExp", "level", strconv.Itoa(int(player.GetLevel())))
			next  = s.table("UserExp", "level", strconv.Itoa(int(player.GetLevel())+1))
		)
		if level == nil {
			return errors.New(constUtil.CopySweepConfigError)
		}
		// 原表最后一级以 -1 结束经验升级，不给空白体力和精力列补造奖励。
		if next == nil {
			if level["exp"] != "-1" {
				return errors.New(constUtil.CopySweepConfigError)
			}
			return nil
		}
		threshold, err := copySweepNumber(level, "exp")
		if err != nil || threshold < 1 {
			return errors.New(constUtil.CopySweepConfigError)
		}
		if player.GetExp() < threshold {
			return nil
		}
		var stamina, energy int64
		for key, target := range map[string]*int64{"stamina": &stamina, "energy": &energy} {
			if next["exp"] == "-1" && next[key] == "" {
				continue
			}
			*target, err = copySweepNumber(next, key)
			if err != nil {
				return err
			}
		}
		if player.GetStamina() > math.MaxInt32-int32(stamina) || player.GetEnergy() > math.MaxInt32-int32(energy) {
			return errors.New(constUtil.CopySweepUpgradeError)
		}
		player.Exp = proto.Int64(player.GetExp() - threshold)
		player.Level = proto.Int32(player.GetLevel() + 1)
		player.Stamina = proto.Int32(player.GetStamina() + int32(stamina))
		player.Energy = proto.Int32(player.GetEnergy() + int32(energy))
	}
}

// copySweepAt 恢复原 APK 每包一次扫荡；客户端多次扫荡通过逐包请求完成。
func (s *GameService) copySweepAt(ctx context.Context, id int64, request *pb.CSCopySweep, stamp int64, now time.Time) ([]proto.Message, error) {
	var (
		response *pb.SCCopySweep
		updated  *pb.Player
		progress *pb.SCCopyPassedInfo
	)
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, player *pb.Player, game *model.GameState) error {
		if value := tx.Statement.Context.Value(ggoNativeSessionKey{}); value != nil {
			session, ok := value.([]byte)
			if !ok {
				return errors.New(constUtil.CopySweepSessionError)
			}
			var current db.Profile
			if err := tx.Select("session").First(&current, id).Error; err != nil {
				return err
			}
			if err := validateProfileSession(&current, session); err != nil {
				return err
			}
		}
		if stamp <= 0 || request == nil {
			return errors.New(constUtil.CopySweepReplayError)
		}
		maximum := game.CopySweepReceipt
		if maximum != nil {
			if _, _, _, err := copySweepReceiptBytes(maximum); err != nil {
				return errors.New(constUtil.CopySweepHistoryUnavailable)
			}
		}
		if s.CopySweepHistoryComplete && (!s.P9PetsReady || !s.CopySweepWitnessReady) {
			return errors.New(constUtil.CopySweepHistoryUnavailable)
		}
		last := maximum
		var (
			witness *db.CopySweepWitness
			dirty   bool
			err     error
		)
		if s.CopySweepWitnessReady {
			witness, last, dirty, err = copySweepLoadWitness(tx, id, maximum, s.CopySweepHistoryComplete, game.CopySweepWitnessInitialized)
			if err != nil {
				return err
			}
		}
		if s.CopySweepHistoryComplete {
			if err := copySweepCheckKnownGrants(tx, id, maximum, last); err != nil {
				return err
			}
		}
		hasGrant := s.CopySweepHistoryComplete && last != nil && stamp == last.Timestamp
		if s.P9PetsReady && !hasGrant {
			hasGrant, err = copySweepHasGrant(tx, id, stamp)
			if err != nil {
				return errors.New(constUtil.CopySweepHistoryUnavailable)
			}
		}
		// 损坏进度须在日切和刷新前拒绝，避免空记录引发异常或重复关卡累计星数。
		seen := map[string]bool{}
		for _, info := range game.Copy.GetPassedCopyInfo() {
			if info == nil || info.GetStar() < 0 || info.GetStar() > constUtil.CopySweepMinimumStars || info.GetChallengeTimes() < 0 {
				return errors.New(constUtil.CopySweepConfigError)
			}
			key := info.GetChapterId() + ":" + info.GetMissionId()
			if seen[key] {
				return errors.New(constUtil.CopySweepConfigError)
			}
			seen[key] = true
		}
		// 最近回执跨日保持有效；相同包不会在新日重放奖励或增加挑战次数。
		if last != nil && stamp == last.Timestamp {
			if !proto.Equal(last.Request, request) {
				return errors.New(constUtil.CopySweepReplayError)
			}
			if s.CopySweepHistoryComplete && !hasGrant {
				return errors.New(constUtil.CopySweepHistoryUnavailable)
			}
			if err := s.ValidateCopyAwardReceiptCurrent(tx, id, game, last.Response); err != nil {
				return err
			}
			if dirty {
				if err := copySweepSaveWitness(tx, witness, maximum, last); err != nil {
					return err
				}
			}
			if s.CopySweepWitnessReady {
				game.CopySweepWitnessInitialized = true
			}
			response = proto.Clone(last.Response).(*pb.SCCopySweep)
			if err := s.refreshCopyCalendar(ctx, tx, id, game, now); err != nil {
				return err
			}
			s.refreshCopyProgress(game)
			updated = player
			progress = game.Copy
			return nil
		}
		// A P9 grant proves a prior committed sweep even when its complete
		// response is no longer retained. Its timestamp is shared across missions.
		if hasGrant || (!s.CopySweepHistoryComplete && maximum != nil && stamp <= maximum.Timestamp) {
			return errors.New(constUtil.CopySweepReplayError)
		}
		mission := s.table("CopyMission", "MissionId", request.GetMissionId())
		if err := s.ensureNoActiveDailyCopy(tx, id, now); err != nil {
			return err
		}
		if mission == nil || mission["ChapterId"] != request.GetChapterId() || s.table("CopyChapter", "ChapterId", request.GetChapterId()) == nil {
			return errors.New(constUtil.CopySweepChapterError)
		}
		minLevel, err := copySweepNumber(s.table("GameConfigure", "StaticName", "FightTenLevel"), "NameValue")
		if err != nil || minLevel < 1 {
			return errors.New(constUtil.CopySweepConfigError)
		}
		minVIP, err := copySweepNumber(s.table("GameConfigure", "StaticName", "FightTenVIPLevel"), "NameValue")
		if err != nil || minVIP < 1 {
			return errors.New(constUtil.CopySweepConfigError)
		}
		if int64(player.GetLevel()) < minLevel && int64(player.GetVip()) < minVIP {
			return errors.New(constUtil.CopySweepLevelError)
		}
		if game.Run != nil && !game.Run.Complete {
			return errors.New(constUtil.CopySweepActiveRunError)
		}
		if game.Copy == nil {
			return errors.New(constUtil.CopySweepUnavailable)
		}
		var passed *pb.CopySimpleInfo
		for _, info := range game.Copy.GetPassedCopyInfo() {
			if info.GetChapterId() == request.GetChapterId() && info.GetMissionId() == request.GetMissionId() {
				passed = info
			}
		}
		if passed == nil || passed.GetStar() < constUtil.CopySweepMinimumStars {
			return errors.New(constUtil.CopySweepUnavailable)
		}
		chapter := s.table("CopyChapter", "ChapterId", request.GetChapterId())
		chapterLevel, err := copySweepNumber(chapter, "LevelLimit")
		if err != nil {
			return err
		}
		if int64(player.GetLevel()) < chapterLevel {
			return errors.New(constUtil.CopySweepChapterLevelError)
		}
		limit, err := copySweepNumber(mission, "ChallengeCount")
		if err != nil || limit < 1 {
			return errors.New(constUtil.CopySweepConfigError)
		}
		cost, err := copySweepNumber(mission, "PowerCost")
		if err != nil || cost < 1 {
			return errors.New(constUtil.CopySweepConfigError)
		}
		if err := s.refreshCopyCalendar(ctx, tx, id, game, now); err != nil {
			return err
		}
		if int64(passed.GetChallengeTimes()) >= limit {
			return errors.New(constUtil.CopySweepLimitError)
		}
		if int64(player.GetStamina()) < cost {
			return errors.New(constUtil.CopySweepStaminaError)
		}
		reward := s.table("CopyExp", "level", strconv.Itoa(int(player.GetLevel())))
		coin, err := copySweepNumber(reward, "coin")
		if err != nil {
			return err
		}
		exp, err := copySweepNumber(reward, "exp")
		if err != nil {
			return err
		}
		// 料理奖励与时效扣减沿用扫荡行锁事务，同包回执分支已提前返回。
		if s.P4Ready {
			coin, exp, err = s.ApplyP4CopyCurrencyBonus(tx, LocalPlayAt(ctx, now), coin, exp)
			if err != nil {
				return err
			}
		}
		coin, exp, err = s.ApplyCookingRewardBonus(tx, id, now.UnixMilli(), coin, exp, 1)
		if err != nil {
			return err
		}
		if player.GetCoin() < 0 || player.GetExp() < 0 || player.GetEnergy() < 0 || player.GetCoin() > math.MaxInt64-coin || player.GetExp() > math.MaxInt64-exp {
			return errors.New(constUtil.CopySweepBalanceError)
		}
		player.Stamina = proto.Int32(player.GetStamina() - int32(cost))
		player.Coin = proto.Int64(player.GetCoin() + coin)
		if err := s.addCopySweepExperience(player, exp); err != nil {
			return err
		}
		passed.ChallengeTimes = proto.Int32(passed.GetChallengeTimes() + 1)
		s.refreshCopyProgress(game)
		// 扫荡逐原任务计一次实际成功，普通随机与具名本地特殊四胜累计同事务；首通不重复。
		response = &pb.SCCopySweep{Cost: []*pb.BaseResource{{Id: proto.String("stamina"), Num: proto.Int32(int32(cost))}}, Gain: &pb.ResultObject{Simps: []*pb.SimpleItem{{Id: proto.String("coin"), Num: proto.Int32(int32(coin))}, {Id: proto.String("exp"), Num: proto.Int32(int32(exp))}}}, WantedId: proto.String(""), WantedLevel: proto.Int32(0)}
		loot := &pb.ResultObject{}
		for _, task := range s.tasks(request.GetMissionId()) {
			gain, _, err := s.rollCopyDrops(tx, player, game, task["DropOdds"])
			if err != nil {
				return err
			}
			p1AppendGain(response.Gain, gain)
			p1AppendGain(loot, gain)
			special, e := s.rollCopySpecialDrops(tx, id, player, game, task)
			if e != nil {
				return e
			}
			p1AppendGain(response.Gain, special)
			p1AppendGain(loot, special)
		}
		// 金币与玩家经验已在上段加倍；这里只复制本次实际随机掉落，额外实例仍由公共奖励创建独立GUID。
		if s.P4Ready {
			source := constUtil.P4CopySweepDropSourcePrefix + strconv.FormatInt(stamp, 10) + ":" + request.GetChapterId() + "/" + request.GetMissionId()
			bonus, _, err := s.ApplyP4CopyDropBonus(tx, id, player, game, source, loot, LocalPlayAt(ctx, now))
			if err != nil {
				return err
			}
			p1AppendGain(response.Gain, bonus)
		}

		if s.P9PetsReady {
			adds, err := s.ApplyP9CopyIntimacy(tx, id, player, game, "sweep:"+strconv.FormatInt(stamp, 10)+":"+request.GetChapterId()+"/"+request.GetMissionId())
			if err != nil {
				return err
			}
			response.PetAdds = adds
		}
		if s.P12WantedReady {
			// 一包真实扫荡只抽一次；重复时间戳在前段返回已冻结完整回执。
			source := "copy-sweep:" + strconv.FormatInt(stamp, 10) + ":" + request.GetChapterId() + "/" + request.GetMissionId()
			found, gain, err := s.DiscoverP12Wanted(tx, id, player, game, source, LocalPlayAt(ctx, now))
			if err != nil {
				return err
			}
			p1AppendGain(response.Gain, gain)
			if found != nil {
				response.WantedId = proto.String(found.GetWantedId())
				response.WantedLevel = proto.Int32(found.GetLevel())
			}
		}
		if err := s.triggerLuxuryDragonLocked(tx, id, game, LocalPlayAt(ctx, now)); err != nil {
			return err
		}
		receipt := &model.CopySweepReceipt{Timestamp: stamp, Request: proto.Clone(request).(*pb.CSCopySweep), Response: proto.Clone(response).(*pb.SCCopySweep)}
		// Keep the legacy field at the maximum so an old binary still rejects
		// every earlier timestamp after rollback or a profile rewrite.
		if maximum == nil || stamp > maximum.Timestamp {
			game.CopySweepReceipt = receipt
			maximum = receipt
		}
		if s.CopySweepWitnessReady {
			if err := copySweepSaveWitness(tx, witness, maximum, receipt); err != nil {
				return err
			}
			game.CopySweepWitnessInitialized = true
		}
		game.CopySweepCommitted = true
		updated = player
		progress = game.Copy
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response, &pb.SCPlayer{Player: updated}, progress}, nil
}

// refreshCopySweepDay 使用可注入时钟测试 UTC+8 边界，旧存档未记日期时保留当前次数。
func refreshCopySweepDay(game *model.GameState, now time.Time) {
	day := now.In(time.FixedZone("UTC+8", 8*3600)).Format("2006-01-02")
	if game.CopyChallengeDay != "" && game.CopyChallengeDay != day {
		for _, info := range game.Copy.GetPassedCopyInfo() {
			info.ChallengeTimes = proto.Int32(0)
		}
	}
	game.CopyChallengeDay = day
}
