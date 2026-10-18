package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// settleResourceRecovery 按 APK Countdown 的整周期算法结算离线和在线恢复。
// lastRefreshTime 是当前周期起点的 Unix 毫秒；保留不足一个周期的余数。
// 满值及道具造成的超上限余额保留原值，跨日不会额外重置或赠送资源。
func settleResourceRecovery(player *pb.Player, nowMS int64) (bool, error) {
	if player == nil || nowMS <= 0 || player.GetEnergy() < 0 || player.GetStamina() < 0 {
		return false, errors.New(constUtil.ErrResourceRecoveryState)
	}
	var (
		energy, energyTime   = recoverResourceValue(player.GetEnergy(), player.GetEnergyLastRefreshTime(), constUtil.EnergyRecoveryLimit, constUtil.EnergyRecoveryIntervalMS, nowMS)
		stamina, staminaTime = recoverResourceValue(player.GetStamina(), player.GetStaminaLastRefreshTime(), constUtil.StaminaRecoveryLimit, constUtil.StaminaRecoveryIntervalMS, nowMS)
		changed              = player.EnergyLastRefreshTime == nil || player.StaminaLastRefreshTime == nil || energy != player.GetEnergy() || stamina != player.GetStamina() || energyTime != player.GetEnergyLastRefreshTime() || staminaTime != player.GetStaminaLastRefreshTime()
	)
	player.Energy = proto.Int32(energy)
	player.Stamina = proto.Int32(stamina)
	player.EnergyLastRefreshTime = proto.Int64(energyTime)
	player.StaminaLastRefreshTime = proto.Int64(staminaTime)
	return changed, nil
}

func recoverResourceValue(value int32, lastMS int64, limit int32, intervalMS, nowMS int64) (int32, int64) {
	// 旧存档缺失时间时从首次读取开始计时；未来时间按当前时刻修正。
	// 这项迁移规则避免把缺失的 0 当成 1970 年并立即补满。
	if lastMS <= 0 || lastMS > nowMS {
		return value, nowMS
	}
	if value >= limit {
		return value, lastMS
	}
	var (
		cycles = (nowMS - lastMS) / intervalMS
		space  = int64(limit - value)
	)
	if cycles > space {
		cycles = space
	}
	return value + int32(cycles), lastMS + cycles*intervalMS
}

// decodeProfileForUpdate 在持有角色行锁的业务事务内先补足应恢复资源。
// 失败事务不提交游标；后续请求仍能结算同一周期，成功时和消费原子保存。
func decodeProfileForUpdate(row *db.Profile) (*pb.Player, *model.GameState, error) {
	player, game, err := decodeProfile(row)
	if err != nil {
		return nil, nil, err
	}
	if _, err = settleResourceRecovery(player, time.Now().UnixMilli()); err != nil {
		return nil, nil, err
	}
	// 固定本次事务开始时的资源基线。保存时不再用较晚时钟重算，
	// 避免无消费的长事务跨过恢复边界后被误判为满值消费。
	raw, err := json.Marshal(player)
	if err != nil {
		return nil, nil, err
	}
	baseline := string(raw)
	row.Player = &baseline
	return player, game, nil
}

// finalizeResourceRecovery 在业务扣减或奖励后修正周期起点。
// APK Countdown.set_Current 只在满值/超上限消费至未满时重新开始计时。
// 调用者须在 fn 前结算并记录余额；fn 后调用，再与存档一起提交。
func finalizeResourceRecovery(player *pb.Player, energyBefore, staminaBefore int32, nowMS int64) error {
	if player == nil || nowMS <= 0 || player.GetEnergy() < 0 || player.GetStamina() < 0 {
		return errors.New(constUtil.ErrResourceRecoveryState)
	}
	if energyBefore >= constUtil.EnergyRecoveryLimit && player.GetEnergy() < constUtil.EnergyRecoveryLimit {
		player.EnergyLastRefreshTime = proto.Int64(nowMS)
	}
	if staminaBefore >= constUtil.StaminaRecoveryLimit && player.GetStamina() < constUtil.StaminaRecoveryLimit {
		player.StaminaLastRefreshTime = proto.Int64(nowMS)
	}
	return nil
}

// resourceProfile 在行锁内结算并返回资源与其他游戏状态的同一份最新存档。
// 普通查询、CSPlayer 和登录均可复用；数据库时间游标防止并发重复恢复。
func (s *GameService) resourceProfile(ctx context.Context, id int64) (*pb.Player, *model.GameState, error) {
	var (
		player *pb.Player
		game   *model.GameState
	)
	// 恢复结算属于查询前置步骤，不沿用外层业务消息触发活动进度回调。
	readCtx := context.WithValue(ctx, constUtil.GameRequestContextKey, struct{}{})
	err := db.GetIns().WithContext(readCtx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		var err error
		player, game, err = decodeProfile(&row)
		if err != nil {
			return err
		}
		clockChanged, err := ServerGameClockPrepareProfile(tx, &row, player)
		if err != nil {
			return err
		}
		penaltyChanged, err := s.syncP12FieldPVPPenaltyForRead(tx, player, game, time.Now())
		if err != nil {
			return err
		}
		if err = s.RefreshP5GuildPrayerPower(readCtx, tx, player, game, p13UFNow(readCtx)); err != nil {
			return err
		}
		changed, err := settleResourceRecovery(player, time.Now().UnixMilli())
		if err != nil || !changed && !penaltyChanged && !clockChanged {
			return err
		}
		return saveProfile(tx, &row, player, game)
	})
	return player, game, err
}

// decodeProfileForCalendarUpdate 在已持有全服与角色行锁的事务中按高水位恢复一次。
// 多人动作读取另一角色时不替对方结算，等待其本人的下一次认证请求。
func decodeProfileForCalendarUpdate(tx *gorm.DB, row *db.Profile) (*pb.Player, *model.GameState, error) {
	player, game, err := decodeProfileForUpdate(row)
	if err != nil {
		return nil, nil, err
	}
	if !ServerGameClockContextBound(tx.Statement.Context) || row.Id != GameCalendarOwner(tx.Statement.Context) {
		return player, game, nil
	}
	changed, err := ServerGameClockPrepareProfile(tx, row, player)
	if err != nil {
		return nil, nil, err
	}
	if changed {
		neutral := context.WithValue(tx.Statement.Context, constUtil.GameRequestContextKey, struct{}{})
		if err = saveProfile(tx.WithContext(neutral), row, player, game); err != nil {
			return nil, nil, err
		}
	}
	return player, game, nil
}
