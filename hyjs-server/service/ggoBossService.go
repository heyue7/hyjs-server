package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// Each line keeps its event in a separate boss row. Contribution and award
// rows retain their frozen policy after the live event is replaced.
type ggoBossClaimSessionKey struct{}

type ggoBossEvent struct {
	Source         string                       `json:"source"`
	ID             string                       `json:"id"`
	Line           int64                        `json:"line,omitempty"`
	PointID        string                       `json:"pointId"`
	Position       *model.AdminFieldGGOPosition `json:"position"`
	Radius         float64                      `json:"radius"`
	StartsAtMS     int64                        `json:"startsAtMs"`
	EndsAtMS       int64                        `json:"endsAtMs"`
	EconomyVersion string                       `json:"economyVersion"`
	ExchangeValues [6]int64                     `json:"exchangeValues"`
	Boss           model.AdminFieldGGOBoss      `json:"boss"`
}

type ggoBossAward struct {
	EconomyVersion string   `json:"economyVersion"`
	ExchangeValues [6]int64 `json:"exchangeValues"`
	Chips          [6]int64 `json:"chips"`
}

// The schedule is fixed to Beijing time. A live interval wins over the next
// warning window; an expired event never awards an uncompleted death.
func ggoBossWindow(cfg model.AdminFieldGGOBoss, now int64) (int64, int64, error) {
	if cfg.DayIntervalMinutes < 1 || cfg.EveningIntervalMinutes < 1 || cfg.EveningStartHour < 0 || cfg.EveningStartHour > 23 || cfg.ActiveSeconds < 1 || cfg.ActiveSeconds > min(cfg.DayIntervalMinutes, cfg.EveningIntervalMinutes)*60 {
		return 0, 0, errors.New("沙漠首领刷新窗口无效")
	}
	zone := time.FixedZone("UTC+8", 8*3600)
	local := time.UnixMilli(now).In(zone)
	day := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, zone)
	starts := make([]int64, 0, 150)
	for offset := -1; offset <= 1; offset++ {
		base := day.AddDate(0, 0, offset)
		split := cfg.EveningStartHour * 60
		for minute := int64(0); minute < split; minute += cfg.DayIntervalMinutes {
			starts = append(starts, base.Add(time.Duration(minute)*time.Minute).UnixMilli())
		}
		for minute := split; minute < 1440; minute += cfg.EveningIntervalMinutes {
			starts = append(starts, base.Add(time.Duration(minute)*time.Minute).UnixMilli())
		}
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i] < starts[j] })
	for _, start := range starts {
		end := start + cfg.ActiveSeconds*1000
		if now >= start && now < end {
			return start, end, nil
		}
		if start > now {
			return start, end, nil
		}
	}
	return 0, 0, errors.New("沙漠首领下一刷新不存在")
}

func ggoBossDecode(row *db.SharedFieldBoss55) (*ggoBossEvent, *pb.SideBoss, map[string]float64, error) {
	var event ggoBossEvent
	var boss pb.SideBoss
	hp := map[string]float64{}
	if json.Unmarshal(row.GGOEvent, &event) != nil || event.Source != constUtil.GGOFieldBossSource || event.ID != row.UUID || row.Line < 1 || (event.Line != row.Line && !(event.Line == 0 && row.Line == constUtil.GGOFieldBossStorageLine)) || event.StartsAtMS >= event.EndsAtMS || event.Boss.HP < 1 || json.Unmarshal(row.Payload, &boss) != nil || boss.GetUuid() != event.ID || json.Unmarshal(row.HP, &hp) != nil {
		return nil, nil, nil, errors.New("沙漠共享首领事件无效")
	}
	value, exists := hp[ggoBossGUID(event.ID)]
	if !exists || len(hp) != 1 || !combatFinite(value) || hp[ggoBossGUID(event.ID)] < 0 || hp[ggoBossGUID(event.ID)] > float64(event.Boss.HP) {
		return nil, nil, nil, errors.New("沙漠共享首领血量无效")
	}
	return &event, &boss, hp, nil
}
func ggoBossGUID(eventID string) string { return "ggo-boss-" + eventID }

func ggoBossEventID(serverID, line, start int64) string {
	return "ggo-" + p6Digest([]byte(fmt.Sprintf("%d:%d:%d", serverID, line, start)))[:40]
}

func ggoBossNeedsEvent(row *db.SharedFieldBoss55, start, end, now int64) (bool, error) {
	if row.UUID == "" {
		return true, nil
	}
	event, _, _, err := ggoBossDecode(row)
	if err != nil {
		return false, err
	}
	// An undefeated event keeps its health and frozen rewards until it is killed.
	if row.KillerID == 0 {
		return false, nil
	}
	due := row.RespawnAt
	if due <= 0 {
		due = event.EndsAtMS // compatibility with old rows
	}
	return now >= due, nil
}

// The optional line keeps older internal callers on the legacy line; live
// requests always pass their validated P10 presence line explicitly.
func (s *GameService) ggoBossCurrent(tx *gorm.DB, cfg *model.AdminFieldGGOConfiguration, now int64, write bool, lines ...int64) (*db.SharedFieldBoss55, *ggoBossEvent, *pb.SideBoss, map[string]float64, error) {
	if cfg == nil || cfg.Runtime == nil {
		return nil, nil, nil, nil, errors.New("沙漠首领配置缺失")
	}
	line := constUtil.GGOFieldBossStorageLine
	if len(lines) > 1 || len(lines) == 1 && (lines[0] < 1 || lines[0] > math.MaxInt32) {
		return nil, nil, nil, nil, errors.New("沙漠首领线路无效")
	}
	if len(lines) == 1 {
		line = lines[0]
	}
	start, end := now, constUtil.GGOFieldBossOpenUntilMS
	if now <= 0 || now >= end {
		return nil, nil, nil, nil, errors.New("沙漠首领时间无效")
	}
	query := func(q *gorm.DB) *gorm.DB {
		return sharedFieldBoss55Query(q, s.OriginalServerID(), constUtil.GGOFieldScene, constUtil.GGOFieldBossPosition, line)
	}
	var row db.SharedFieldBoss55
	err := query(tx).Take(&row).Error
	missing := errors.Is(err, gorm.ErrRecordNotFound)
	if err != nil && !missing {
		return nil, nil, nil, nil, err
	}
	needs := missing
	if !missing {
		needs, err = ggoBossNeedsEvent(&row, start, end, now)
		if err != nil {
			return nil, nil, nil, nil, err
		}
	}
	promote, migrateFormation := false, false
	if !missing && row.KillerID == 0 {
		old, _, hp, e := ggoBossDecode(&row)
		if e != nil {
			return nil, nil, nil, nil, e
		}
		promote = old.StartsAtMS > now || old.EndsAtMS != end
		// Keep the same event and earned damage when the published RE boss
		// replaces the earlier single-character model. Do not reduce a live
		// boss to zero if damage already exceeds the new maximum.
		damage := float64(old.Boss.HP) - hp[ggoBossGUID(old.ID)]
		migrateFormation = old.Boss.HeroID == "hero_017" && cfg.Runtime.Boss.HeroID == "monster_047" &&
			cfg.Runtime.Boss.HP > 0 && damage >= 0 && damage < float64(cfg.Runtime.Boss.HP)
	}
	if write || needs || promote || migrateFormation {
		if missing {
			row = db.SharedFieldBoss55{ServerID: s.OriginalServerID(), Scene: constUtil.GGOFieldScene, Line: line, PosID: constUtil.GGOFieldBossPosition, Payload: []byte("{}"), HP: []byte("{}")}
			if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return nil, nil, nil, nil, err
			}
		}
		if err = query(tx.Clauses(clause.Locking{Strength: "UPDATE"})).Take(&row).Error; err != nil {
			return nil, nil, nil, nil, err
		}
		needs, err = ggoBossNeedsEvent(&row, start, end, now)
		if err != nil {
			return nil, nil, nil, nil, err
		}
		if needs {
			var point *model.AdminFieldGGOPoint
			for i := range cfg.Runtime.Points {
				if cfg.Runtime.Points[i].Kind == "boss" {
					if point != nil {
						return nil, nil, nil, nil, errors.New("沙漠仅允许一只首领")
					}
					point = &cfg.Runtime.Points[i]
				}
			}
			if point == nil {
				return nil, nil, nil, nil, errors.New("沙漠首领点位缺失")
			}
			position, e := ggoWirePoint(point.Position)
			if e != nil {
				return nil, nil, nil, nil, e
			}
			values, e := ggoChipValues(cfg)
			if e != nil {
				return nil, nil, nil, nil, e
			}
			id := ggoBossEventID(s.OriginalServerID(), line, start)
			event := ggoBossEvent{Source: constUtil.GGOFieldBossSource, ID: id, Line: line, PointID: point.ID, Position: point.Position, Radius: point.Radius, StartsAtMS: start, EndsAtMS: end, EconomyVersion: cfg.Runtime.EconomyVersion, ExchangeValues: values, Boss: cfg.Runtime.Boss}
			boss := &pb.SideBoss{Uuid: proto.String(id), BossId: proto.String(constUtil.GGOFieldBossID), BossPosId: proto.String(point.ID), HeroId: proto.String(event.Boss.HeroID), CurrentPoint: position, State: proto.Int32(1), CurHp: proto.Float64(1), Nature: proto.Int32(0)}
			row.UUID = id
			row.RespawnAt = 0
			row.KillerID = 0
			row.DeathRules73 = nil
			row.GGOEvent, err = json.Marshal(event)
			if err != nil {
				return nil, nil, nil, nil, err
			}
			if err = saveSharedFieldBoss55(tx, &row, boss, map[string]float64{ggoBossGUID(id): float64(event.Boss.HP)}); err != nil {
				return nil, nil, nil, nil, err
			}
		} else if row.KillerID == 0 {
			// Promote the existing generation without discarding its health,
			// contribution or frozen rewards when changing the availability rule.
			event, boss, hp, e := ggoBossDecode(&row)
			if e != nil {
				return nil, nil, nil, nil, e
			}
			changed := false
			if event.StartsAtMS > now || event.EndsAtMS != end {
				if event.StartsAtMS > now {
					event.StartsAtMS = now
				}
				event.EndsAtMS = end
				changed = true
			}
			damage := float64(event.Boss.HP) - hp[ggoBossGUID(event.ID)]
			if event.Boss.HeroID == "hero_017" && cfg.Runtime.Boss.HeroID == "monster_047" &&
				cfg.Runtime.Boss.HP > 0 && damage >= 0 && damage < float64(cfg.Runtime.Boss.HP) {
				event.Boss.HeroID = cfg.Runtime.Boss.HeroID
				event.Boss.Level = cfg.Runtime.Boss.Level
				event.Boss.HP = cfg.Runtime.Boss.HP
				event.Boss.Attack = cfg.Runtime.Boss.Attack
				event.Boss.PhyDefend = cfg.Runtime.Boss.PhyDefend
				event.Boss.MagicDefend = cfg.Runtime.Boss.MagicDefend
				hp[ggoBossGUID(event.ID)] = float64(event.Boss.HP) - damage
				boss.HeroId = proto.String(event.Boss.HeroID)
				boss.CurHp = proto.Float64(hp[ggoBossGUID(event.ID)] / float64(event.Boss.HP))
				changed = true
			}
			if changed {
				row.GGOEvent, e = json.Marshal(event)
				if e != nil {
					return nil, nil, nil, nil, e
				}
				if e = saveSharedFieldBoss55(tx, &row, boss, hp); e != nil {
					return nil, nil, nil, nil, e
				}
			}
		}
	}
	event, boss, hp, err := ggoBossDecode(&row)
	return &row, event, boss, hp, err
}

func (s *GameService) ggoBossSnapshotLocked(tx *gorm.DB, cfg *model.AdminFieldGGOConfiguration, now, line int64) (*model.GGOFieldBossView, error) {
	row, event, boss, hp, err := s.ggoBossCurrent(tx, cfg, now, false, line)
	if err != nil {
		return nil, err
	}
	state := "active"
	starts, ends := event.StartsAtMS, event.EndsAtMS
	if row.KillerID > 0 {
		starts = row.RespawnAt
		if starts <= 0 {
			starts = event.EndsAtMS
		}
		ends = constUtil.GGOFieldBossOpenUntilMS
		state = "waiting"
	} else if now < event.StartsAtMS {
		state = "waiting"
	} else if now >= event.EndsAtMS {
		state = "expired"
	}
	return &model.GGOFieldBossView{Nature: int64(boss.GetNature()), Level: event.Boss.Level, WarningAtMS: starts - event.Boss.WarningSeconds*1000, HeroID: event.Boss.HeroID, BossID: constUtil.GGOFieldBossID, Radius: event.Radius, PointID: event.PointID, EventID: event.ID, Position: event.Position, HP: int64(math.Ceil(hp[ggoBossGUID(event.ID)])), MaxHP: event.Boss.HP, StartsAtMS: starts, EndsAtMS: ends, State: state}, nil
}
