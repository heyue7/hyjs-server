package service

import (
	"encoding/json"
	"errors"
	"math"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/db"
	"hyjs-server/model"
)

// One row owns the bounded public nodes and their short reservations per realm line.
type ggoLineState struct {
	DeploymentRevision string            `json:"deploymentRevision"`
	NextGeneration     int64             `json:"nextGeneration"`
	CycleMS            int64             `json:"cycleMs"`
	Resources          []ggoResourceNode `json:"resources"`
	Monsters           []ggoMonsterNode  `json:"monsters"`
	LegacyBlocks       []ggoLegacyBlock  `json:"legacyBlocks,omitempty"`
}

// A published revision can replace visible nodes while earlier accepted
// actions retain their independently frozen results. At most one prior action
// blocks any point present in the new version.
type ggoLegacyBlock struct {
	PointID    string `json:"pointId"`
	Kind       string `json:"kind"`
	PlayerID   int64  `json:"playerId"`
	Token      string `json:"token"`
	Generation int64  `json:"generation"`
}

func ggoLegacyBlocked(state *ggoLineState, pointID string) bool {
	for _, b := range state.LegacyBlocks {
		if b.PointID == pointID {
			return true
		}
	}
	return false
}
func ggoReleaseLegacyBlock(state *ggoLineState, pointID, kind string, playerID int64, token string, generation int64) bool {
	for i, b := range state.LegacyBlocks {
		if b.PointID == pointID && b.Kind == kind && b.PlayerID == playerID && b.Token == token && b.Generation == generation {
			state.LegacyBlocks = append(state.LegacyBlocks[:i], state.LegacyBlocks[i+1:]...)
			return true
		}
	}
	return false
}

type ggoResourceNode struct {
	PointID       string `json:"pointId"`
	Kind          string `json:"kind"`
	Level         int64  `json:"level"`
	Remaining     int64  `json:"remaining"`
	Generation    int64  `json:"generation"`
	ScheduledAtMS int64  `json:"scheduledAtMs"`
	RefreshAtMS   int64  `json:"refreshAtMs"`
	ReservedBy    int64  `json:"reservedBy,omitempty"`
	AttemptToken  string `json:"attemptToken,omitempty"`
	ReadyAtMS     int64  `json:"readyAtMs,omitempty"`
}
type ggoMonsterNode struct {
	PointID          string `json:"pointId"`
	Level            int64  `json:"level"`
	HeroID           string `json:"heroId"`
	MonID            string `json:"monId"`
	UUID             string `json:"uuid"`
	Generation       int64  `json:"generation"`
	RefreshAtMS      int64  `json:"refreshAtMs"`
	ReservedBy       int64  `json:"reservedBy,omitempty"`
	FightID          string `json:"fightId,omitempty"`
	ReservedUntilMS  int64  `json:"reservedUntilMs,omitempty"`
	PatrolEpochMS    int64  `json:"patrolEpochMs,omitempty"`
	PatrolPausedAtMS int64  `json:"patrolPausedAtMs,omitempty"`
}

// Each id is an original FieldMonster group with the intended displayed
// tier; SideMonster.heroId overrides its scene model in the native client.
func ggoMonsterGroup(heroID string) string {
	switch heroID {
	case "monster_026":
		return "mon_021"
	case "monster_032":
		return "mon_031"
	case "monster_031":
		return "mon_041"
	default:
		return ""
	}
}

// ggoCycleBoundary is anchored to Beijing wall time, never the process start time.
func ggoCycleBoundary(nowMS, seconds int64) int64 {
	cycle := seconds * 1000
	const chinaOffsetMS int64 = 8 * 60 * 60 * 1000
	return (nowMS+chinaOffsetMS)/cycle*cycle - chinaOffsetMS
}

func ggoRollResourceTier(tiers []model.AdminFieldGGOResourceTier) (int64, error) {
	var total int64
	for _, tier := range tiers {
		if tier.Weight < 0 || tier.Weight > math.MaxInt64-total {
			return 0, errors.New("沙漠资源权重无效")
		}
		total += tier.Weight
	}
	if total <= 0 {
		return 0, errors.New("沙漠资源权重为空")
	}
	n, err := gemRoll(total)
	if err != nil {
		return 0, err
	}
	for _, tier := range tiers {
		if n < tier.Weight {
			return tier.Level, nil
		}
		n -= tier.Weight
	}
	return 0, errors.New("沙漠资源权重无效")
}

func ggoNewLineState(cfg *model.AdminFieldGGOConfiguration, revision string, now int64) (*ggoLineState, error) {
	if cfg == nil || cfg.Runtime == nil || revision == "" || !model.ValidAdminFieldGGORuntime(cfg) {
		return nil, errors.New("沙漠运行配置无效")
	}
	state := &ggoLineState{DeploymentRevision: revision, NextGeneration: 1, CycleMS: cfg.Runtime.Resource.CycleSeconds * 1000, Resources: []ggoResourceNode{}, Monsters: []ggoMonsterNode{}}
	boundary := ggoCycleBoundary(now, cfg.Runtime.Resource.CycleSeconds)
	tiers := map[int64]string{}
	for _, tier := range cfg.Runtime.MonsterTiers {
		tiers[tier.Level] = tier.HeroID
	}
	for _, point := range cfg.Runtime.Points {
		switch point.Kind {
		case "mine", "fish":
			level, err := ggoRollResourceTier(cfg.Runtime.Resource.Tiers)
			if err != nil {
				return nil, err
			}
			generation, err := ggoNextGeneration(state)
			if err != nil {
				return nil, err
			}
			state.Resources = append(state.Resources, ggoResourceNode{PointID: point.ID, Kind: point.Kind, Level: level, Remaining: cfg.Runtime.Resource.Capacity, Generation: generation, ScheduledAtMS: boundary})
		case "monster":
			token, err := fieldToken()
			if err != nil {
				return nil, err
			}
			generation, err := ggoNextGeneration(state)
			if err != nil {
				return nil, err
			}
			state.Monsters = append(state.Monsters, ggoMonsterNode{PointID: point.ID, Level: point.Level, HeroID: tiers[point.Level], MonID: ggoMonsterGroup(tiers[point.Level]), UUID: token, Generation: generation, PatrolEpochMS: now})
		}
	}
	sort.Slice(state.Resources, func(i, j int) bool { return state.Resources[i].PointID < state.Resources[j].PointID })
	sort.Slice(state.Monsters, func(i, j int) bool { return state.Monsters[i].PointID < state.Monsters[j].PointID })
	return state, nil
}

func ggoNextGeneration(state *ggoLineState) (int64, error) {
	if state.NextGeneration < 1 || state.NextGeneration == math.MaxInt64 {
		return 0, errors.New("沙漠场景代次溢出")
	}
	value := state.NextGeneration
	state.NextGeneration++
	return value, nil
}

// The persisted deadline makes restart recovery an indexed bounded query.
// Zero schedules one migration pass for snapshots written before this field.
func ggoNextLineDue(state *ggoLineState) int64 {
	if state == nil || state.CycleMS <= 0 {
		return 0
	}
	due := int64(math.MaxInt64)
	add := func(at int64) {
		if at > 0 && at < due {
			due = at
		}
	}
	for _, node := range state.Resources {
		if node.ReservedBy != 0 {
			continue
		}
		if node.ScheduledAtMS > math.MaxInt64-state.CycleMS {
			return 0
		}
		add(node.ScheduledAtMS + state.CycleMS)
		if node.Remaining == 0 {
			add(node.RefreshAtMS)
		}
	}
	for _, node := range state.Monsters {
		if node.ReservedBy == 0 {
			add(node.RefreshAtMS)
		} else {
			add(node.ReservedUntilMS)
		}
	}
	return due
}

func ggoLineSave(tx *gorm.DB, row *db.GGOFieldLine, state *ggoLineState, now int64) error {
	if row.Revision < 0 || row.Revision == math.MaxInt64 {
		return errors.New("沙漠场景版本无效")
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return err
	}
	row.Revision++
	row.Payload = payload
	row.UpdatedAt = now
	row.NextDueAt = ggoNextLineDue(state)
	return tx.Model(&db.GGOFieldLine{}).Where("server_id=? AND line=?", row.ServerID, row.Line).Updates(map[string]any{"revision": row.Revision, "payload": row.Payload, "updated_at": row.UpdatedAt, "next_due_at": row.NextDueAt}).Error
}

// ggoLoadLine obtains a single line row lock before changing a node, so a
// resource reservation, generation change and reward commit share a transaction.
func ggoLoadLine(tx *gorm.DB, serverID, line int64, cfg *model.AdminFieldGGOConfiguration, revision string, now int64) (*db.GGOFieldLine, *ggoLineState, bool, error) {
	if serverID < 1 || line < 1 {
		return nil, nil, false, errors.New("沙漠区服或分线无效")
	}
	var row db.GGOFieldLine
	query := func() *gorm.DB { return tx.Where("server_id=? AND line=?", serverID, line) }
	err := query().Clauses(clause.Locking{Strength: "UPDATE"}).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		initial, e := ggoNewLineState(cfg, revision, now)
		if e != nil {
			return nil, nil, false, e
		}
		payload, e := json.Marshal(initial)
		if e != nil {
			return nil, nil, false, e
		}
		row = db.GGOFieldLine{ServerID: serverID, Line: line, Revision: 1, Payload: payload, UpdatedAt: now, NextDueAt: ggoNextLineDue(initial)}
		if e = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error; e != nil {
			return nil, nil, false, e
		}
		if e = query().Clauses(clause.Locking{Strength: "UPDATE"}).First(&row).Error; e != nil {
			return nil, nil, false, e
		}
	} else if err != nil {
		return nil, nil, false, err
	}
	var state ggoLineState
	if len(row.Payload) > 262144 || json.Unmarshal(row.Payload, &state) != nil || row.Revision < 1 || state.NextGeneration < 1 {
		return nil, nil, false, errors.New("沙漠场景状态损坏")
	}
	// Only migrate bounded pre-release test reservations. Current V4 in-flight
	// battles have no wall-clock expiry and stay bound to their generation.
	expiredMonster := false
	for i := range state.Monsters {
		n := &state.Monsters[i]
		if n.ReservedBy > 0 && n.ReservedUntilMS > 0 && n.ReservedUntilMS <= now {
			token, e := fieldToken()
			if e != nil {
				return nil, nil, false, e
			}
			n.Generation, e = ggoNextGeneration(&state)
			if e != nil {
				return nil, nil, false, e
			}
			n.UUID = token
			n.ReservedBy = 0
			n.FightID = ""
			n.ReservedUntilMS = 0
			n.PatrolEpochMS, n.PatrolPausedAtMS = now, 0
			expiredMonster = true
		}
	}
	if expiredMonster {
		if e := ggoLineSave(tx, &row, &state, now); e != nil {
			return nil, nil, false, e
		}
	}
	if state.DeploymentRevision != revision {
		next, e := ggoNewLineState(cfg, revision, now)
		if e != nil {
			return nil, nil, false, e
		}
		currentPoints := map[string]bool{}
		for _, point := range cfg.Runtime.Points {
			currentPoints[point.ID] = true
		}
		addBlock := func(b ggoLegacyBlock) {
			if currentPoints[b.PointID] && !ggoLegacyBlocked(next, b.PointID) {
				next.LegacyBlocks = append(next.LegacyBlocks, b)
			}
		}
		for _, b := range state.LegacyBlocks {
			addBlock(b)
		}
		for _, n := range state.Resources {
			if n.ReservedBy > 0 {
				addBlock(ggoLegacyBlock{PointID: n.PointID, Kind: "gather", PlayerID: n.ReservedBy, Token: n.AttemptToken, Generation: n.Generation})
			}
		}
		for _, n := range state.Monsters {
			if n.ReservedBy > 0 {
				addBlock(ggoLegacyBlock{PointID: n.PointID, Kind: "monster", PlayerID: n.ReservedBy, Token: n.FightID, Generation: n.Generation})
			}
		}
		next.NextGeneration = state.NextGeneration
		for i := range next.Resources {
			next.Resources[i].Generation, e = ggoNextGeneration(next)
			if e != nil {
				return nil, nil, false, e
			}
		}
		for i := range next.Monsters {
			next.Monsters[i].Generation, e = ggoNextGeneration(next)
			if e != nil {
				return nil, nil, false, e
			}
		}
		if e = ggoValidateLineState(next, cfg); e != nil {
			return nil, nil, false, e
		}
		if e = ggoLineSave(tx, &row, next, now); e != nil {
			return nil, nil, false, e
		}
		return &row, next, true, nil
	}
	if err := ggoValidateLineState(&state, cfg); err != nil {
		return nil, nil, false, err
	}
	updated := false
	if state.CycleMS == 0 {
		state.CycleMS = cfg.Runtime.Resource.CycleSeconds * 1000
		updated = true
	} else if state.CycleMS != cfg.Runtime.Resource.CycleSeconds*1000 {
		return nil, nil, false, errors.New("沙漠资源刷新周期与发布版本不匹配")
	}
	for i := range state.Monsters {
		n := &state.Monsters[i]
		if n.PatrolEpochMS == 0 {
			n.PatrolEpochMS = now
			if n.ReservedBy > 0 || n.RefreshAtMS > 0 {
				n.PatrolPausedAtMS = now
			}
			updated = true
		}
	}
	if updated {
		if err := ggoLineSave(tx, &row, &state, now); err != nil {
			return nil, nil, false, err
		}
	}
	boundary := ggoCycleBoundary(now, cfg.Runtime.Resource.CycleSeconds)
	changed := false
	for i := range state.Resources {
		node := &state.Resources[i]
		if node.ReservedBy != 0 {
			continue
		}
		if node.ScheduledAtMS < boundary || node.Remaining == 0 && node.RefreshAtMS > 0 && node.RefreshAtMS <= now {
			level, e := ggoRollResourceTier(cfg.Runtime.Resource.Tiers)
			if e != nil {
				return nil, nil, false, e
			}
			node.Level = level
			node.Generation, e = ggoNextGeneration(&state)
			if e != nil {
				return nil, nil, false, e
			}
			node.Remaining = cfg.Runtime.Resource.Capacity
			node.ScheduledAtMS = boundary
			node.RefreshAtMS = 0
			changed = true
		}
	}
	for i := range state.Monsters {
		monster := &state.Monsters[i]
		if monster.ReservedBy == 0 && monster.RefreshAtMS > 0 && monster.RefreshAtMS <= now {
			token, e := fieldToken()
			if e != nil {
				return nil, nil, false, e
			}
			monster.Generation, e = ggoNextGeneration(&state)
			if e != nil {
				return nil, nil, false, e
			}
			monster.UUID = token
			monster.RefreshAtMS = 0
			monster.PatrolEpochMS, monster.PatrolPausedAtMS = now, 0
			changed = true
		}
	}
	if changed {
		if err := ggoLineSave(tx, &row, &state, now); err != nil {
			return nil, nil, false, err
		}
	}
	return &row, &state, changed, nil
}

func ggoValidateLineState(state *ggoLineState, cfg *model.AdminFieldGGOConfiguration) error {
	if state == nil || cfg == nil || cfg.Runtime == nil || len(state.Resources) != int(cfg.Runtime.Resource.MineCount+cfg.Runtime.Resource.FishCount) {
		return errors.New("沙漠资源状态数量无效")
	}
	points := map[string]model.AdminFieldGGOPoint{}
	for _, point := range cfg.Runtime.Points {
		points[point.ID] = point
	}
	blocks := map[string]bool{}
	if len(state.LegacyBlocks) > len(points) {
		return errors.New("沙漠旧版占用数量无效")
	}
	for _, b := range state.LegacyBlocks {
		if _, exists := points[b.PointID]; !exists || blocks[b.PointID] || (b.Kind != "gather" && b.Kind != "monster") || b.PlayerID < 1 || b.Token == "" || b.Generation < 1 || b.Generation >= state.NextGeneration {
			return errors.New("沙漠旧版占用状态无效")
		}
		blocks[b.PointID] = true
	}
	seen := map[string]bool{}
	for _, node := range state.Resources {
		point, ok := points[node.PointID]
		if !ok || seen[node.PointID] || point.Kind != node.Kind || (node.Kind != "mine" && node.Kind != "fish") ||
			node.Level < 3 || node.Level > 6 || node.Remaining < 0 || node.Remaining > cfg.Runtime.Resource.Capacity ||
			node.Generation < 1 || node.Generation >= state.NextGeneration || node.RefreshAtMS < 0 || node.ReservedBy < 0 ||
			node.ReservedBy == 0 && (node.AttemptToken != "" || node.ReadyAtMS != 0) ||
			node.ReservedBy > 0 && (node.AttemptToken == "" || node.ReadyAtMS < 1 || node.Remaining < 1) {
			return errors.New("沙漠资源状态无效")
		}
		seen[node.PointID] = true
	}
	var total int64
	for _, tier := range cfg.Runtime.MonsterTiers {
		total += tier.Count
	}
	if int64(len(state.Monsters)) != total {
		return errors.New("沙漠小怪状态数量无效")
	}
	for _, monster := range state.Monsters {
		point, ok := points[monster.PointID]
		if !ok || monster.MonID != ggoMonsterGroup(monster.HeroID) || seen[monster.PointID] || point.Kind != "monster" || point.Level != monster.Level || monster.HeroID == "" ||
			monster.UUID == "" || monster.Generation < 1 || monster.Generation >= state.NextGeneration ||
			monster.ReservedBy < 0 || monster.RefreshAtMS < 0 || monster.PatrolEpochMS < 0 || monster.PatrolPausedAtMS < 0 ||
			monster.ReservedBy == 0 && (monster.FightID != "" || monster.ReservedUntilMS != 0) ||
			monster.ReservedBy > 0 && (monster.FightID == "" || monster.ReservedUntilMS < 0 || monster.RefreshAtMS != 0) {
			return errors.New("沙漠小怪状态无效")
		}
		seen[monster.PointID] = true
	}
	return nil
}
