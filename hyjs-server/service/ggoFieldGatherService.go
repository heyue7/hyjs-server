package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

type ggoGatherFrozen struct {
	p8GatherConfig
	StartRequestID    string   `json:"ggoStartRequestId"`
	Line              int64    `json:"ggoLine"`
	EconomyVersion    string   `json:"ggoEconomyVersion"`
	ExchangeValues    [6]int64 `json:"ggoExchangeValues"`
	Chips             [6]int64 `json:"ggoChips"`
	MaterialNumerator int64    `json:"ggoMaterialNumerator"`
}

func ggoChipValues(cfg *model.AdminFieldGGOConfiguration) ([6]int64, error) {
	var values [6]int64
	for _, chip := range cfg.Chips {
		var index int
		if _, err := fmt.Sscanf(chip.ItemID, "item_%d", &index); err != nil || index < 9701 || index > 9706 || chip.ExchangeValue == nil || *chip.ExchangeValue < 1 {
			return values, errors.New("沙漠芯片配置无效")
		}
		values[index-9701] = *chip.ExchangeValue
	}
	for _, v := range values {
		if v < 1 {
			return values, errors.New("沙漠芯片配置不完整")
		}
	}
	return values, nil
}

func ggoRollChip(cfg *model.AdminFieldGGOConfiguration, trigger int64) ([6]int64, error) {
	var result [6]int64
	if trigger < 0 || trigger > 10000 {
		return result, errors.New("沙漠芯片概率无效")
	}
	draw, err := gemRoll(10000)
	if err != nil {
		return result, err
	}
	if draw >= trigger {
		return result, nil
	}
	draw, err = gemRoll(10000)
	if err != nil {
		return result, err
	}
	for _, chip := range cfg.Chips {
		if chip.DropWeight == nil || *chip.DropWeight < 0 {
			return result, errors.New("沙漠芯片品质无效")
		}
		if draw < *chip.DropWeight {
			var index int
			if _, err = fmt.Sscanf(chip.ItemID, "item_%d", &index); err != nil || index < 9701 || index > 9706 {
				return result, errors.New("沙漠芯片身份无效")
			}
			result[index-9701] = 1
			return result, nil
		}
		draw -= *chip.DropWeight
	}
	return result, errors.New("沙漠芯片权重无效")
}

func ggoGatherConfig(s *GameService, cfg *model.AdminFieldGGOConfiguration, node ggoResourceNode, point model.AdminFieldGGOPoint, startID string, line int64) (*ggoGatherFrozen, error) {
	var tier *model.AdminFieldGGOResourceTier
	for i := range cfg.Runtime.Resource.Tiers {
		if cfg.Runtime.Resource.Tiers[i].Level == node.Level {
			tier = &cfg.Runtime.Resource.Tiers[i]
			break
		}
	}
	if tier == nil {
		return nil, errors.New("沙漠资源阶数无效")
	}
	stand := point.StandPosition
	if stand == nil {
		stand = point.Position
	}
	if stand == nil || stand.X == nil || stand.Y == nil || stand.Z == nil {
		return nil, errors.New("沙漠采集位置未配置")
	}
	duration, cooldown, drops := cfg.Runtime.Resource.MineGatherMillis, cfg.Runtime.Resource.MineCooldownSeconds, tier.MineDrops
	if node.Kind == "fish" {
		duration, cooldown, drops = cfg.Runtime.Resource.FishGatherMillis, cfg.Runtime.Resource.FishCooldownSeconds, tier.FishDrops
	}
	chips, err := ggoRollChip(cfg, tier.ChipTriggerBps)
	if err != nil {
		return nil, err
	}
	values, err := ggoChipValues(cfg)
	if err != nil {
		return nil, err
	}
	frozen := &ggoGatherFrozen{
		p8GatherConfig: p8GatherConfig{ResourceID: fmt.Sprintf("group_%s_%03d", node.Kind, node.Level), Kind: node.Kind, Level: int32(node.Level), GatherMS: duration, CooldownMS: cooldown * 1000, Times: int32(cfg.Runtime.Resource.Capacity), Fatigue: int32(cfg.Runtime.Resource.Fatigue), Reward: adminFieldDropText(drops), Point: &pb.PointNew{X: proto.Float32(float32(*stand.X)), Y: proto.Float32(float32(*stand.Y)), Z: proto.Float32(float32(*stand.Z))}, Radius: point.Radius, Source: "ggo-v4:" + s.GGOFieldRevision},
		StartRequestID: startID, Line: line, EconomyVersion: cfg.Runtime.EconomyVersion, ExchangeValues: values, Chips: chips, MaterialNumerator: cfg.Runtime.Resource.MaterialNumerator,
	}
	raw, err := json.Marshal(frozen)
	if err != nil {
		return nil, err
	}
	if _, err = p8DecodeConfig(string(raw)); err != nil {
		return nil, err
	}
	return frozen, nil
}

func ggoStartHash(req model.GGOFieldGatherStartRequest) string {
	raw, _ := json.Marshal(struct {
		PointID    string `json:"pointId"`
		Generation int64  `json:"generation"`
	}{req.PointID, req.Generation})
	return p6Digest(raw)
}

func ggoGatherRequirementError(kind string, requiredLevel int64, level int32, requiredFatigue int64, fatigue int32) error {
	if int64(level) < requiredLevel {
		name := "冶炼"
		if kind == "fish" {
			name = "料理"
		}
		return ggoFieldError(constUtil.GGOFieldInsufficientLife, fmt.Sprintf("需要%s等级%d，当前等级%d", name, requiredLevel, level))
	}
	if int64(fatigue) < requiredFatigue {
		return ggoFieldError(constUtil.GGOFieldInsufficientLife, fmt.Sprintf("疲劳不足，当前%d，需要%d", fatigue, requiredFatigue))
	}
	return nil
}

// GGOFieldGatherStart freezes the result, reserves one public point and records
// its UUID under the same profile-then-line transaction.
func (s *GameService) GGOFieldGatherStart(ctx context.Context, req model.GGOFieldGatherStartRequest, session []byte) (*model.GGOFieldGatherStartResponse, error) {
	var out model.GGOFieldGatherStartResponse
	accepted := false
	err := s.ggoWithActor(ctx, req.GGOFieldAuth, session, func(tx *gorm.DB, a *ggoFieldActor) error {
		hash := ggoStartHash(req)
		var previous db.GGOFieldReceipt
		e := tx.First(&previous, "player_id=? AND request_id=?", a.Row.Id, req.RequestID).Error
		if e == nil {
			if previous.Operation == constUtil.GGOFieldOpGatherTombstone {
				return ggoFieldError(constUtil.GGOFieldStaleGeneration, "采集请求已取消")
			}
			if previous.Operation != constUtil.GGOFieldOpGatherStart || previous.RequestHash != hash {
				return ggoFieldError(constUtil.GGOFieldRequestConflict, "请求编号已用于其他操作")
			}
			return json.Unmarshal(previous.Response, &out)
		}
		if !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if a.Game.GGOField == nil {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请先从向导进入危险区")
		}
		if a.Game.GGOField.ProtectionUntil > a.Now {
			return ggoFieldError(constUtil.GGOFieldProtectionConfirmation, "确认解除入场保护后请再次操作")
		}
		if err := P12FieldPVPRequireIdle(tx, a.Row.Id, time.UnixMilli(a.Now)); err != nil {
			return ggoFieldError(constUtil.GGOFieldBusy, "当前角色正在战斗")
		}
		if a.Game.Field.Side.GetCurHp() <= 0 || a.Game.Run != nil && !a.Game.Run.Complete || a.Game.Field.Fight != nil && !a.Game.Field.Fight.Settled {
			return ggoFieldError(constUtil.GGOFieldBusy, "倒下或战斗中无法采集")
		}
		var active db.P8GatherAttempt
		e = tx.First(&active, "player_id=? AND active=?", a.Row.Id, true).Error
		if e != nil && !errors.Is(e, gorm.ErrRecordNotFound) {
			return e
		}
		if e == nil {
			return ggoFieldError(constUtil.GGOFieldBusy, "已有采集尚未结束")
		}
		cfg := a.Service.GGOFieldConfig
		values, err := ggoChipValues(cfg)
		if err != nil {
			return err
		}
		_, total, err := ggoProtectedChips(a.Game.GGOField.Chips, cfg.Runtime.ProtectedChips)
		if err != nil {
			return err
		}
		if total > 0 && (a.Game.GGOField.EconomyVersion != cfg.Runtime.EconomyVersion || a.Game.GGOField.ExchangeValues != values) {
			return ggoFieldError(constUtil.GGOFieldEconomyConflict, "请先返回安全区结算旧版本芯片")
		}
		row, state, _, err := ggoLoadLine(tx, a.Service.OriginalServerID(), int64(a.Presence.Line), cfg, a.Service.GGOFieldRevision, a.Now)
		if err != nil {
			return err
		}
		var node *ggoResourceNode
		for i := range state.Resources {
			if state.Resources[i].PointID == req.PointID {
				node = &state.Resources[i]
				break
			}
		}
		if node == nil || node.Generation != req.Generation {
			return ggoFieldError(constUtil.GGOFieldStaleGeneration, "资源已刷新")
		}
		if node.Remaining < 1 || node.ReservedBy != 0 || ggoLegacyBlocked(state, node.PointID) {
			return ggoFieldError(constUtil.GGOFieldBusy, "资源点已耗尽或被占用")
		}
		var point *model.AdminFieldGGOPoint
		for i := range cfg.Runtime.Points {
			if cfg.Runtime.Points[i].ID == node.PointID {
				point = &cfg.Runtime.Points[i]
				break
			}
		}
		if point == nil {
			return errors.New("沙漠资源点配置缺失")
		}
		frozen, err := ggoGatherConfig(a.Service, cfg, *node, *point, req.RequestID, int64(a.Presence.Line))
		if err != nil {
			return err
		}
		if !p8Near(a.Game.Position.GetCurrent(), &frozen.p8GatherConfig) {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请靠近资源点")
		}
		level, err := a.Service.p8LifeLevel(tx, a.Row.Id, node.Kind)
		if err != nil {
			return err
		}
		if err := ggoGatherRequirementError(node.Kind, node.Level, level, cfg.Runtime.Resource.Fatigue, a.Game.Field.Side.GetTiredness()); err != nil {
			return err
		}
		token, err := fieldToken()
		if err != nil {
			return err
		}
		rewards, err := a.Service.p8RollGather(&frozen.p8GatherConfig)
		if err != nil {
			return err
		}
		raw, err := json.Marshal(frozen)
		if err != nil {
			return err
		}
		if a.Now > math.MaxInt64-frozen.GatherMS {
			return errors.New("采集时间溢出")
		}
		if err = ggoInterruptExtractionLocked(tx, a.Row.Id, a.Game, a.Now); err != nil {
			return err
		}
		attempt := db.P8GatherAttempt{PlayerID: a.Row.Id, Token: token, Scene: constUtil.GGOFieldScene, PosID: node.PointID, Generation: node.Generation, StartedAt: a.Now, ReadyAt: a.Now + frozen.GatherMS, Active: true, Snapshot: string(raw), Rewards: rewards}
		node.ReservedBy = a.Row.Id
		node.AttemptToken = token
		node.ReadyAtMS = attempt.ReadyAt
		if err = ggoLineSave(tx, row, state, a.Now); err != nil {
			return err
		}
		if err = tx.Save(&attempt).Error; err != nil {
			return err
		}
		if total == 0 {
			a.Game.GGOField.EconomyVersion = cfg.Runtime.EconomyVersion
			a.Game.GGOField.ExchangeValues = values
		}
		if node.Kind == "fish" {
			a.Game.Field.Side.State = proto.Int32(constUtil.P8FieldFishing)
		} else {
			a.Game.Field.Side.State = proto.Int32(constUtil.P8FieldMining)
		}
		if err = saveProfile(tx, a.Row, a.Player, a.Game); err != nil {
			return err
		}
		out = model.GGOFieldGatherStartResponse{RequestID: req.RequestID, AttemptToken: token, PointID: node.PointID, Generation: node.Generation, AcceptedAtMS: a.Now, ReadyAtMS: attempt.ReadyAt, FatigueCost: int64(frozen.Fatigue)}
		if err = ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpGatherStart, hash, cfg.Runtime.EconomyVersion, nil, [6]int64{}, 0, a.Now, out); err != nil {
			return err
		}
		accepted = true
		return nil
	})
	if err == nil && accepted {
		s.ggoScheduleGather(ctx, req.PlayerID, out.AttemptToken, out.ReadyAtMS)
	}
	return &out, err
}
