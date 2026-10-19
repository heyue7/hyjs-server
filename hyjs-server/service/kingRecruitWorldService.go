package service

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sort"
)

type kingRecruitAvailablePool struct {
	Key   string
	World int64
	EndAt int64
}

func (s *GameService) InitializeKingRecruitWorlds(ctx context.Context) error {
	return db.GetIns().WithContext(ctx).AutoMigrate(&db.KingRecruitSelection{})
}

// Every source is checked against its actual consumer window and original rules.
func (s *GameService) kingRecruitAvailablePools(tx *gorm.DB, now int64) ([]kingRecruitAvailablePool, bool, error) {
	out := []kingRecruitAvailablePool{}
	tx = tx.WithContext(context.WithValue(tx.Statement.Context, constUtil.GameServiceContextKey, s))
	owner := GameCalendarOwner(tx.Statement.Context)
	keys := []string{}
	scheduled := false
	if tx.Migrator().HasTable(&db.AdminActivityOccurrence{}) {
		var count int64
		if err := tx.Model(&db.AdminActivityOccurrence{}).Where("server_id=? AND mode=?", s.OriginalServerID(), constUtil.AdminKingRecruitMode).Count(&count).Error; err != nil {
			return nil, false, err
		}
		scheduled = count > 0
		if scheduled {
			if owner < 1 {
				return nil, true, constUtil.ErrAdminPermission
			}
			var rows []db.AdminActivityOccurrence
			if err := tx.Where("server_id=? AND mode=? AND timing<>? AND enabled=? AND archived=? AND start_at<=? AND end_at>?", s.OriginalServerID(), constUtil.AdminKingRecruitMode, "character-day", true, false, now, now).Order("start_at,id").Find(&rows).Error; err != nil {
				return nil, true, err
			}
			for _, row := range rows {
				allowed, err := adminAcceptActivityTemplate(tx, owner, row)
				if err != nil {
					return nil, true, err
				}
				if allowed {
					keys = append(keys, row.ConsumerKey)
				}
			}
			if tx.Migrator().HasTable(&db.AdminCharacterActivityInstance{}) {
				var rows []db.AdminCharacterActivityInstance
				if err := tx.Where("player_id=? AND server_id=? AND mode=? AND start_at<=? AND end_at>?", owner, s.OriginalServerID(), constUtil.AdminKingRecruitMode, now, now).Order("start_at,occurrence_id").Find(&rows).Error; err != nil {
					return nil, true, err
				}
				for _, row := range rows {
					allowed, err := adminCharacterActivityAccess(tx, row.ConsumerKey)
					if err != nil {
						return nil, true, err
					}
					if allowed {
						keys = append(keys, row.ConsumerKey)
					}
				}
			}
		}
	}
	if !scheduled {
		keys = []string{"recruit_3"}
	}
	seen := map[int64]bool{}
	var level int32
	if owner > 0 {
		var profile db.Profile
		if err := tx.First(&profile, owner).Error; err != nil {
			return nil, scheduled, err
		}
		player, _, err := decodeProfile(&profile)
		if err != nil {
			return nil, scheduled, err
		}
		level = player.GetLevel()
	}
	for _, key := range keys {
		var pool db.GameplayPool
		if err := tx.First(&pool, "id=?", key).Error; err != nil {
			return nil, scheduled, err
		}
		if !pool.Enabled || pool.StartAt > now || pool.EndAt > 0 && now >= pool.EndAt || owner > 0 && level < pool.MinLevel {
			continue
		}
		world := int64(0)
		if tx.Migrator().HasTable(&db.KingRecruitActivitySnapshot{}) {
			var snapshot db.KingRecruitActivitySnapshot
			err := tx.First(&snapshot, "pool_id=?", key).Error
			if err == nil {
				var source adminKingRecruitSource
				if json.Unmarshal(snapshot.Payload, &source) != nil || source.Pool.ID != key {
					return nil, scheduled, constUtil.ErrAdminConfiguration
				}
				world = source.World
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, scheduled, err
			} else if key != "recruit_3" {
				return nil, scheduled, constUtil.ErrAdminConfiguration
			}
		}
		var rewards []db.GameplayPoolReward
		if err := tx.Where("pool_id=?", key).Find(&rewards).Error; err != nil {
			return nil, scheduled, err
		}
		if err := s.validateKingRecruitActivityPool(pool, rewards, world); err != nil {
			return nil, scheduled, err
		}
		if seen[world] || world == 0 && len(out) > 0 || world > 0 && seen[0] {
			return nil, scheduled, constUtil.ErrAdminConfiguration
		}
		seen[world] = true
		out = append(out, kingRecruitAvailablePool{Key: key, World: world, EndAt: pool.EndAt})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].World != out[j].World {
			return out[i].World < out[j].World
		}
		return out[i].Key < out[j].Key
	})
	return out, scheduled, nil
}

// Selection and original draw both hold the profile lock; no counters or receipts are reset.
func (s *GameService) KingRecruitWorlds(ctx context.Context, id int64, session []byte, poolID, version string) (*model.KingRecruitWorldsResponse, error) {
	ctx = context.WithValue(ctx, constUtil.GameServiceContextKey, s)
	var profile db.Profile
	if err := db.GetIns().WithContext(ctx).First(&profile, id).Error; err != nil {
		return nil, err
	}
	if AdminProfileServerID(profile) != s.OriginalServerID() {
		return nil, constUtil.ErrAdminPermission
	}
	var err error
	ctx, err = s.BindLocalPlayContext(ctx, &profile)
	if err != nil {
		return nil, err
	}
	if err = s.PrepareAdminCharacterActivitySchedules(ctx, id); err != nil {
		return nil, err
	}
	out := &model.KingRecruitWorldsResponse{Worlds: []model.KingRecruitWorld{}}
	err = updateProfileWithTx(ctx, id, func(tx *gorm.DB, _ *pb.Player, _ *model.GameState) error {
		var current db.Profile
		if err := tx.Select("id", "session", "server_id").First(&current, id).Error; err != nil {
			return err
		}
		if AdminProfileServerID(current) != s.OriginalServerID() || len(session) != 16 || subtle.ConstantTimeCompare(current.Session, session) != 1 {
			return constUtil.ErrAdminPermission
		}
		candidates, _, err := s.kingRecruitAvailablePools(tx, LocalPlayNow(ctx).UnixMilli())
		if err != nil {
			return err
		}
		out.Version = adminVersion(candidates)
		selected := db.KingRecruitSelection{PlayerID: id}
		err = tx.First(&selected, "player_id=?", id).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		present := false
		for _, candidate := range candidates {
			label := map[int64]string{0: "轮换世界", 1: "SAO", 2: "ALO", 3: "GGO"}[candidate.World]
			out.Worlds = append(out.Worlds, model.KingRecruitWorld{PoolID: candidate.Key, World: candidate.World, Label: label, EndAt: candidate.EndAt})
			if selected.PoolID == candidate.Key {
				present = true
			}
		}
		if poolID != "" {
			if version != out.Version {
				return constUtil.ErrAdminConfigConflict
			}
			present = false
			for _, candidate := range candidates {
				if candidate.Key == poolID {
					present = true
					selected.PoolID = poolID
					break
				}
			}
			if !present {
				return constUtil.ErrKingRecruitActivityClosed
			}
		} else if !present {
			selected.PoolID = ""
			if len(candidates) > 0 {
				selected.PoolID = candidates[0].Key
			}
		}
		if err := tx.Save(&selected).Error; err != nil {
			return err
		}
		out.SelectedPoolID = selected.PoolID
		return nil
	})
	return out, err
}
