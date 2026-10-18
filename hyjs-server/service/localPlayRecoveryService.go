package service

import (
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// applyLocalPlayRecoveryJump 仅由已授权的个人日期事务调用。新推进的一天
// 按原倒计时恢复体力/耐力；回拨与重新访问旧日期不再发放同一天的恢复。
// 存档游标仍用真实时间，普通恢复和未启用工具的账号不改变时钟。
func applyLocalPlayRecoveryJump(tx *gorm.DB, player *pb.Player, id, beforeDays, afterDays int64, real time.Time) (bool, error) {
	if beforeDays < -3650 || beforeDays > 3650 || afterDays < -3650 || afterDays > 3650 {
		return false, errors.New(constUtil.LocalPlayRangeMessage)
	}
	high := max(int64(0), beforeDays)
	if afterDays <= high {
		return false, nil
	}
	// 已提交的日期回执就是高水位来源，旧档无需补造游玩记录。
	var history []db.LocalPlayClockOperation
	if err := tx.Select("response").Where("player_id=?", id).Find(&history).Error; err != nil {
		return false, err
	}
	for _, operation := range history {
		var previous model.LocalPlayTimeView
		if json.Unmarshal(operation.Response, &previous) != nil || previous.OffsetDays < -3650 || previous.OffsetDays > 3650 {
			return false, errors.New(constUtil.LocalPlayInvalidMessage)
		}
		high = max(high, previous.OffsetDays)
	}
	if afterDays > high {
		if _, err := settleResourceRecovery(player, real.UnixMilli()); err != nil {
			return false, err
		}
		virtual := real.Add(time.Duration(afterDays-high) * 24 * time.Hour).UnixMilli()
		energy, _ := recoverResourceValue(player.GetEnergy(), player.GetEnergyLastRefreshTime(), constUtil.EnergyRecoveryLimit, constUtil.EnergyRecoveryIntervalMS, virtual)
		stamina, _ := recoverResourceValue(player.GetStamina(), player.GetStaminaLastRefreshTime(), constUtil.StaminaRecoveryLimit, constUtil.StaminaRecoveryIntervalMS, virtual)
		changed := energy != player.GetEnergy() || stamina != player.GetStamina()
		if energy != player.GetEnergy() {
			player.Energy = proto.Int32(energy)
			player.EnergyLastRefreshTime = proto.Int64(real.UnixMilli())
		}
		if stamina != player.GetStamina() {
			player.Stamina = proto.Int32(stamina)
			player.StaminaLastRefreshTime = proto.Int64(real.UnixMilli())
		}
		return changed, nil
	}
	return false, nil
}
