package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"sort"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitializeP11Arena 真实入列起点是透明有限策略；原机器人数量不生成虚构玩家。
func (s *GameService) InitializeP11Arena(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P11ArenaPolicy{}, &db.P11ArenaMember{}, &db.P11ArenaMatch{}, &db.P11ArenaReceipt{}, &db.P11FriendDuel{}, &db.P11FriendFightSettlement{}); err != nil {
		return err
	}
	low, err := s.p10Setting("PVPPushLowNum")
	if err != nil {
		return err
	}
	high, err := s.p10Setting("PVPPushHighNum")
	if err != nil {
		return err
	}
	cfg := db.P11ArenaPolicy{ID: constUtil.P11ArenaPolicyID, NextRank: 1, MatchCount: low + high, RankShow: 50, MatchMillis: 300000, Source: constUtil.P11ArenaSource}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error; err != nil {
		return err
	}
	if err := s.InitializeP11ArenaFight(ctx); err != nil {
		return err
	}
	if err := s.InitializeP11Limit(ctx); err != nil {
		return err
	}
	if err := s.InitializeP11Plunder(ctx); err != nil {
		return err
	}
	return s.InitializeP11Grab(ctx)
}
func p11ArenaConfig(tx *gorm.DB) (*db.P11ArenaPolicy, error) {
	var cfg db.P11ArenaPolicy
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, constUtil.P11ArenaPolicyID).Error; err != nil {
		return nil, err
	}
	if cfg.Source == "" || cfg.NextRank < 1 || cfg.MatchCount < 1 || cfg.MatchCount > 20 || cfg.RankShow < 1 || cfg.RankShow > 500 || cfg.MatchMillis < 1000 || cfg.MatchMillis > 3600000 {
		return nil, errors.New("竞技真实名册运营值无效")
	}
	return &cfg, nil
}
func (s *GameService) p11ArenaProjection(row *db.Profile, projectionTx ...*gorm.DB) (*pb.SimplePlayerInfo, *pb.BattlePlayer, error) {
	p, g, err := decodeProfile(row)
	if err != nil {
		return nil, nil, err
	}
	if p.GetPlayerId() != row.Id {
		return nil, nil, errors.New("竞技角色归属存档无效")
	}
	if len(projectionTx) == 1 {
		tx := projectionTx[0]
		if err := s.PrepareP5GuildPrayerProperties(tx.Statement.Context, tx, p, g, p13UFNow(tx.Statement.Context)); err != nil {
			return nil, nil, err
		}
	}
	battle, err := s.battlePlayer(p, g)
	if err != nil {
		return nil, nil, err
	}
	if len(battle.GetFormation()) == 0 {
		return nil, nil, errors.New("竞技需实际队伍")
	}
	power, err := s.formationFightPower(p, g)
	if err != nil {
		return nil, nil, err
	}
	hero, err := P5WorldHeroID(p, g)
	if err != nil {
		return nil, nil, err
	}
	return &pb.SimplePlayerInfo{PlayerId: proto.Int64(row.Id), ServerId: proto.Int32(1), PlayerName: p.Name, Level: p.Level, HeroWarId: proto.String(hero), FightPower: proto.Int32(power)}, battle, nil
}

// p11SyncArenaMembers 从实际Profile同一行快照生成名册，失败候选停用且不影响其他真实角色。
func (s *GameService) p11SyncArenaMembers(tx *gorm.DB, cfg *db.P11ArenaPolicy, now time.Time) error {
	if _, err := s.PrepareP11ArenaDailyMutation(tx.Statement.Context, tx, now); err != nil {
		return err
	}
	minLevel, err := s.p10Setting("PVPLevel")
	if err != nil {
		return err
	}
	var profiles []db.Profile
	if err = tx.Where("player IS NOT NULL AND game IS NOT NULL").Order("id").Find(&profiles).Error; err != nil {
		return err
	}
	valid := map[int64]bool{}
	for _, profile := range profiles {
		info, battle, err := s.p11ArenaProjection(&profile, tx)
		if err != nil || info.GetLevel() < minLevel {
			continue
		}
		valid[profile.Id] = true
		rawInfo, err := proto.Marshal(info)
		if err != nil {
			return err
		}
		rawBattle, err := proto.Marshal(battle)
		if err != nil {
			return err
		}
		hash := p6Digest(append(append([]byte(nil), rawInfo...), rawBattle...))
		var member db.P11ArenaMember
		err = tx.First(&member, "player_id=?", profile.Id).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if cfg.NextRank == math.MaxInt64 {
				return errors.New("竞技名次编号溢出")
			}
			member = db.P11ArenaMember{PlayerID: profile.Id, Rank: cfg.NextRank, CreatedAt: now.UnixMilli()}
			cfg.NextRank++
		}
		member.Active = true
		member.Info = rawInfo
		member.Battle = rawBattle
		member.SnapshotHash = hash
		member.UpdatedAt = now.UnixMilli()
		if err = tx.Save(&member).Error; err != nil {
			return err
		}
	}
	var members []db.P11ArenaMember
	if err = tx.Find(&members).Error; err != nil {
		return err
	}
	for _, member := range members {
		if member.Active && !valid[member.PlayerID] {
			if err = tx.Model(&member).Update("active", false).Error; err != nil {
				return err
			}
		}
	}
	return tx.Save(cfg).Error
}
func p11ArenaMemberInfo(member db.P11ArenaMember) (*pb.PvpRankPlayerInfo, error) {
	info := &pb.SimplePlayerInfo{}
	battle := &pb.BattlePlayer{}
	if member.Rank < 1 || member.PlayerID < 1 || proto.Unmarshal(member.Info, info) != nil || proto.Unmarshal(member.Battle, battle) != nil || info.GetPlayerId() != member.PlayerID || battle.GetPlayerId() != member.PlayerID || p6Digest(append(append([]byte(nil), member.Info...), member.Battle...)) != member.SnapshotHash {
		return nil, errors.New("竞技真实名册快照损坏")
	}
	return &pb.PvpRankPlayerInfo{Info: info, PvpRank: proto.Int64(member.Rank)}, nil
}
func p11ArenaSnapshotHash(row db.P11ArenaMatch) string {
	meta := fmt.Sprintf("%s:%d:%d:%d:%d:%d:%s:%s:%d:%d:", row.ID, row.ViewerID, row.OpponentID, row.ViewerRank, row.OpponentRank, row.Seed, row.ConfigurationHash, row.CapabilityHash, row.CreatedAt, row.ExpiresAt)
	return p6Digest(append(append([]byte(meta), row.Attacker...), row.Defender...))
}
func (s *GameService) p11ArenaConfigurationHash() (string, error) {
	tables := map[string][]map[string]string{}
	for _, name := range []string{"Hero", "Skill", "SkillCast", "Buff", "Talent", "GameConfigure", "CopyEnemy", "BuffInfExtend", "HeroAwakenAdd"} {
		tables[name] = s.Tables[name]
	}
	raw, err := json.Marshal(tables)
	if err != nil {
		return "", err
	}
	return p6Digest(raw), nil
}

// p11ArenaQuery 原名册/对手查询在运营关闭时可读取；同stamp保持原匹配批次而不重新抽对手。
func (s *GameService) p11ArenaQuery(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	match := false
	switch request.(type) {
	case *pb.CSPvpFindMatch:
		match = true
	case *pb.CSPvpRank:
	default:
		return nil, errors.New("竞技引导或其他查询本批尚未恢复")
	}
	var reply proto.Message
	if match {
		reply = &pb.SCPvpFindMatch{}
	} else {
		reply = &pb.SCPvpRank{}
	}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		cfg, err := p11ArenaConfig(tx)
		if err != nil {
			return err
		}
		rawRequest, err := proto.Marshal(request)
		if err != nil {
			return err
		}
		if match && stamp > 0 {
			var old db.P11ArenaReceipt
			err = tx.First(&old, "player_id=? AND operation=? AND stamp=?", id, "match-query", stamp).Error
			if err == nil {
				if old.RequestHash != p6Digest(rawRequest) {
					return errors.New("竞技匹配回执请求冲突")
				}
				return proto.Unmarshal(old.Response, reply)
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		if err = s.p11SyncArenaMembers(tx, cfg, now); err != nil {
			return err
		}
		var own db.Profile
		if err = tx.First(&own, id).Error; err != nil {
			return err
		}
		if err = p11ArenaGuideQueryProfile(ctx, &own); err != nil {
			return err
		}
		info, _, err := s.p11ArenaProjection(&own, tx)
		if err != nil {
			return err
		}
		mine := &pb.PvpRankPlayerInfo{Info: info, PvpRank: proto.Int64(0)}
		var member db.P11ArenaMember
		enrollmentErr := tx.First(&member, "player_id=? AND active=?", id, true).Error
		if enrollmentErr != nil && !errors.Is(enrollmentErr, gorm.ErrRecordNotFound) {
			return enrollmentErr
		}
		enrolled := enrollmentErr == nil
		if enrolled {
			mine, err = p11ArenaMemberInfo(member)
			if err != nil {
				return err
			}
		}
		var candidates []db.P11ArenaMember
		if err = tx.Where("active=?", true).Order("position").Find(&candidates).Error; err != nil {
			return err
		}
		var selected []db.P11ArenaMember
		if match {
			if enrolled {
				for _, candidate := range candidates {
					if candidate.PlayerID == id {
						continue
					}
					blocked, err := p10Blocked(tx, id, candidate.PlayerID)
					if err != nil {
						return err
					}
					if !blocked {
						selected = append(selected, candidate)
					}
				}
				sort.SliceStable(selected, func(i, j int) bool {
					a := selected[i].Rank - member.Rank
					if a < 0 {
						a = -a
					}
					b := selected[j].Rank - member.Rank
					if b < 0 {
						b = -b
					}
					if a == b {
						return selected[i].Rank < selected[j].Rank
					}
					return a < b
				})
				if len(selected) > int(cfg.MatchCount) {
					selected = selected[:int(cfg.MatchCount)]
				}
			}
		} else {
			selected = candidates
			if len(selected) > int(cfg.RankShow) {
				selected = selected[:int(cfg.RankShow)]
			}
		}
		list := []*pb.PvpRankPlayerInfo{}
		batch := uuid.NewString()
		configHash, err := s.p11ArenaConfigurationHash()
		if err != nil {
			return err
		}
		for _, candidate := range selected {
			item, err := p11ArenaMemberInfo(candidate)
			if err != nil {
				return err
			}
			list = append(list, item)
			if match {
				seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
				if err != nil {
					return err
				}
				frozen := db.P11ArenaMatch{ID: uuid.NewString(), ViewerID: id, OpponentID: candidate.PlayerID, BatchID: batch, ViewerRank: member.Rank, OpponentRank: candidate.Rank, Attacker: append([]byte(nil), member.Battle...), Defender: append([]byte(nil), candidate.Battle...), ConfigurationHash: configHash, CapabilityHash: P6CapabilityHash(), Seed: seed.Int64(), CreatedAt: now.UnixMilli(), ExpiresAt: now.UnixMilli() + cfg.MatchMillis}
				// 新匹配写入原播放器明确先手；历史匹配读取保留冻结字节。
				attacker, defender := &pb.BattlePlayer{}, &pb.BattlePlayer{}
				if proto.Unmarshal(frozen.Attacker, attacker) != nil || proto.Unmarshal(frozen.Defender, defender) != nil {
					return errors.New("竞技双方快照损坏")
				}
				defenderFirst := attacker.GetInst().GetFirstOrder() == 0 && defender.GetInst().GetFirstOrder() == 1
				if err = SetP6FirstOrder(attacker, defender, defenderFirst); err != nil {
					return err
				}
				if frozen.Attacker, err = proto.Marshal(attacker); err != nil {
					return err
				}
				if frozen.Defender, err = proto.Marshal(defender); err != nil {
					return err
				}
				frozen.SnapshotHash = p11ArenaSnapshotHash(frozen)
				if err = tx.Create(&frozen).Error; err != nil {
					return err
				}
			}
		}
		if match {
			reply = &pb.SCPvpFindMatch{Myrank: mine, RankList: list}
		} else {
			reply = &pb.SCPvpRank{Myrank: mine, RankList: list}
		}
		if match && stamp > 0 {
			raw, err := proto.Marshal(reply)
			if err != nil {
				return err
			}
			return tx.Create(&db.P11ArenaReceipt{PlayerID: id, Operation: "match-query", Stamp: stamp, RequestHash: p6Digest(rawRequest), Response: raw, BatchID: batch, CreatedAt: now.UnixMilli()}).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{reply}, nil
}

// RecoverP11ArenaMatch 只恢复归属者原双方快照，当前改装不污染已经选择的对手；过期或配置更新要求重新匹配。
func (s *GameService) RecoverP11ArenaMatch(ctx context.Context, id int64, matchID string, now time.Time) (*model.FrozenBattleInput, error) {
	var row db.P11ArenaMatch
	if err := db.GetIns().WithContext(ctx).First(&row, "id=? AND viewer_id=?", matchID, id).Error; err != nil {
		return nil, errors.New("竞技匹配快照不存在或不属于当前角色")
	}
	hash, err := s.p11ArenaConfigurationHash()
	if err != nil {
		return nil, err
	}
	if row.ExpiresAt <= now.UnixMilli() || row.ConfigurationHash != hash || row.CapabilityHash != P6CapabilityHash() || row.SnapshotHash != p11ArenaSnapshotHash(row) {
		return nil, errors.New("竞技匹配已过期、配置变化或快照损坏")
	}
	attacker, defender := &pb.BattlePlayer{}, &pb.BattlePlayer{}
	if proto.Unmarshal(row.Attacker, attacker) != nil || proto.Unmarshal(row.Defender, defender) != nil || attacker.GetPlayerId() != id || defender.GetPlayerId() != row.OpponentID {
		return nil, errors.New("竞技双方归属快照损坏")
	}
	defenderFirst := attacker.GetInst().GetFirstOrder() == 0 && defender.GetInst().GetFirstOrder() == 1
	return s.FreezeBattleInput(model.FrozenBattleInput{Attacker: attacker, Defender: defender, Seed: row.Seed, MaxRound: 40, BattleType: 2, DefenderFirst: defenderFirst})
}

// p11ProbeArenaMatch 为受支持真实队伍建立内部诊断录像；公共协议没有此入口，结果不写次数/名次/奖励。
func (s *GameService) p11ProbeArenaMatch(ctx context.Context, id int64, matchID string, now time.Time) (*model.CombatOutcome, error) {
	frozen, err := s.RecoverP11ArenaMatch(ctx, id, matchID, now)
	if err != nil {
		return nil, err
	}
	var result *model.CombatOutcome
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		// 与P6冻结来源约定保持角色行锁顺序，内部重复诊断只创建一份来源录像。
		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		if _, err := s.CaptureBattleReplay(ctx, tx, id, "p11-arena-diagnostic", matchID, *frozen); err != nil {
			return err
		}
		outcome, err := s.ComputeBattleReplay(ctx, tx, id, "p11-arena-diagnostic", matchID, nil)
		if err != nil {
			return err
		}
		if outcome.Authoritative {
			return errors.New("本批竞技内部诊断不得替代公共权威结算")
		}
		result = outcome
		return nil
	})
	return result, err
}

// prepareP11FriendDuel 仅供内部真实关系冻结，公共CSFriendFight仍经过原生权威门禁。
func (s *GameService) prepareP11FriendDuel(ctx context.Context, id, target, stamp int64, now time.Time) (*db.P11FriendDuel, error) {
	var result *db.P11FriendDuel
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		// 与好友/黑名单写入使用相同双方排序行锁，冻结前关系检查不与并发拉黑错开。
		if _, err := p10LockProfiles(tx, id, target); err != nil {
			return err
		}
		friend, err := p10Friends(tx, id, target)
		if err != nil {
			return err
		}
		blocked, err := p10Blocked(tx, id, target)
		if err != nil {
			return err
		}
		if id == target || !friend || blocked {
			return errors.New("好友切磋需真实双方好友且无黑名单")
		}
		request := &pb.CSFriendFight{PlayerId: proto.Int64(target), ServerId: proto.Int32(1)}
		rawRequest, err := proto.Marshal(request)
		if err != nil {
			return err
		}
		if stamp > 0 {
			var receipt db.P11ArenaReceipt
			err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, "friend-duel-prepare", stamp).Error
			if err == nil {
				if receipt.RequestHash != p6Digest(rawRequest) {
					return errors.New("好友切磋冻结回执请求冲突")
				}
				var old db.P11FriendDuel
				if err = tx.First(&old, "id=? AND viewer_id=?", string(receipt.Response), id).Error; err != nil {
					return err
				}
				result = &old
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		}
		var a, b db.Profile
		if err = tx.First(&a, id).Error; err != nil {
			return err
		}
		if err = tx.First(&b, target).Error; err != nil {
			return err
		}
		_, attacker, err := s.p11ArenaProjection(&a, tx)
		if err != nil {
			return err
		}
		_, defender, err := s.p11ArenaProjection(&b, tx)
		if err != nil {
			return err
		}
		defenderFirst := attacker.GetInst().GetFirstOrder() == 0 && defender.GetInst().GetFirstOrder() == 1
		if err = SetP6FirstOrder(attacker, defender, defenderFirst); err != nil {
			return err
		}
		rawA, err := proto.Marshal(attacker)
		if err != nil {
			return err
		}
		rawB, err := proto.Marshal(defender)
		if err != nil {
			return err
		}
		configHash, err := s.p11ArenaConfigurationHash()
		if err != nil {
			return err
		}
		seed, err := rand.Int(rand.Reader, big.NewInt(math.MaxInt64))
		if err != nil {
			return err
		}
		var cfg db.P11ArenaPolicy
		if err = tx.First(&cfg, constUtil.P11ArenaPolicyID).Error; err != nil {
			return err
		}
		if cfg.MatchMillis < 1000 || cfg.MatchMillis > 3600000 {
			return errors.New("好友切磋冻结期限无效")
		}
		row := db.P11FriendDuel{ID: uuid.NewString(), ViewerID: id, OpponentID: target, Attacker: rawA, Defender: rawB, ConfigurationHash: configHash, CapabilityHash: P6CapabilityHash(), Seed: seed.Int64(), CreatedAt: now.UnixMilli(), ExpiresAt: now.UnixMilli() + cfg.MatchMillis}
		row.SnapshotHash = p11FriendDuelHash(row)
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
		if stamp > 0 {
			if err = tx.Create(&db.P11ArenaReceipt{PlayerID: id, Operation: "friend-duel-prepare", Stamp: stamp, RequestHash: p6Digest(rawRequest), Response: []byte(row.ID), BatchID: row.ID, CreatedAt: now.UnixMilli()}).Error; err != nil {
				return err
			}
		}
		result = &row
		return nil
	})
	return result, err
}
func p11FriendDuelHash(row db.P11FriendDuel) string {
	meta := fmt.Sprintf("friend:11:%s:%d:%d:%d:%s:%s:%d:%d:", row.ID, row.ViewerID, row.OpponentID, row.Seed, row.ConfigurationHash, row.CapabilityHash, row.CreatedAt, row.ExpiresAt)
	return p6Digest(append(append([]byte(meta), row.Attacker...), row.Defender...))
}

// recoverP11FriendDuel 恢复原battleType11，重新检查关系权限；删除/拉黑后旧未付费快照不可使用。
func (s *GameService) recoverP11FriendDuel(ctx context.Context, id int64, duelID string, now time.Time) (*model.FrozenBattleInput, error) {
	var frozen *model.FrozenBattleInput
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.P11FriendDuel
		if err := tx.First(&row, "id=? AND viewer_id=?", duelID, id).Error; err != nil {
			return errors.New("好友切磋冻结不存在或不属于当前角色")
		}
		if _, err := p10LockProfiles(tx, id, row.OpponentID); err != nil {
			return err
		}
		friend, err := p10Friends(tx, id, row.OpponentID)
		if err != nil {
			return err
		}
		blocked, err := p10Blocked(tx, id, row.OpponentID)
		if err != nil {
			return err
		}
		if !friend || blocked {
			return errors.New("好友关系已变化，原切磋冻结不可使用")
		}
		configHash, err := s.p11ArenaConfigurationHash()
		if err != nil {
			return err
		}
		if row.ExpiresAt <= now.UnixMilli() || row.ConfigurationHash != configHash || row.CapabilityHash != P6CapabilityHash() || row.SnapshotHash != p11FriendDuelHash(row) {
			return errors.New("好友切磋已过期、配置变化或快照损坏")
		}
		a, b := &pb.BattlePlayer{}, &pb.BattlePlayer{}
		if proto.Unmarshal(row.Attacker, a) != nil || proto.Unmarshal(row.Defender, b) != nil || a.GetPlayerId() != id || b.GetPlayerId() != row.OpponentID {
			return errors.New("好友切磋双方归属快照损坏")
		}
		defenderFirst := a.GetInst().GetFirstOrder() == 0 && b.GetInst().GetFirstOrder() == 1
		frozen, err = s.FreezeBattleInput(model.FrozenBattleInput{Attacker: a, Defender: b, Seed: row.Seed, MaxRound: 40, BattleType: 11, DefenderFirst: defenderFirst})
		return err
	})
	return frozen, err
}

func (s *GameService) p11ArenaChallenge(ctx context.Context, id int64, request proto.Message) ([]proto.Message, error) {
	return nil, db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		cfg, err := p11ArenaConfig(tx)
		if err != nil {
			return err
		}
		if !cfg.Enabled {
			return errors.New(constUtil.P11ArenaClosed)
		}
		if _, duel := request.(*pb.CSFriendFight); duel {
			if !cfg.DuelEnabled {
				return errors.New(constUtil.P11ArenaClosed)
			}
			b := request.(*pb.CSFriendFight)
			friend, err := p10Friends(tx, id, b.GetPlayerId())
			if err != nil {
				return err
			}
			blocked, err := p10Blocked(tx, id, b.GetPlayerId())
			if err != nil {
				return err
			}
			if !friend || blocked {
				return errors.New("好友切磋需真实双方好友且无黑名单")
			}
		}
		available, err := p7AuthorityAvailable(tx)
		if err != nil {
			return err
		}
		if !available {
			return errors.New(constUtil.P11ArenaClosed)
		}
		// 完整原胜负、名次交换/次数/奖励尚未闭合。SQL或客户端winPlayerId不能提前释放成功战斗。
		return errors.New("竞技原权威结算本批尚未完成")
	})
}
func (s *GameService) DispatchP11Arena(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request.(type) {
	case *pb.CSPvpFindMatchGuide, *pb.CSPvpFightResultGuide:
		result, err := s.p11ArenaGuideAt(ctx, id, request, stamp, LocalPlayNow(ctx))
		return result, true, err
	case *pb.CSPvpFightResult:
		result, err := s.p11ArenaScopedFightAt(ctx, id, request.(*pb.CSPvpFightResult), stamp, LocalPlayNow(ctx))
		return result, true, err
	case *pb.CSFriendFight:
		result, err := s.p11FriendScopedFightAt(ctx, id, request.(*pb.CSFriendFight), stamp, LocalPlayNow(ctx))
		return result, true, err
	case *pb.CSGetGrabData, *pb.CSGrab, *pb.CSBuyGrab, *pb.CSGiveUpGrab, *pb.CSGetTimeRewardInfo, *pb.CSGetTimeReward, *pb.CSGetRandReward:
		result, err := s.p11GrabAt(ctx, id, request, stamp, LocalPlayNow(ctx))
		return result, true, err
	case *pb.CSRefreshTreasFrag, *pb.CSUseAvoidPlunder, *pb.CSTreasureBattleList, *pb.CSFightTreasureStartResult, *pb.CSFightTreasureStart, *pb.CSFightTreasureEnd, *pb.CSSweepTreasureFight:
		result, err := s.p11PlunderAt(ctx, id, request, stamp, LocalPlayNow(ctx))
		return result, true, err
	case *pb.CSLimitPvpFindMatch, *pb.CSLimitPvpRank, *pb.CSBuyLimitPvpTimes, *pb.CSLimitPvpFightResult, *pb.CSLimitPvpRewardInfo, *pb.CSGetLimitPvpRewardInfo:
		result, err := s.p11LimitAt(ctx, id, request, stamp, LocalPlayNow(ctx))
		return result, true, err
	case *pb.CSPvpRank, *pb.CSPvpFindMatch:
		result, err := s.p11ArenaQuery(ctx, id, request, stamp, LocalPlayNow(ctx))
		return result, true, err
	case *pb.CSPvpFight, *pb.CSPvpFightEnd, *pb.CSPvpFightGuide, *pb.CSPvpFightEndGuide:
		result, err := s.p11ArenaChallenge(ctx, id, request)
		return result, true, err
	}
	return nil, false, nil
}
