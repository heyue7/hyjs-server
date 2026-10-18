package service

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"strconv"
	"strings"
	"time"
)

// ObserveP5Collection is an optional save hook: old databases retain their migration scope.
// It records real ownership, never a client-supplied handbook ID; removal preserves history.
func (s *GameService) ObserveP5Collection(tx *gorm.DB, id int64, g *model.GameState) error {
	if !tx.Migrator().HasTable(&db.P5Collection{}) {
		return nil
	}
	// Preserve the first original Hero row's existing eligibility rule. A
	// lookup avoids scanning the complete Hero table for every experience card.
	eligibility := map[string]bool{}
	for _, row := range s.Tables["Hero"] {
		if _, exists := eligibility[row["id"]]; !exists {
			eligibility[row["id"]] = row != nil && row["heroClass"] != "99"
		}
	}
	seen := map[string]bool{}
	var ids []string
	for _, w := range g.Warriors {
		if !eligibility[w.GetId()] || seen[w.GetId()] {
			continue
		}
		seen[w.GetId()] = true
		ids = append(ids, w.GetId())
	}
	if len(ids) == 0 {
		return nil
	}
	// The profile transaction already owns the player row lock. Read historical
	// membership once and insert only new legitimate IDs without touching dates.
	var existing []string
	if err := tx.Model(&db.P5Collection{}).Where("player_id=? AND hero_id IN ?", id, ids).Pluck("hero_id", &existing).Error; err != nil {
		return err
	}
	for _, key := range existing {
		delete(seen, key)
	}
	var entries []db.P5Collection
	now := time.Now().UnixMilli()
	for _, key := range ids {
		if seen[key] {
			entries = append(entries, db.P5Collection{PlayerID: id, HeroID: key, AcquiredAt: now})
		}
	}
	if len(entries) > 0 {
		return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&entries).Error
	}
	return nil
}
func (s *GameService) p5Handbook(ctx context.Context, id int64) ([]proto.Message, error) {
	out := &pb.SCWarriorHandbook{}
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, _ *db.Profile, p *pb.Player, g *model.GameState) error {
		if err := s.ObserveP5Collection(tx, id, g); err != nil {
			return err
		}
		var rows []db.P5Collection
		if err := tx.Where("player_id=?", id).Order("hero_id").Find(&rows).Error; err != nil {
			return err
		}
		for _, r := range rows {
			out.IdList = append(out.IdList, r.HeroID)
		}
		return nil
	})
	return []proto.Message{out}, err
}

func (s *GameService) p5AchievementValue(tx *gorm.DB, p *pb.Player, g *model.GameState, r *db.P5Achievement) (int32, error) {
	switch r.Kind {
	case "level", "level-advanced":
		return p.GetLevel(), nil
	case "vip":
		return p.GetVip(), nil
	case "fight":
		return p.GetFightPower(), nil
	case "trial":
		if !tx.Migrator().HasTable(&db.P7RushState{}) {
			return 0, nil
		}
		var progress db.P7RushState
		err := tx.Select("record_stars").Where("player_id=?", p.GetPlayerId()).First(&progress).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return progress.RecordStars, err
	case "copy-stars", "copy-stars-advanced":
		highest := map[string]int32{}
		for _, passed := range g.Copy.GetPassedCopyInfo() {
			mission := s.table("CopyMission", "MissionId", passed.GetMissionId())
			if mission == nil || mission["ChapterId"] != passed.GetChapterId() {
				continue
			}
			key := passed.GetChapterId() + "/" + passed.GetMissionId()
			highest[key] = max(highest[key], min(int32(3), max(int32(0), passed.GetStar())))
		}
		var total int64
		for _, v := range highest {
			total += int64(v)
		}
		return int32(min(total, int64(math.MaxInt32))), nil
	case "equipped-heroes", "treasure-heroes", "formation-level", "formation-breach":
		return s.p5AchievementFormation(g, r.Kind), nil
	case "expedition-hours":
		if !tx.Migrator().HasTable(&db.P7ExpeditionMission{}) {
			return 0, nil
		}
		var missions []db.P7ExpeditionMission
		if err := tx.Where("player_id=? AND state IN ?", p.GetPlayerId(), []int32{constUtil.P7ExpeditionRunning, constUtil.P7ExpeditionReady, constUtil.P7ExpeditionClaimed}).Find(&missions).Error; err != nil {
			return 0, err
		}
		var total int64
		for _, mission := range missions {
			quote, err := p7ReadExpeditionQuote(mission)
			if err != nil {
				return 0, err
			}
			now, err := s.p7ExpeditionMissionNow(tx, mission, time.Now())
			if err != nil {
				return 0, err
			}
			hours := max(int64(0), min(now.UnixMilli(), mission.EndAt)-mission.StartAt) / 3600000
			if mission.State == constUtil.P7ExpeditionClaimed {
				hours = int64(quote.Hours)
			}
			total += min(hours, int64(quote.Hours))
		}
		return int32(min(total, int64(math.MaxInt32))), nil
	case "wanted-kills":
		if !tx.Migrator().HasTable(&db.P12WantedInstance{}) {
			return 0, nil
		}
		var count int64
		err := tx.Model(&db.P12WantedInstance{}).Where("killer_id=? AND killed_at>0", p.GetPlayerId()).Count(&count).Error
		return int32(min(count, int64(math.MaxInt32))), err
	case "friend-disaster-help":
		if !tx.Migrator().HasTable(&db.P10ExpeditionHelpAction{}) {
			return 0, nil
		}
		var count int64
		err := tx.Model(&db.P10ExpeditionHelpAction{}).Where("helper_id=? AND owner_id<>?", p.GetPlayerId(), p.GetPlayerId()).Count(&count).Error
		return int32(min(count, int64(math.MaxInt32))), err
	case "pet-recruit":
		if !tx.Migrator().HasTable(&db.P9PetRecruitProgress{}) {
			return 0, nil
		}
		var row db.P9PetRecruitProgress
		err := tx.First(&row, "player_id=?", p.GetPlayerId()).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, nil
		}
		return int32(min(row.Draws, int64(math.MaxInt32))), err
	case "pet-count", "pet-elements", "pet-level", "pet-breach", "pet-talents", "pet-intimacy":
		return s.p5AchievementPetValue(tx, p.GetPlayerId(), r.Kind)
	case "awake-1-1":
		return s.p5AchievementAwake(tx, p.GetPlayerId())
	case "collection":
		var rows []db.P5Collection
		if err := tx.Where("player_id=?", p.GetPlayerId()).Find(&rows).Error; err != nil {
			return 0, err
		}
		var n int32
		for _, v := range rows {
			row := s.table("Hero", "id", v.HeroID)
			if row != nil && row["heroClass"] != "100" && row["heroClass"] != "101" && row["heroClass"] != "99" {
				n++
			}
		}
		return n, nil
	case "warrior_training":
		var amount int64
		err := tx.Model(&db.EventFact{}).Select("COALESCE(SUM(amount),0)").Where("player_id=? AND kind=?", p.GetPlayerId(), "warrior_training").Scan(&amount).Error
		return int32(min(int64(math.MaxInt32), amount)), err
	}
	return 0, p5Error()
}
func p5AchievementSnapshot(tx *gorm.DB, id int64, r *db.P5Achievement) (*db.P5AchievementClaim, *db.P5Achievement, error) {
	var state db.P5AchievementClaim
	err := tx.First(&state, "player_id=? AND id=?", id, r.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		raw, err := json.Marshal(r)
		if err != nil {
			return nil, nil, err
		}
		state = db.P5AchievementClaim{PlayerID: id, ID: r.ID, Snapshot: string(raw)}
		if err = tx.Create(&state).Error; err != nil {
			return nil, nil, err
		}
	} else if err != nil {
		return nil, nil, err
	}
	var frozen db.P5Achievement
	if err = json.Unmarshal([]byte(state.Snapshot), &frozen); err != nil {
		return nil, nil, err
	}
	if frozen.Target < 1 || frozen.Source == "" || frozen.Reward == "" {
		return nil, nil, p5Error()
	}
	return &state, &frozen, nil
}
func (s *GameService) p5Achievements(ctx context.Context, id int64) ([]proto.Message, error) {
	out := &pb.SCAchievementList{}
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, _ *db.Profile, p *pb.Player, g *model.GameState) error {
		if err := s.ObserveP5Collection(tx, id, g); err != nil {
			return err
		}
		var rules []db.P5Achievement
		if err := tx.Order("id").Find(&rules).Error; err != nil {
			return err
		}
		valuesByKind := map[string]int32{}
		for _, r := range rules {
			// Disabled rows no longer create player snapshots; already frozen contracts remain claimable.
			if r.Disabled {
				var prior db.P5AchievementClaim
				err := tx.First(&prior, "player_id=? AND id=?", id, r.ID).Error
				if errors.Is(err, gorm.ErrRecordNotFound) {
					continue
				}
				if err != nil {
					return err
				}
			}
			state, frozen, err := p5AchievementSnapshot(tx, id, &r)
			if err != nil {
				return err
			}
			value, known := valuesByKind[frozen.Kind]
			if !known {
				value, err = s.p5AchievementValue(tx, p, g, frozen)
				if err != nil {
					return err
				}
				valuesByKind[frozen.Kind] = value
			}
			value = max(value, state.Progress)
			if value > state.Progress {
				state.Progress = value
				if err = tx.Save(state).Error; err != nil {
					return err
				}
			}
			status := int32(0)
			if value >= frozen.Target {
				status = 1
			}
			if state.ClaimedAt > 0 {
				status = 2
			}
			out.List = append(out.List, &pb.AchievementInfo{Id: proto.String(r.ID), Type: proto.Int32(max(int32(1), frozen.Type)), State: proto.Int32(status), CurrentNum: proto.Int32(value), TotalNum: proto.Int32(frozen.Target), Reward: proto.String(frozen.Reward)})
		}
		return nil
	})
	return []proto.Message{out}, err
}
func (s *GameService) p5AchievementReward(ctx context.Context, id int64, b *pb.CSGetAchievementReward, stamp int64) ([]proto.Message, error) {
	out := &pb.SCGetAchievementReward{}
	return s.p5Mutation(ctx, id, "achievement", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		var r db.P5Achievement
		if err := tx.First(&r, "id=?", b.GetId()).Error; err != nil {
			return err
		}
		if r.Disabled {
			var prior db.P5AchievementClaim
			if err := tx.First(&prior, "player_id=? AND id=?", id, r.ID).Error; err != nil {
				return p5Error()
			}
		}
		state, frozen, err := p5AchievementSnapshot(tx, id, &r)
		if err != nil {
			return err
		}
		value, err := s.p5AchievementValue(tx, p, g, frozen)
		if err != nil {
			return err
		}
		value = max(value, state.Progress)
		state.Progress = value
		if state.ClaimedAt > 0 || value < frozen.Target {
			return p5Error()
		}
		gain, err := s.GrantFixedReward(p, g, frozen.Reward)
		if err != nil {
			return err
		}
		state.ClaimedAt = time.Now().UnixMilli()
		out.Get = gain
		return tx.Save(state).Error
	})
}

type p5TitleState struct {
	Effects  string `json:"effects"`
	DeadTime int64  `json:"deadTime"`
	Source   string `json:"source"`
}

func (s *GameService) p5TitleQualification(tx *gorm.DB, p *pb.Player, g *model.GameState, row map[string]string) (int32, int32, error) {
	switch row["type"] {
	case "1":
		var n int32
		for _, slot := range g.Formation {
			if formationHero(slot) != nil {
				n++
			}
		}
		target := int32(tableNumber(row, "param"))
		if target < 1 {
			return 0, 0, p5Error()
		}
		return n, target, nil
	case "2", "3", "4", "5", "6", "7", "8":
		var (
			space   = map[string]string{"2": "1", "3": "2", "4": "3"}[row["type"]]
			quality = map[string]string{"5": "13", "6": "18", "7": "20", "8": "23"}[row["type"]]
		)
		var holdings []db.P5Collection
		if err := tx.Where("player_id=?", p.GetPlayerId()).Find(&holdings).Error; err != nil {
			return 0, 0, err
		}
		owned := map[string]bool{}
		for _, r := range holdings {
			owned[r.HeroID] = true
		}
		var n, total int32
		// Match the original GetAllHeros -> AllHerosPage visible card catalog.
		// Historical ownership remains untouched; monsters, main templates, hidden
		// cards and green training partners do not enlarge the displayed cohort.
		for _, hero := range s.Tables["Hero"] {
			if !strings.HasPrefix(hero["id"], "hero") || hero["heroClass"] != "0" || hero["iconName"] == "" || tableNumber(hero, "IsShowInCard") != 0 || tableNumber(hero, "quality") <= 12 {
				continue
			}
			if (space != "" && hero["space"] == space) || (quality != "" && hero["quality"] == quality) {
				total++
				if owned[hero["id"]] {
					n++
				}
			}
		}
		return n, total, nil
	case "9":
		target := int32(tableNumber(row, "param"))
		if target < 1 || p.GetLevel() < 1 {
			return 0, 0, p5Error()
		}
		return p.GetLevel(), target, nil
	case "10":
		// An opened chapter can still have uncompleted missions. Count a chapter
		// only after every original mission has an actual saved positive star.
		target := int32(tableNumber(row, "param"))
		if target < 1 || s.table("CopyChapter", "ChapterId", row["param"]) == nil {
			return 0, 0, p5Error()
		}
		var (
			stars   = map[string]int32{}
			totals  = map[string]int32{}
			passed  = map[string]int32{}
			highest int32
		)
		for _, saved := range g.Copy.GetPassedCopyInfo() {
			key := saved.GetChapterId() + "/" + saved.GetMissionId()
			if _, exists := stars[key]; !exists {
				stars[key] = saved.GetStar()
			}
		}
		for _, mission := range s.Tables["CopyMission"] {
			chapter := mission["ChapterId"]
			totals[chapter]++
			if star := stars[chapter+"/"+mission["MissionId"]]; star > 0 && star <= 5 {
				passed[chapter]++
			}
		}
		for _, chapter := range s.Tables["CopyChapter"] {
			id := chapter["ChapterId"]
			if totals[id] > 0 && passed[id] == totals[id] {
				highest = max(highest, int32(tableNumber(chapter, "ChapterId")))
			}
		}
		return highest, target, nil
	case "14":
		// Rank titles use only the owning arena's accepted immutable winner proof.
		// Initial placement and legacy BestWonRank do not grant qualifications.
		target := int32(tableNumber(row, "param"))
		if target < 1 {
			return 0, 0, p5Error()
		}
		rank, proof, err := P11ArenaVerifiedBestWonRank(tx, p.GetPlayerId())
		if err != nil {
			return 0, 0, err
		}
		if rank <= 0 || proof == "" {
			return 0, target, nil
		}
		return int32(min(rank, int64(math.MaxInt32))), target, nil
	case "13":
		// HighFloor is advanced by the owning Rush settlement and survives resets.
		// Reading a title never advances a floor, creates state or enables a battle.
		target := int32(tableNumber(row, "param"))
		if target < 1 {
			return 0, 0, p5Error()
		}
		if !tx.Migrator().HasTable(&db.P7RushState{}) {
			return 0, target, nil
		}
		var state db.P7RushState
		if err := tx.Model(&db.P7RushState{}).Select("high_floor").Where("player_id=?", p.GetPlayerId()).Scan(&state).Error; err != nil {
			return 0, 0, err
		}
		if state.HighFloor < 0 {
			return 0, 0, p5Error()
		}
		return state.HighFloor, target, nil
	case "11", "12":
		// Original title015..018 read the player's actual jewelry/cooking craft level.
		// Missing optional old schema gives no qualification; a fresh skill starts at level1.
		target := int32(tableNumber(row, "param"))
		var (
			model   any = &db.GemCraftState{}
			maximum     = int32(10)
			state       = db.GemCraftState{Level: 1}
		)
		if row["type"] == "12" {
			model, maximum = &db.CookingCraftState{}, 6
		}
		if target < 1 || target > maximum {
			return 0, 0, p5Error()
		}
		if !tx.Migrator().HasTable(model) {
			return 0, target, nil
		}
		if err := tx.Model(model).Select("level", "exp").Where("player_id=?", p.GetPlayerId()).Scan(&state).Error; err != nil {
			return 0, 0, err
		}
		if state.Level < 1 || state.Level > maximum || state.Exp < 0 {
			return 0, 0, p5Error()
		}
		return state.Level, target, nil
	}
	return 0, 0, nil // later competitive/season titles require their owner phase's server facts.
}
func (s *GameService) p5TitleListTx(tx *gorm.DB, p *pb.Player, g *model.GameState, now time.Time) ([]*pb.TitleInfo, error) {
	if err := s.ObserveP5Collection(tx, p.GetPlayerId(), g); err != nil {
		return nil, err
	}
	out := []*pb.TitleInfo{}
	// One original list uses one verified winner snapshot for all five ranks.
	// This cache exists only in the current transaction and response projection.
	var (
		arenaLoaded bool
		arenaRank   int64
		arenaProof  string
	)
	overrides := map[string]db.AdminTitleOverride{}
	if tx.Migrator().HasTable(&db.AdminTitleOverride{}) {
		var values []db.AdminTitleOverride
		if err := tx.Find(&values).Error; err != nil {
			return nil, err
		}
		for _, v := range values {
			overrides[v.ID] = v
		}
	}
	for _, original := range s.Tables["Title"] {
		row := original
		enabled := true
		if value, ok := overrides[original["titleId"]]; ok {
			row = map[string]string{}
			for k, v := range original {
				row[k] = v
			}
			row["param"] = value.Param
			row["effects"] = value.Effects
			row["effectTime"] = strconv.FormatInt(value.EffectTime, 10)
			if value.Show {
				row["isNeedShow"] = "0"
			} else {
				row["isNeedShow"] = "1"
			}
			enabled = value.Enabled
		}
		id := row["titleId"]
		if id == "" {
			continue
		}
		var (
			current, total int32
			err            error
		)
		if row["type"] == "14" {
			if !arenaLoaded {
				arenaRank, arenaProof, err = P11ArenaVerifiedBestWonRank(tx, p.GetPlayerId())
				if err != nil {
					return nil, err
				}
				arenaLoaded = true
			}
			total = int32(tableNumber(row, "param"))
			if total < 1 {
				return nil, p5Error()
			}
			if arenaRank > 0 && arenaProof != "" {
				current = int32(min(arenaRank, int64(math.MaxInt32)))
			}
		} else {
			current, total, err = s.p5TitleQualification(tx, p, g, row)
			if err != nil {
				return nil, err
			}
		}
		raw := p5KV(g, "p5.title:"+id)
		var owned p5TitleState
		if enabled && (row["type"] == "17" || row["type"] == "18") {
			// A new delivered month may renew the same original generic title.
			// Frozen cutoff expiry prevents delayed listing from extending its term.
			current, total = 0, 1
			grant, err := p13UFMonthlyTitle(tx, p.GetPlayerId(), id, now)
			if err != nil {
				return nil, err
			}
			if grant != nil {
				current = 1
				owned = p5TitleState{Effects: grant.Effects, DeadTime: grant.ExpiresAt, Source: grant.Source + "; month=" + grant.Month + "; result=" + grant.ResultHash}
				encoded, err := json.Marshal(owned)
				if err != nil {
					return nil, err
				}
				if string(encoded) != raw {
					raw = string(encoded)
					p5SetKV(g, "p5.title:"+id, raw)
				}
			}
		}
		qualified := total > 0 && current >= total
		if row["type"] == "14" {
			qualified = total > 0 && current > 0 && current <= total
		}
		if raw == "" && enabled && qualified {
			hours := int64(tableNumber(row, "effectTime"))
			if hours < 0 || hours > 87600 {
				return nil, p5Error()
			}
			owned = p5TitleState{Effects: row["effects"], Source: "APK TableTitle；实际阵容、曾持有图鉴、当前角色/生活制作等级、完整已通章节及试炼最高通关层资格；effectTime沿有限本地小时规则（672/336为28/14日，原服单位未取到服务证据）"}
			if row["type"] == "14" {
				if arenaRank <= 0 || arenaProof == "" || arenaRank != int64(current) {
					return nil, p5Error()
				}
				owned.Source = "APK TableTitle type14; accepted original arena winner; verified best rank=" + strconv.FormatInt(arenaRank, 10) + "; proof=" + arenaProof
			}
			if hours > 0 {
				owned.DeadTime = now.UnixMilli() + hours*3600*1000
			}
			encoded, _ := json.Marshal(owned)
			raw = string(encoded)
			p5SetKV(g, "p5.title:"+id, raw)
		}
		state := int32(0)
		if raw != "" {
			if err = json.Unmarshal([]byte(raw), &owned); err != nil {
				return nil, err
			}
			if owned.DeadTime == 0 || owned.DeadTime > now.UnixMilli() {
				state = 1
			}
		}
		if value, ok := overrides[id]; ok && !value.Show && raw == "" {
			continue
		}
		out = append(out, &pb.TitleInfo{TitleId: proto.String(id), State: proto.Int32(state), Current: proto.Int32(current), Total: proto.Int32(total), DeadTime: proto.Int64(owned.DeadTime)})
	}
	active := p5KV(g, "p5.current-title")
	valid := active == ""
	for _, r := range out {
		if r.GetTitleId() == active && r.GetState() == 1 {
			valid = true
		}
	}
	if !valid {
		p5SetKV(g, "p5.current-title", "")
	}
	return out, nil
}
func (s *GameService) p5Titles(ctx context.Context, id int64) ([]proto.Message, error) {
	out := &pb.SCTitleList{}
	var updated *pb.Player
	err := s.p3Profile(ctx, id, func(tx *gorm.DB, row *db.Profile, p *pb.Player, g *model.GameState) error {
		titles, err := s.p5TitleListTx(tx, p, g, p13UFNow(ctx))
		if err != nil {
			return err
		}
		out.TitleList = titles
		out.CurTitle = proto.String(p5KV(g, "p5.current-title"))
		power, err := s.formationFightPower(p, g)
		if err != nil {
			return err
		}
		p.FightPower = proto.Int32(power)
		updated = proto.Clone(p).(*pb.Player)
		return saveProfile(tx, row, p, g)
	})
	return []proto.Message{out, &pb.SCPlayer{Player: updated}}, err
}
func (s *GameService) p5ChangeTitle(ctx context.Context, id int64, b *pb.CSChangeTitle, stamp int64) ([]proto.Message, error) {
	out := &pb.SCChangeTitle{}
	return s.p5Mutation(ctx, id, "title", stamp, b, out, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		titles, err := s.p5TitleListTx(tx, p, g, p13UFNow(ctx))
		if err != nil {
			return err
		}
		if b.GetTitleId() != "" {
			valid := false
			for _, t := range titles {
				valid = valid || (t.GetTitleId() == b.GetTitleId() && t.GetState() == 1)
			}
			if !valid {
				return p5Error()
			}
		}
		p5SetKV(g, "p5.current-title", b.GetTitleId())
		out.CurTitle = proto.String(b.GetTitleId())
		out.TitleList = titles
		return nil
	})
}
func (s *GameService) p5TitleBonus(g *model.GameState, values map[string]float64) error {
	id := p5KV(g, "p5.current-title")
	if id == "" {
		return nil
	}
	raw := p5KV(g, "p5.title:"+id)
	var title p5TitleState
	if json.Unmarshal([]byte(raw), &title) != nil || title.Source == "" {
		return p5Error()
	}
	// Attribute expiry follows the same server-wide calendar as the title list.
	// This calculation is read-only and leaves real billing/session clocks untouched.
	now := time.Now()
	if s.ServerClockReady {
		if clock := s.ServerClockState.Load(); clock != nil {
			now = now.AddDate(0, 0, int(clock.OffsetDays))
		}
	}
	if title.DeadTime > 0 && title.DeadTime <= now.UnixMilli() {
		return nil
	}
	for _, entry := range strings.Split(title.Effects, ";") {
		pair := strings.Split(entry, ",")
		if len(pair) != 2 {
			return p5Error()
		}
		value, err := strconv.ParseFloat(pair[1], 64)
		if err != nil || value < 0 || math.IsInf(value, 0) || math.IsNaN(value) {
			return p5Error()
		}
		switch pair[0] {
		case "hp", "att", "pdef", "mdef", "cri", "hit", "dodge", "antiCri", "criFactor", "damageAdd", "damageReduce":
			values[pair[0]] += value
		default:
			return p5Error()
		}
	}
	return nil
}

// p5AchievementFormation reads actual inventory-backed native formation slots.
func (s *GameService) p5AchievementFormation(g *model.GameState, kind string) int32 {
	var count int32
	lowest := int32(math.MaxInt32)
	seen := map[string]bool{}
	for _, slot := range g.Formation {
		hero := formationHero(slot)
		if hero == nil || seen[hero.GetGuid()] {
			continue
		}
		warrior, err := s.formationWarrior(g, hero.GetGuid(), hero.GetId())
		if err != nil {
			continue
		}
		seen[hero.GetGuid()] = true
		if kind == "formation-level" || kind == "formation-breach" {
			value := warrior.GetLevel()
			if kind == "formation-breach" {
				value = warrior.GetBreached().GetLevel()
			}
			lowest = min(lowest, value)
			count++
			continue
		}
		slots := map[int32]bool{}
		for _, thing := range slot.GetThings() {
			pos := thing.GetPos()
			if kind == "equipped-heroes" && pos >= 1 && pos <= 4 {
				for _, item := range g.Equipment {
					if item.GetGuid() == thing.GetGuid() && item.GetEquipId() == thing.GetId() {
						slots[pos] = true
						break
					}
				}
			}
			if kind == "treasure-heroes" && (pos == 5 || pos == 6) {
				for _, item := range g.Treasures {
					if item.GetGuid() == thing.GetGuid() && item.GetTreasureId() == thing.GetId() {
						slots[pos] = true
						break
					}
				}
			}
		}
		if (kind == "equipped-heroes" && len(slots) == 4) || (kind == "treasure-heroes" && len(slots) == 2) {
			count++
		}
	}
	if kind == "formation-level" || kind == "formation-breach" {
		if count < 4 {
			return 0
		}
		return lowest
	}
	return count
}

// p5AchievementPets unifies actual held, retired and consumed donor GUID histories.
// Snapshots stay intact: a reset, sale or growth donation cannot erase an observed lifetime condition.
func p5AchievementPets(tx *gorm.DB, owner int64) ([]*pb.Pet, error) {
	out := []*pb.Pet{}
	appendPet := func(raw []byte, guid string) error {
		pet := &pb.Pet{}
		if proto.Unmarshal(raw, pet) != nil || pet.GetGuid() == "" || pet.GetId() == "" || (guid != "" && pet.GetGuid() != guid) {
			return p5Error()
		}
		out = append(out, pet)
		return nil
	}
	if tx.Migrator().HasTable(&db.P9PetInstance{}) {
		var rows []db.P9PetInstance
		if err := tx.Where("player_id=?", owner).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			if err := appendPet(r.Data, r.GUID); err != nil {
				return nil, err
			}
		}
	}
	if tx.Migrator().HasTable(&db.P9PetRetired{}) {
		var rows []db.P9PetRetired
		if err := tx.Where("player_id=?", owner).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			if err := appendPet(r.Data, r.GUID); err != nil {
				return nil, err
			}
		}
	}
	if tx.Migrator().HasTable(&db.P9PetInvestment{}) {
		var rows []db.P9PetInvestment
		if err := tx.Where("player_id=?", owner).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			var donors []*pb.Pet
			if json.Unmarshal(r.Donors, &donors) != nil {
				return nil, p5Error()
			}
			for _, pet := range donors {
				if pet == nil || pet.GetGuid() == "" || pet.GetId() == "" {
					return nil, p5Error()
				}
				out = append(out, pet)
			}
		}
	}
	return out, nil
}
func (s *GameService) p5AchievementPetValue(tx *gorm.DB, owner int64, kind string) (int32, error) {
	pets, err := p5AchievementPets(tx, owner)
	if err != nil {
		return 0, err
	}
	seen := map[string]bool{}
	families := map[string]map[string]bool{}
	var value int32
	for _, pet := range pets {
		seen[pet.GetGuid()] = true
		switch kind {
		case "pet-level":
			value = max(value, pet.GetLevel())
		case "pet-breach":
			value = max(value, pet.GetBreached().GetLevel())
		case "pet-intimacy":
			value = max(value, pet.GetEat().GetLevel())
		case "pet-talents":
			active := map[string]bool{}
			for _, talent := range pet.GetTalentList() {
				if talent.GetTalent() != "" && talent.GetTalentLevel() <= pet.GetBreached().GetLevel() {
					active[talent.GetTalent()] = true
				}
			}
			value = max(value, int32(len(active)))
		case "pet-elements":
			row := s.table("Pet", "id", pet.GetId())
			if row == nil || row["resName"] == "" || row["nature"] == "" {
				continue
			}
			if families[row["resName"]] == nil {
				families[row["resName"]] = map[string]bool{}
			}
			families[row["resName"]][row["nature"]] = true
			value = max(value, int32(len(families[row["resName"]])))
		}
	}
	if kind == "pet-count" {
		value = int32(min(len(seen), math.MaxInt32))
	}
	return value, nil
}
func (s *GameService) p5AchievementAwake(tx *gorm.DB, owner int64) (int32, error) {
	if !tx.Migrator().HasTable(&db.P7AwakeRun{}) || !tx.Migrator().HasTable(&db.P7AwakeExploration{}) {
		return 0, nil
	}
	var runs []db.P7AwakeRun
	if err := tx.Where("player_id=? AND copy_id=? AND diagnostic=?", owner, constUtil.P7AwakeExploreCopy, false).Find(&runs).Error; err != nil {
		return 0, err
	}
	var value int32
	for _, run := range runs {
		frozen, err := p7AwakeReadFrozen(&run)
		if err != nil {
			return 0, err
		}
		var ledger db.P7AwakeExploration
		if err = tx.First(&ledger, "run_id=? AND player_id=?", run.ID, owner).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		} else if err != nil {
			return 0, err
		}
		state, err := p7AwakeReadProgress(&ledger, frozen)
		if err != nil {
			return 0, err
		}
		value = max(value, state.Score)
	}
	return value, nil
}
