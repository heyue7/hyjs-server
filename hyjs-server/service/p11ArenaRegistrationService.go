package service

import (
	"bytes"
	"context"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// Startup is the only full registry rebuild. It runs before serving requests;
// the allocator is initialized after the existing history-aware seat migration.
func (s *GameService) initializeP11ArenaRanks(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var cfg db.P11ArenaPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&cfg, constUtil.P11ArenaPolicyID).Error; err != nil {
			return err
		}
		var allocator db.P11ArenaRankAllocator
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&allocator, "policy_id=?", cfg.ID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil {
			cfg.NextRank = max(cfg.NextRank, allocator.NextRank)
		}
		return s.p11SyncArenaMembers(tx, &cfg, LocalPlayNow(ctx))
	})
}

func p11SetArenaProjection(tx *gorm.DB, member *db.P11ArenaMember, info *pb.SimplePlayerInfo, battle *pb.BattlePlayer, now time.Time) error {
	rawInfo, err := proto.Marshal(info)
	if err != nil {
		return err
	}
	rawBattle, err := proto.Marshal(battle)
	if err != nil {
		return err
	}
	hash := p6Digest(append(append([]byte(nil), rawInfo...), rawBattle...))
	if bytes.Equal(member.Info, rawInfo) && bytes.Equal(member.Battle, rawBattle) && member.SnapshotHash == hash {
		return nil
	}
	member.Info, member.Battle, member.SnapshotHash, member.UpdatedAt = rawInfo, rawBattle, hash, now.UnixMilli()
	return tx.Model(member).Updates(map[string]any{"info": rawInfo, "battle": rawBattle, "snapshot_hash": hash, "updated_at": member.UpdatedAt}).Error
}

// Only missing/re-eligible members need preparation. The indexed candidate
// page contains identities, never everyone's inventory or formation JSON.
func (s *GameService) p11ArenaNewMemberPage(ctx context.Context, after int64) ([]int64, error) {
	minimum, err := s.p10Setting("PVPLevel")
	if err != nil {
		return nil, err
	}
	var ids []int64
	err = db.GetIns().WithContext(ctx).Table("profiles AS p").
		Select("p.id").Joins("LEFT JOIN p11_arena_members AS m ON m.player_id=p.id").
		Where("p.server_id=? AND p.player_level>=? AND p.id>? AND p.game IS NOT NULL AND (m.player_id IS NULL OR m.active=?)", s.OriginalServerID(), minimum, after, false).
		Order("p.id").Limit(64).Scan(&ids).Error
	return ids, err
}

// Eligibility discovery is bounded. A worker also advances a keyset cursor, so
// a malformed candidate cannot indefinitely hide later eligible identities.
func (s *GameService) p11PrepareArenaMembership(ctx context.Context, id int64, now time.Time) error {
	ids, err := s.p11ArenaNewMemberPage(ctx, 0)
	if err != nil {
		return err
	}
	ownSeen := false
	for _, candidate := range ids {
		if candidate == id {
			ownSeen = true
		}
		if err := s.p11EnrollArenaMember(ctx, candidate, now); err != nil {
			return err
		}
	}
	if !ownSeen {
		return s.p11EnrollArenaMember(ctx, id, now)
	}
	return nil
}

func (s *GameService) p11EnrollArenaMember(ctx context.Context, id int64, now time.Time) error {
	// Already active roles do not acquire the allocation resource at all.
	var member db.P11ArenaMember
	read := db.GetIns().WithContext(ctx).Select("player_id", "active").First(&member, "player_id=?", id).Error
	if read == nil && member.Active {
		return nil
	}
	if read != nil && !errors.Is(read, gorm.ErrRecordNotFound) {
		return read
	}
	return s.p11ArenaTransaction(ctx, now, func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var profile db.Profile
		// Registration reads a committed eligibility snapshot and changes only
		// the rank registry. It never takes another role's inventory lock;
		// actual matching and settlement recheck current level and formation.
		if err := tx.First(&profile, id).Error; err != nil {
			return err
		}
		if AdminProfileServerID(profile) != s.OriginalServerID() {
			return nil
		}
		minimum, err := s.p10Setting("PVPLevel")
		if err != nil {
			return err
		}
		if profile.PlayerLevel == nil || *profile.PlayerLevel < int64(minimum) {
			return nil
		}
		cfg, err := p11ArenaConfig(tx)
		if err != nil {
			return err
		}
		if _, err = s.PrepareP11ArenaDailyMutation(ctx, tx, now); err != nil {
			return err
		}
		// Build and validate before taking the shared allocation row's X lock.
		info, battle, err := s.p11ArenaProjection(&profile, tx)
		if err != nil {
			return nil
		} // invalid profiles do not block other registrants
		rawInfo, err := proto.Marshal(info)
		if err != nil {
			return err
		}
		rawBattle, err := proto.Marshal(battle)
		if err != nil {
			return err
		}
		err = tx.First(&member, "player_id=?", id).Error
		if err == nil {
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&member, "player_id=?", id).Error; err != nil {
				return err
			}
			if member.Active {
				return nil
			}
			if err := p11SetArenaProjection(tx, &member, info, battle, now); err != nil {
				return err
			}
			return tx.Model(&member).Updates(map[string]any{"active": true, "updated_at": now.UnixMilli()}).Error
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var allocator db.P11ArenaRankAllocator
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&allocator, "policy_id=?", cfg.ID).Error; err != nil {
			return err
		}
		// Recheck with a current locking read after acquiring the allocator;
		// no absent-member gap lock is held while waiting for allocation.
		var current db.P11ArenaMember
		err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "player_id=?", id).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if allocator.NextRank < constUtil.P11ArenaVirtualSlots+1 || allocator.NextRank >= constUtil.P11ArenaNPCMaxRank {
			return errors.New("竞技名次分配器已满或无效")
		}
		member = db.P11ArenaMember{PlayerID: id, Rank: allocator.NextRank, Active: true, Info: rawInfo, Battle: rawBattle, SnapshotHash: p6Digest(append(append([]byte(nil), rawInfo...), rawBattle...)), RankRewardBest: constUtil.P11ArenaVirtualSlots + 1, CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli()}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}
		return tx.Model(&allocator).Update("next_rank", allocator.NextRank+1).Error
	})
}

// Selected opponents are projected from the same committed query snapshot.
// The projections are private to this match; other players' member rows are not
// rewritten or locked just because somebody opened the match page.
func (s *GameService) p11RefreshSelectedArenaSnapshots(tx *gorm.DB, selected []db.P11ArenaMember) error {
	var ids []int64
	for _, member := range selected {
		if !p11ArenaNPCID(member.PlayerID) {
			ids = append(ids, member.PlayerID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var profiles []db.Profile
	if err := tx.Where("id IN ?", ids).Find(&profiles).Error; err != nil {
		return err
	}
	rows := make(map[int64]*db.Profile, len(profiles))
	for i := range profiles {
		rows[profiles[i].Id] = &profiles[i]
	}
	minimum, err := s.p10Setting("PVPLevel")
	if err != nil {
		return err
	}
	for i := range selected {
		member := &selected[i]
		if p11ArenaNPCID(member.PlayerID) {
			continue
		}
		profile := rows[member.PlayerID]
		if profile == nil || AdminProfileServerID(*profile) != s.OriginalServerID() {
			return errors.New(constUtil.P11ArenaFightRefreshRequired)
		}
		info, battle, err := s.p11ArenaProjection(profile, tx)
		if err != nil {
			return err
		}
		if info.GetLevel() < minimum {
			return errors.New(constUtil.P11ArenaFightRefreshRequired)
		}
		if member.Info, err = proto.Marshal(info); err != nil {
			return err
		}
		if member.Battle, err = proto.Marshal(battle); err != nil {
			return err
		}
		member.SnapshotHash = p6Digest(append(append([]byte(nil), member.Info...), member.Battle...))
	}
	return nil
}
