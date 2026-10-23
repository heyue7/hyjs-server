package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// InitializeSharedFieldBoss55 imports only still-live old death deadlines, once per realm.
// It never writes a profile or replays a historical reward or notice.
func (s *GameService) InitializeSharedFieldBoss55(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.SharedFieldBoss55{}); err != nil {
		return err
	}
	var marker db.SharedFieldBoss55
	err := tx.Where("server_id=? AND scene=? AND pos_id=?", s.OriginalServerID(), "", "").First(&marker).Error
	if err == nil {
		s.SharedFieldBoss55Ready = true
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	winners := map[string]db.SharedFieldBoss55{}
	now := time.Now().UnixMilli()
	last := int64(0)
	for {
		var rows []db.Profile
		if err := tx.Where("server_id=? AND id>?", s.OriginalServerID(), last).Order("id").Limit(100).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		for _, row := range rows {
			last = row.Id
			if row.Game == nil {
				continue
			}
			var g model.GameState
			if json.Unmarshal([]byte(*row.Game), &g) != nil || g.Field == nil {
				continue
			}
			snapshots := map[string][]*model.FieldState{}
			snapshots[g.Field.Scene] = append(snapshots[g.Field.Scene], g.Field)
			for areaScene, area := range g.Field.Areas {
				if area != nil {
					snapshots[areaScene] = append(snapshots[areaScene], &model.FieldState{Scene: areaScene, Bosses: area.Bosses, RespawnAt: area.RespawnAt, BossHP: area.BossHP})
				}
			}
			for _, versions := range snapshots {
				for _, f := range versions {
					scene := s.fieldSceneRow(f.Scene)
					if scene == nil {
						continue
					}
					positions := map[string]bool{}
					for _, pos := range strings.Split(scene["bossPosIds"], ";") {
						positions[pos] = true
					}
					for _, b := range f.Bosses {
						due := f.RespawnAt[b.GetUuid()]
						if b.GetState() != 3 || due <= now || !positions[b.GetBossPosId()] || s.fieldBossLeaderInScene(b.GetBossId(), f.Scene) == nil {
							continue
						}
						key := f.Scene + ":" + b.GetBossPosId()
						if old, ok := winners[key]; ok && (old.RespawnAt > due || old.RespawnAt == due && old.UUID <= b.GetUuid()) {
							continue
						}
						payload, e := json.Marshal(b)
						if e != nil {
							return e
						}
						hp, _ := json.Marshal(f.BossHP[b.GetUuid()])
						winners[key] = db.SharedFieldBoss55{ServerID: s.OriginalServerID(), Scene: f.Scene, PosID: b.GetBossPosId(), UUID: b.GetUuid(), Payload: payload, HP: hp, RespawnAt: due}
					}
				}
			}
		}
	}
	err = tx.Transaction(func(tx *gorm.DB) error {
		keys := []string{}
		for key := range winners {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			row := winners[key]
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; err != nil {
				return err
			}
		}
		marker = db.SharedFieldBoss55{ServerID: s.OriginalServerID(), UUID: "migration55", Payload: []byte("{}"), HP: []byte("{}")}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&marker).Error; err != nil {
			return err
		}
		return nil
	})
	if err == nil {
		s.SharedFieldBoss55Ready = true
	}
	return err
}

func sharedFieldBoss55Query(tx *gorm.DB, sid int64, scene, pos string) *gorm.DB {
	return tx.Where("server_id=? AND scene=? AND pos_id=?", sid, scene, pos)
}
func (s *GameService) sharedFieldBoss55Locked(tx *gorm.DB, scene, pos string, now int64) (*db.SharedFieldBoss55, *pb.SideBoss, map[string]float64, error) {
	var row db.SharedFieldBoss55
	err := sharedFieldBoss55Query(tx.Clauses(clause.Locking{Strength: "UPDATE"}), s.OriginalServerID(), scene, pos).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		b, e := s.fieldBossSpawn(pos)
		if e != nil {
			return nil, nil, nil, e
		}
		raw, e := json.Marshal(b)
		if e != nil {
			return nil, nil, nil, e
		}
		row = db.SharedFieldBoss55{ServerID: s.OriginalServerID(), Scene: scene, PosID: pos, UUID: b.GetUuid(), Payload: raw, HP: []byte("{}")}
		if e := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
			return nil, nil, nil, e
		}
		if e := sharedFieldBoss55Query(tx.Clauses(clause.Locking{Strength: "UPDATE"}), s.OriginalServerID(), scene, pos).First(&row).Error; e != nil {
			return nil, nil, nil, e
		}
	} else if err != nil {
		return nil, nil, nil, err
	}
	var boss pb.SideBoss
	hp := map[string]float64{}
	if err := json.Unmarshal(row.Payload, &boss); err != nil {
		return nil, nil, nil, err
	}
	if err := json.Unmarshal(row.HP, &hp); err != nil {
		return nil, nil, nil, err
	}
	if row.RespawnAt > 0 && row.RespawnAt <= now {
		fresh, e := s.fieldBossSpawn(pos)
		if e != nil {
			return nil, nil, nil, e
		}
		proto.Reset(&boss)
		proto.Merge(&boss, fresh)
		row.UUID = fresh.GetUuid()
		row.RespawnAt = 0
		row.KillerID = 0
		hp = map[string]float64{}
		if err := saveSharedFieldBoss55(tx, &row, &boss, hp); err != nil {
			return nil, nil, nil, err
		}
	}
	return &row, &boss, hp, nil
}
func saveSharedFieldBoss55(tx *gorm.DB, row *db.SharedFieldBoss55, boss *pb.SideBoss, hp map[string]float64) error {
	raw, err := json.Marshal(boss)
	if err != nil {
		return err
	}
	h, err := json.Marshal(hp)
	if err != nil {
		return err
	}
	row.Payload = raw
	row.HP = h
	return tx.Save(row).Error
}

// refreshSharedFieldBosses55 runs inside the caller's profile transaction.
// Accepted fights retain their frozen personal projection until their receipt commits.
func (s *GameService) refreshSharedFieldBosses55(tx *gorm.DB, g *model.GameState) error {
	if !s.SharedFieldBoss55Ready || g.Field == nil {
		return nil
	}
	f := g.Field
	scene := s.fieldSceneRow(f.Scene)
	if scene == nil || scene["adminOrdinaryBossEnabled"] == "false" {
		return nil
	}
	if f.RespawnAt == nil {
		f.RespawnAt = map[string]int64{}
	}
	if f.BossHP == nil {
		f.BossHP = map[string]map[string]float64{}
	}
	positions := strings.Split(scene["bossPosIds"], ";")
	sort.Strings(positions)
	var projected []*pb.SideBoss
	for _, pos := range positions {
		if pos == "" {
			continue
		}
		if f.Fight != nil && !f.Fight.Settled && f.Fight.BossStart != nil {
			old := fieldBossFind(f, f.Fight.BossUUID)
			if old != nil && old.GetBossPosId() == pos {
				projected = append(projected, old)
				continue
			}
		}
		row, boss, hp, err := s.sharedFieldBoss55Locked(tx, f.Scene, pos, time.Now().UnixMilli())
		if err != nil {
			return err
		}
		projected = append(projected, boss)
		f.RespawnAt[boss.GetUuid()] = row.RespawnAt
		f.BossHP[boss.GetUuid()] = hp
	}
	// Drop only obsolete boss references. RespawnAt also owns monster timers.
	keep := map[string]bool{}
	for _, boss := range projected {
		keep[boss.GetUuid()] = true
	}
	if f.Fight != nil && f.Fight.BossUUID != "" {
		keep[f.Fight.BossUUID] = true
	}
	for _, area := range f.Areas {
		if area != nil && area.Fight != nil && area.Fight.BossUUID != "" {
			keep[area.Fight.BossUUID] = true
		}
	}
	for _, old := range f.Bosses {
		if !keep[old.GetUuid()] {
			delete(f.BossHP, old.GetUuid())
			delete(f.RespawnAt, old.GetUuid())
		}
	}
	for uuid := range f.BossHP {
		if !keep[uuid] {
			delete(f.BossHP, uuid)
			delete(f.RespawnAt, uuid)
		}
	}
	f.Bosses = projected
	return nil
}

// applySharedFieldBoss55 subtracts only the authoritative frozen battle damage.
// A previous generation and a repeated terminal can never kill the current spawn.
func (s *GameService) applySharedFieldBoss55(tx *gorm.DB, id int64, f *model.FieldState, boss *pb.SideBoss, input *model.FrozenBattleInput, final map[string]float64, cooldown int64) (bool, bool, error) {
	if !s.SharedFieldBoss55Ready {
		return false, false, nil
	}
	var row db.SharedFieldBoss55
	err := sharedFieldBoss55Query(tx.Clauses(clause.Locking{Strength: "UPDATE"}), s.OriginalServerID(), f.Scene, boss.GetBossPosId()).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if row.UUID != boss.GetUuid() {
		return false, false, nil
	}
	var shared pb.SideBoss
	hp := map[string]float64{}
	if err := json.Unmarshal(row.Payload, &shared); err != nil {
		return false, false, err
	}
	if err := json.Unmarshal(row.HP, &hp); err != nil {
		return false, false, err
	}
	if hp == nil {
		hp = map[string]float64{}
	}
	wasDead := row.RespawnAt > 0
	allDead := true
	for _, actor := range input.Monster.GetFormation() {
		guid := actor.GetGuid()
		end, ok := final[guid]
		if !ok {
			return false, false, errors.New("共享首领终局成员缺失")
		}
		start := actor.GetInst().GetCurHp()
		current, ok := hp[guid]
		if !ok {
			current = actor.GetProp().GetHp()
		}
		damage := math.Max(0, start-end)
		if !combatFinite(damage) || !combatFinite(current) {
			return false, false, errors.New("共享首领血量无效")
		}
		current = math.Max(0, current-damage)
		hp[guid] = current
		if current > 0 {
			allDead = false
		}
		if guid == f.Fight.BossLeaderGUID {
			shared.CurHp = proto.Float64(current / actor.GetProp().GetHp())
		}
	}
	killed := allDead && !wasDead
	if killed {
		if cooldown <= 0 {
			return false, false, errors.New("共享首领刷新时间无效")
		}
		row.RespawnAt = time.Now().UnixMilli() + cooldown
		row.KillerID = id
		shared.State = proto.Int32(3)
	} else if !wasDead {
		shared.State = proto.Int32(1)
	}
	if err := saveSharedFieldBoss55(tx, &row, &shared, hp); err != nil {
		return false, false, err
	}
	proto.Reset(boss)
	proto.Merge(boss, &shared)
	f.BossHP[boss.GetUuid()] = hp
	f.RespawnAt[boss.GetUuid()] = row.RespawnAt
	return true, killed, nil
}
