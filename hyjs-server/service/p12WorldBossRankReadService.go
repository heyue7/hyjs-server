package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

type p12WorldBossRankRow struct {
	PlayerID int64
	Damage   int64
}

// p12WorldBossRankRead only observes an existing window and its committed battles.
func (s *GameService) p12WorldBossRankRead(ctx context.Context, id int64, now time.Time) ([]proto.Message, error) {
	var reply *pb.SCWorldBossRank
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}
		var caller db.Profile
		if err := tx.Select("id", "player").First(&caller, id).Error; err != nil {
			return err
		}
		if caller.Player == nil {
			return errors.New("请先创建角色")
		}
		var player pb.Player
		if err := json.Unmarshal([]byte(*caller.Player), &player); err != nil {
			return err
		}
		if player.GetPlayerId() != id {
			return errors.New("世界首领查询角色归属无效")
		}
		var policy db.P12WorldBossPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		cfg, def, err := p12WorldBossValidateConfig(tx, &policy)
		if err != nil {
			return err
		}
		if !cfg.Enabled {
			return errors.New(constUtil.P12WorldBossClosed)
		}
		key, start, end, err := p12WorldBossWindow(*def, now)
		if err != nil {
			return err
		}
		var cycle db.P12WorldBossCycle
		err = tx.First(&cycle, "window_key=?", key).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if _, _, err := p12WorldBossProjection(*def, cfg.Level, cfg.PositionID); err != nil {
				return err
			}
			reply = &pb.SCWorldBossRank{BossHpPercent: proto.Float64(1), State: proto.Int32(0)}
			return nil
		}
		if err != nil {
			return err
		}
		if cycle.StartsAt != start.UnixMilli() || cycle.EndsAt != end.UnixMilli() || cycle.DefinitionHash != p6Digest([]byte(cycle.DefinitionJSON)) {
			return errors.New("既有世界首领窗口/定义损坏")
		}
		var frozen p12WorldBossDefinition
		if err := json.Unmarshal([]byte(cycle.DefinitionJSON), &frozen); err != nil {
			return err
		}
		if _, _, err := p12WorldBossProjection(frozen, cycle.Level, cycle.PositionID); err != nil {
			return err
		}
		active := cycle.State == constUtil.P12WorldBossActive || cycle.State == constUtil.P12WorldBossScheduled && !now.Before(start)
		state := int32(0)
		if active {
			state = 1
			if _, _, _, err := p12WorldBossWindow(frozen, time.UnixMilli(cycle.EndsAt)); err != nil {
				return err
			}
		}
		if s.P12WorldBossBattleReady {
			var battlePolicy db.P12WorldBossBattlePolicy
			if err := tx.First(&battlePolicy, 1).Error; err != nil {
				return err
			}
			if battlePolicy.Enabled && active {
				reply, err = p12WorldBossBattleRankView(tx, cycle.ID, id)
				return err
			}
		}
		reply = &pb.SCWorldBossRank{BossHpPercent: proto.Float64(1), State: proto.Int32(state)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{reply}, nil
}

// p12WorldBossBattleRankView reads one MVCC snapshot without locking shared combat rows.
func p12WorldBossBattleRankView(tx *gorm.DB, instanceID string, id int64) (*pb.SCWorldBossRank, error) {
	var instance db.P12WorldBossBattleInstance
	err := tx.Select("id", "total_hp", "current_hp", "killer_id").First(&instance, "id=?", instanceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &pb.SCWorldBossRank{BossHpPercent: proto.Float64(1), State: proto.Int32(1)}, nil
	}
	if err != nil {
		return nil, err
	}
	var rows []p12WorldBossRankRow
	if err := tx.Model(&db.P12WorldBossBattleReceipt{}).
		Select("player_id, SUM(applied_damage) AS damage").
		Where("instance_id=? AND committed=? AND applied_damage>0", instanceID, true).
		Group("player_id").Order("damage DESC, player_id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return p12WorldBossRankProjection(tx, &instance, rows, id)
}

func p12WorldBossRankProjection(tx *gorm.DB, instance *db.P12WorldBossBattleInstance, rows []p12WorldBossRankRow, id int64) (*pb.SCWorldBossRank, error) {
	const batchSize = 128
	names := make(map[int64]string, len(rows))
	for first := 0; first < len(rows); first += batchSize {
		last := min(first+batchSize, len(rows))
		ids := make([]int64, 0, last-first)
		for _, row := range rows[first:last] {
			ids = append(ids, row.PlayerID)
		}
		var profiles []playerRank48Projection
		if err := tx.Table("profiles").Select(playerRank48ProjectionSQL(tx, false, true)).
			Where("id IN ? AND player IS NOT NULL", ids).Find(&profiles).Error; err != nil {
			return nil, err
		}
		if len(profiles) != len(ids) {
			return nil, gorm.ErrRecordNotFound
		}
		for _, profile := range profiles {
			names[profile.ID] = profile.Name
		}
	}
	state := int32(1)
	if instance.CurrentHP == 0 {
		state = 2
	}
	out := &pb.SCWorldBossRank{BossHpPercent: proto.Float64(float64(instance.CurrentHP) / float64(instance.TotalHP)), State: proto.Int32(state)}
	for index, row := range rows {
		info := &pb.WorldBossDamageInfo{Damage: proto.Float64(float64(row.Damage)), PlayerName: proto.String(names[row.PlayerID]), Rank: proto.Int32(int32(index + 1))}
		out.DamageInfos = append(out.DamageInfos, info)
		if row.PlayerID == id {
			out.MyDamageInfo = info
		}
		if row.PlayerID == instance.KillerID {
			out.LastDamageInfo = info
		}
	}
	return out, nil
}
