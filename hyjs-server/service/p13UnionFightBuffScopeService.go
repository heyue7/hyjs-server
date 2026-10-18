package service

import (
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// p13UFBuffScopeCancel 在退场/会员撤销事务内关闭施法者和目标的冲锋，并立即关闭自身恢复法阵。
// 留存已获得的加速时间，重入只读取新的当前时段；没有新的伤害、收益或胜负事实。
func p13UFBuffScopeCancel(tx *gorm.DB, scope string, id int64, now time.Time) ([]proto.Message, error) {
	var speed []db.P13UnionFightSpeedItemEffect
	if err := tx.Where("match_id=? AND ended_at=0 AND (caster_id=? OR target_id=?)", scope, id, id).Order("guid").Find(&speed).Error; err != nil {
		return nil, err
	}
	var off []int64
	targets := map[int64]bool{}
	for _, row := range speed {
		if _, err := p13UFSpeedRead(tx, &row); err != nil {
			return nil, err
		}
		row.EndedAt = max(row.StartedAt, min(row.ExpiresAt, now.UnixMilli()))
		row.SourceHash = p13UFSpeedEffectHash(&row)
		if err := tx.Save(&row).Error; err != nil {
			return nil, err
		}
		targets[row.TargetID] = true
		off = append(off, row.GUID)
	}
	for target := range targets {
		var actor db.P13UnionFightFieldPlayer
		if err := tx.First(&actor, "match_id=? AND player_id=?", scope, target).Error; err != nil {
			return nil, err
		}
		frame, err := p13UFFieldReadActor(&actor)
		if err != nil {
			return nil, err
		}
		if _, err = p13UFSpeedProject(tx, scope, target, frame, now); err != nil {
			return nil, err
		}
		if err = p13UFFieldSaveActor(tx, &actor, frame); err != nil {
			return nil, err
		}
	}
	auraOff, err := p13UFHealAuraCancel(tx, scope, id, now)
	if err != nil {
		return nil, err
	}
	off = append(off, auraOff...)
	if len(off) == 0 {
		return nil, nil
	}
	return []proto.Message{&pb.SCUFElementBuffOff{BuffGuid: off}}, nil
}

func p13UFHealAuraCancel(tx *gorm.DB, scope string, id int64, now time.Time) ([]int64, error) {
	var effects []db.P13UnionFightHealAuraEffect
	if err := tx.Where("match_id=? AND caster_id=? AND ended_at=0", scope, id).Order("guid").Find(&effects).Error; err != nil {
		return nil, err
	}
	var off []int64
	for _, row := range effects {
		if _, err := p13UFHealAuraRead(tx, &row); err != nil {
			return nil, err
		}
		row.EndedAt = max(row.StartedAt, min(row.ExpiresAt, now.UnixMilli()))
		row.SourceHash = p13UFHealAuraEffectHash(&row)
		if err := tx.Save(&row).Error; err != nil {
			return nil, err
		}
		off = append(off, row.GUID)
	}
	if len(off) > 0 {
		var actor db.P13UnionFightFieldPlayer
		if err := tx.First(&actor, "match_id=? AND player_id=?", scope, id).Error; err != nil {
			return nil, err
		}
		frame, err := p13UFFieldReadActor(&actor)
		if err != nil {
			return nil, err
		}
		if _, err = p13UFHealAuraProject(tx, scope, id, frame, now); err != nil {
			return nil, err
		}
		if err = p13UFFieldSaveActor(tx, &actor, frame); err != nil {
			return nil, err
		}
	}
	return off, nil
}
