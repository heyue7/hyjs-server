package service

import (
	"context"
	"crypto/subtle"
	"errors"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func ggoWirePoint(p *model.AdminFieldGGOPosition) (*pb.PointNew, error) {
	if p == nil || p.X == nil || p.Y == nil || p.Z == nil {
		return nil, errors.New("沙漠传送位置未配置")
	}
	for _, v := range []float64{*p.X, *p.Y, *p.Z} {
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 100000 {
			return nil, errors.New("沙漠传送位置无效")
		}
	}
	return &pb.PointNew{X: proto.Float32(float32(*p.X)), Y: proto.Float32(float32(*p.Y)), Z: proto.Float32(float32(*p.Z))}, nil
}

func ggoCurrentPresence(tx *gorm.DB, a *ggoFieldActor, session []byte) error {
	if a.Game.PositionLoading || a.Game.Position == nil || a.Game.Position.GetScene() != constUtil.GGOFieldScene || a.Game.Field == nil || a.Game.Field.Side == nil {
		return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请先进入沙漠安全区")
	}
	var presence db.P10ScenePresence
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&presence, "player_id=?", a.Row.Id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "分线尚未就绪")
		}
		return err
	}
	v, ok := p10ClientSinks.Load(p10ClientKey{a.Service.p10ClientOwner(), a.Row.Id})
	if !ok || presence.Scene != constUtil.GGOFieldScene || presence.Line < 1 || presence.SessionHash != p6Digest(session) || subtle.ConstantTimeCompare(v.(*p10ClientSink).session, session) != 1 {
		return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请连接当前游戏场景")
	}
	a.Presence = presence
	return nil
}

// GGOFieldEntry chooses a configured landing point once and persists protection,
// position and receipt under the actor lock. The client completes the original
// position handshake to obtain authoritative current position after retries.
func (s *GameService) GGOFieldEntry(ctx context.Context, req model.GGOFieldEntryRequest, session []byte) (*model.GGOFieldEntryResponse, error) {
	var out model.GGOFieldEntryResponse
	var tradePeer int64
	err := s.ggoWithProfile(ctx, req.GGOFieldAuth, session, false, false, func(tx *gorm.DB, a *ggoFieldActor) error {
		hash := p6Digest([]byte(constUtil.GGOFieldOpEntry))
		if found, err := ggoReadReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpEntry, hash, &out); err != nil || found {
			return err
		}
		cfg := a.Service.GGOFieldConfig
		if cfg == nil || cfg.Runtime == nil {
			return ggoFieldError(constUtil.GGOFieldUnavailable, "沙漠搜打撤暂未开放")
		}
		if err := ggoCheckClientBuildLocked(tx, a.Row.Id, session, cfg); err != nil {
			return err
		}
		if err := ggoCurrentPresence(tx, a, session); err != nil {
			return err
		}
		if !ggoPointInsideSafe(a.Game.Position.GetCurrent(), cfg.Runtime.SafeArea) {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请在安全区找向导出发")
		}
		var guide *model.AdminFieldGGONPC
		for i := range cfg.NPCs {
			if cfg.NPCs[i].Role == "teleport" {
				guide = &cfg.NPCs[i]
				break
			}
		}
		if guide == nil {
			return errors.New("沙漠向导未配置")
		}
		guidePoint, err := ggoWirePoint(guide.Position)
		if err != nil {
			return err
		}
		if !p8Near(a.Game.Position.GetCurrent(), &p8GatherConfig{Point: guidePoint, Radius: 3}) {
			return ggoFieldError(constUtil.GGOFieldInvalidLocation, "请靠近沙漠向导")
		}
		if a.Game.GGOField == nil || a.Game.Field.Side.GetCurHp() <= 0 || a.Game.Field.Fight != nil && !a.Game.Field.Fight.Settled || a.Game.Run != nil && !a.Game.Run.Complete {
			return ggoFieldError(constUtil.GGOFieldBusy, "当前无法出发")
		}
		if err = P12FieldPVPRequireIdle(tx, a.Row.Id, time.UnixMilli(a.Now)); err != nil {
			return err
		}
		var active db.P8GatherAttempt
		if err = tx.Where("player_id=? AND active=?", a.Row.Id, true).First(&active).Error; err == nil {
			return ggoFieldError(constUtil.GGOFieldBusy, "当前无法出发")
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		values, err := ggoChipValues(cfg)
		if err != nil {
			return err
		}
		_, total, err := ggoProtectedChips(a.Game.GGOField.Chips, cfg.Runtime.ProtectedChips)
		if err != nil {
			return err
		}
		if total > 0 && (a.Game.GGOField.EconomyVersion != cfg.Runtime.EconomyVersion || a.Game.GGOField.ExchangeValues != values) {
			return ggoFieldError(constUtil.GGOFieldEconomyConflict, "请先结算已有版本芯片")
		}
		if total == 0 {
			a.Game.GGOField.EconomyVersion = cfg.Runtime.EconomyVersion
			a.Game.GGOField.ExchangeValues = values
		}
		if len(cfg.Runtime.EntryPoints) == 0 || cfg.Rules.ProtectionSeconds < 0 || cfg.Rules.ProtectionSeconds > 300 {
			return errors.New("沙漠出发配置无效")
		}
		n, err := gemRoll(int64(len(cfg.Runtime.EntryPoints)))
		if err != nil {
			return err
		}
		selected := cfg.Runtime.EntryPoints[n]
		point, err := ggoWirePoint(selected)
		if err != nil {
			return err
		}
		tradePeer = ggoCancelTradeForActor(a.Row.Id, a.Game)
		if err = ggoPositionBump(a.Game); err != nil {
			return err
		}
		a.Game.Position.Current = point
		a.Game.PositionLoading = true
		a.Game.GGOField.ProtectionUntil = a.Now + cfg.Rules.ProtectionSeconds*1000
		out = model.GGOFieldEntryResponse{RequestID: req.RequestID, AcceptedAtMS: a.Now, ProtectionUntilMS: a.Game.GGOField.ProtectionUntil, Position: selected, PositionRevision: a.Game.GGOField.PositionRevision}
		if err = saveProfile(tx, a.Row, a.Player, a.Game); err != nil {
			return err
		}
		return ggoSaveReceipt(tx, a.Row.Id, req.RequestID, constUtil.GGOFieldOpEntry, hash, cfg.Runtime.EconomyVersion, nil, [6]int64{}, 0, a.Now, out)
	})
	if err != nil {
		return nil, err
	}
	if tradePeer > 0 {
		ggoTradeNotify(s, ctx, tradePeer)
	}
	return &out, nil
}
