package service

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// navigationItems restores the original 20125/20126 snapshot. The client replaces
// all server-owned hints on each reply, and appends even Value=0 entries. Emit
// actionable entries only; the original five local hints remain client-owned.
// Profile locking serializes this snapshot with inventory/claim transactions.
func (s *GameService) navigationItems(ctx context.Context, id int64) ([]proto.Message, error) {
	out := &pb.SCNavigationItem{}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		p, g, err := decodeProfile(&row)
		if err != nil {
			return err
		}
		flags := map[int32]bool{}
		if s.P5Ready {
			flags[1] = s.navigationTreasure(p, g)
			flags[10] = s.navigationWorldSeed(p, g)
		}
		if err = s.navigationLife(tx, p, g, flags); err != nil {
			return err
		}
		now := time.Now()
		if err = s.navigationDaily(tx, p, now, flags); err != nil {
			return err
		}
		if err = s.navigationAwake(tx, p, now, flags); err != nil {
			return err
		}
		if err = s.navigationRush(tx, p, g, now, flags); err != nil {
			return err
		}
		if err = s.navigationExpedition(tx, p, g, now, flags); err != nil {
			return err
		}
		if err = s.navigationFriendHelp(tx, p, now, flags); err != nil {
			return err
		}
		if err = s.navigationGuild(tx, p, now, flags); err != nil {
			return err
		}
		if err = s.navigationMemory(tx, p, now, flags); err != nil {
			return err
		}
		out = navigationProjection(flags)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{out}, nil
}

func navigationProjection(flags map[int32]bool) *pb.SCNavigationItem {
	out := &pb.SCNavigationItem{}
	for kind := int32(1); kind <= 22; kind++ {
		switch kind {
		case 11, 12, 19, 20, 21:
			continue
		}
		if flags[kind] {
			out.NavItems = append(out.NavItems, &pb.NavigationItem{Type: proto.Int32(kind), Value: proto.Int32(1)})
		}
	}
	return out
}

// navigationCanPay checks a single original recipe without mutating stacks.
// Duplicate/negative stacks fail closed, as the real craft mutation does.
func navigationCanPay(p *pb.Player, stacks []*pb.SimpleItem, spec string) bool {
	owned := map[string]int64{"coin": p.GetCoin(), "diamond": p.GetDiamond()}
	for _, item := range stacks {
		if item == nil || item.GetId() == "" || item.GetNum() < 0 {
			return false
		}
		if _, ok := owned[item.GetId()]; ok {
			return false
		}
		owned[item.GetId()] = int64(item.GetNum())
	}
	seen := map[string]bool{}
	if spec == "" {
		return false
	}
	for _, raw := range strings.Split(spec, ";") {
		pair := strings.Split(raw, ",")
		if len(pair) != 2 || pair[0] == "" || seen[pair[0]] {
			return false
		}
		n, e := strconv.ParseInt(pair[1], 10, 32)
		if e != nil || n <= 0 || owned[pair[0]] < n {
			return false
		}
		seen[pair[0]] = true
	}
	return true
}

func (s *GameService) navigationTreasure(p *pb.Player, g *model.GameState) bool {
	if treasureBagCapacity(p, g, 1) != nil {
		return false
	}
	for _, row := range s.Tables["Treasure"] {
		var costs []string
		for _, frag := range s.Tables["Frag"] {
			if frag["target"] == row["id"] {
				costs = append(costs, frag["id"]+","+frag["num"])
			}
		}
		if len(costs) > 0 && len(costs) == int(tableNumber(row, "needChipNum")) && navigationCanPay(p, g.Items, strings.Join(costs, ";")) {
			return true
		}
	}
	return false
}
func (s *GameService) navigationWorldSeed(p *pb.Player, g *model.GameState) bool {
	for _, row := range s.Tables["WorldSeed"] {
		if p5SeedID(row) == p.GetCurrentWorldSeedId()+1 {
			return navigationCanPay(p, g.Items, row["cost"])
		}
	}
	return false
}

func (s *GameService) navigationLife(tx *gorm.DB, p *pb.Player, g *model.GameState, flags map[int32]bool) error {
	if !s.P8LifeReady || s.cookingOpen(p) != nil {
		return nil
	}
	skills := map[string]bool{}
	var learned []db.P8LifeSkill
	if err := tx.Where("player_id=?", p.GetPlayerId()).Find(&learned).Error; err != nil {
		return err
	}
	for _, r := range learned {
		skills[r.SkillID] = true
	}
	var cooking db.CookingCraftState
	cooking.Level = 1
	err := tx.Where("player_id=?", p.GetPlayerId()).First(&cooking).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	for _, r := range s.Tables["FishSkill"] {
		if r["Type"] == "dish" && r["IsDish"] == "1" && tableNumber(r, "LevelLimit") >= 1 && tableNumber(r, "LevelLimit") <= float64(cooking.Level) && (r["SkillNeeded"] == "" || skills[r["SkillNeeded"]]) && navigationCanPay(p, g.Cooking, r["Material"]) {
			flags[15] = true
			break
		}
	}
	var gem db.GemCraftState
	gem.Level = 1
	err = tx.Where("player_id=?", p.GetPlayerId()).First(&gem).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var rule db.GemRule
	err = tx.First(&rule, 1).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	// Original OnGemMake checks IsHaveMineCanResolve: five ore for refinement.
	// A craftable finished gem is also a real action at this same original NPC.
	for _, r := range s.Tables["MineSkill"] {
		if s.gemLevelGate(p, &gem, r) != nil || (r["SkillNeeded"] != "" && !skills[r["SkillNeeded"]]) {
			continue
		}
		if r["Type"] == "stone" && rule.Enabled && navigationCanPay(p, g.Gems, r["MineItemID"]+",5") {
			flags[14] = true
			break
		}
		if r["Type"] == "magicGem" && navigationCanPay(p, g.Gems, r["Material"]) {
			flags[14] = true
			break
		}
	}
	return nil
}

func (s *GameService) navigationDaily(tx *gorm.DB, p *pb.Player, now time.Time, flags map[int32]bool) error {
	if !tx.Migrator().HasTable(&db.DailyCopyPolicy{}) {
		return nil
	}
	info, err := s.dailyCopyInfo(tx, p, now)
	if err != nil {
		return err
	}
	for _, kind := range info.Items {
		if kind.GetIsOpen() > 0 && kind.GetLastTime() > 0 {
			for _, stage := range kind.DailyCopyState {
				if stage.GetIsOpen() > 0 && p.GetLevel() >= stage.GetLevelLimit() {
					flags[13] = true
				}
			}
		}
	}
	return nil
}
func (s *GameService) navigationAwake(tx *gorm.DB, p *pb.Player, now time.Time, flags map[int32]bool) error {
	if !tx.Migrator().HasTable(&db.P7AwakePolicy{}) {
		return nil
	}
	var policy db.P7AwakePolicy
	err := tx.First(&policy, constUtil.P7AwakeExplorePolicyID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !policy.Enabled || policy.Source != constUtil.P7AwakeExploreSource || p.GetLevel() < policy.MinLevel {
		return nil
	}
	_, left, err := s.p7AwakeState(tx, p.GetPlayerId(), now)
	if err != nil || left <= 0 {
		return err
	}
	var rules []db.P7AwakeStageRule
	if err = tx.Where("enabled=? AND content_restored=?", true, true).Find(&rules).Error; err != nil {
		return err
	}
	for _, r := range rules {
		open, e := p7AwakeOpenDay(s.table("AwakeCopyChapter", "chapterId", r.ChapterID)["openDay"], LocalPlayAt(tx.Statement.Context, now))
		if e != nil {
			return e
		}
		if open && r.Source == constUtil.P7AwakeExploreSource {
			flags[22] = true
		}
	}
	return nil
}
func (s *GameService) navigationRush(tx *gorm.DB, p *pb.Player, g *model.GameState, now time.Time, flags map[int32]bool) error {
	if !tx.Migrator().HasTable(&db.P7RushPolicy{}) {
		return nil
	}
	cfg, err := s.p7Policy(tx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil || !cfg.Enabled {
		return err
	}
	st, err := p7State(tx, p.GetPlayerId(), now)
	if err != nil {
		return err
	}
	if st.ActiveFight != "" {
		return nil
	}
	authority, err := p7AuthorityAvailable(tx)
	if err != nil {
		return err
	}
	failed, err := p7TrialMustReset(tx, st)
	if err != nil {
		return err
	}
	completed, err := s.p7CompletedLocalSectionResetAllowed(tx, st)
	if err != nil {
		return err
	}
	free, err := s.p7VIP(p, "numPerk2")
	if err != nil {
		return err
	}
	flags[2] = (authority || failed || completed) && (st.Floor > 1 || st.TotalStars > 0 || failed) && st.FreeResetUsed < free
	count, err := s.p7VIP(p, "numPerks24")
	if err != nil {
		return err
	}
	if st.EliteUsed >= count+st.EliteBought {
		return nil
	}
	stages, err := s.p7Stages(tx, constUtil.P7RushElite)
	if err != nil {
		return err
	}
	for _, stage := range stages {
		if !stage.Enabled || p.GetLevel() < stage.MinLevel {
			continue
		}
		passed, e := p7HasPassed(tx, p.GetPlayerId(), stage.ConditionTaskID)
		if e != nil {
			return e
		}
		if !passed {
			continue
		}
		scoped := false
		if !authority {
			scoped, e = s.p7ScopedStageAvailable(tx.Statement.Context, tx, p, g, stage, now)
			if e != nil {
				return e
			}
		}
		if authority || scoped {
			if _, e = s.validateP7Stage(stage); e == nil {
				flags[3] = true
				break
			}
		}
	}
	return nil
}

func (s *GameService) navigationExpedition(tx *gorm.DB, p *pb.Player, g *model.GameState, now time.Time, flags map[int32]bool) error {
	if !tx.Migrator().HasTable(&db.P7ExpeditionPolicy{}) {
		return nil
	}
	var missions []db.P7ExpeditionMission
	if err := tx.Where("player_id=? AND state IN ?", p.GetPlayerId(), []int32{0, 1, 2}).Find(&missions).Error; err != nil {
		return err
	}
	active := int32(0)
	for _, m := range missions {
		if m.State != constUtil.P7ExpeditionOffered {
			active++
			at, err := s.p7ExpeditionMissionNow(tx, m, now)
			if err != nil {
				return err
			}
			if m.State == constUtil.P7ExpeditionReady || at.UnixMilli() >= m.EndAt {
				flags[7] = true
			}
		}
	}
	var policy db.P7ExpeditionPolicy
	err := tx.First(&policy, constUtil.P7ExpeditionPolicyID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	level := tableNumber(s.table("GameConfigure", "StaticName", "SailingLevel"), "NameValue")
	if !policy.Enabled || level <= 0 || float64(p.GetLevel()) < level {
		return nil
	}
	state := db.P7ExpeditionState{Level: "D"}
	err = tx.First(&state, "player_id=?", p.GetPlayerId()).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	limit := int32(tableNumber(s.table("ExpeditionLevel", "level", state.Level), "maxMissionNum"))
	if active >= limit {
		return nil
	}
	var busy []db.P7ExpeditionHero
	if err = tx.Where("player_id=?", p.GetPlayerId()).Find(&busy).Error; err != nil {
		return err
	}
	used := map[string]bool{}
	for _, r := range busy {
		used[r.GUID] = true
	}
	idle := false
	for _, w := range g.Warriors {
		if w.GetGuid() != p.GetHeroWarId() && !used[w.GetGuid()] && !strings.HasPrefix(w.GetId(), "hero_exp_") && !p7ExpeditionInFormation(g, w.GetGuid()) {
			idle = true
			break
		}
	}
	if !idle {
		return nil
	}
	for _, m := range missions {
		if m.State != constUtil.P7ExpeditionOffered {
			continue
		}
		q, e := p7ReadExpeditionQuote(m)
		if e != nil {
			return e
		}
		if p.GetEnergy() >= q.Energy {
			flags[8] = true
			break
		}
	}
	return nil
}

func (s *GameService) navigationMemory(tx *gorm.DB, p *pb.Player, now time.Time, flags map[int32]bool) error {
	if !s.P4Ready || p.GetLevel() < 16 {
		return nil
	}
	var rows []db.P4CommerceRule
	if err := tx.Where("shop_type=? AND enabled=?", 3, true).Order("start_at DESC,id DESC").Find(&rows).Error; err != nil {
		return err
	}
	at := LocalPlayAt(tx.Statement.Context, now)
	for _, live := range rows {
		visible, err := localPlayActivityVisible(tx, live.ID)
		if err != nil {
			return err
		}
		if !visible {
			continue
		}
		cfg, rules, err := s.p4CommerceSnapshot(live)
		if err != nil {
			return err
		}
		if at.UnixMilli() < cfg.StartAt || at.UnixMilli() >= cfg.EndAt {
			continue
		}
		st := p4CommerceProgress{FreeLeft: 1, FreeAt: at.UnixMilli()}
		var saved db.P4CommerceState
		err = tx.First(&saved, "player_id=? AND rule_id=?", p.GetPlayerId(), cfg.ID).Error
		if err == nil {
			if json.Unmarshal(saved.Payload, &st) != nil {
				return errors.New("导航读取圣坛进度无效")
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if rules.FreeSeconds <= 0 {
			return errors.New("导航读取圣坛计时无效")
		}
		recovered := max(int64(0), (at.UnixMilli()-st.FreeAt)/(rules.FreeSeconds*1000))
		flags[4] = st.FreeLeft > 0 || recovered > 0
		return nil
	}
	return nil
}

// Guild welfare hints use the existing policy and authoritative membership.
// No donation, stamina or box claim is performed by this projection.
func (s *GameService) navigationGuild(tx *gorm.DB, p *pb.Player, now time.Time, flags map[int32]bool) error {
	if !tx.Migrator().HasTable(&db.P13GuildPolicy{}) {
		return nil
	}
	var policy db.P13GuildPolicy
	err := tx.First(&policy, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !policy.ManagementEnabled || !policy.EconomyEnabled {
		return nil
	}
	var member db.P13GuildMember
	err = tx.First(&member, "player_id=?", p.GetPlayerId()).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if member.GuildID == "" {
		return nil
	}
	var guild db.P13Guild
	if err = tx.First(&guild, "id=?", member.GuildID).Error; err != nil {
		return err
	}
	day := p13GuildDay(LocalPlayAt(tx.Statement.Context, now))
	flags[17] = member.StaminaDay != day
	spent := member.TodayCoin
	if member.DonateDay != day {
		spent = 0
	}
	amount, err := s.p10Setting("UnionMinDailyDonateCoin")
	if err != nil {
		return err
	}
	limit, err := s.p10Setting("UnionMaxDailyDonateCoin")
	if err != nil {
		return err
	}
	flags[16] = amount > 0 && p.GetCoin() >= int64(amount) && spent+int64(amount) <= int64(limit)
	if tx.Migrator().HasTable(&db.P13GuildDonateBoxPolicy{}) {
		boxes, _, _, err := s.ProjectP13GuildDonateBoxes(tx, &member, LocalPlayAt(tx.Statement.Context, now))
		if err != nil {
			return err
		}
		for _, box := range boxes {
			if box.GetState() == 1 {
				flags[18] = true
				break
			}
		}
	}
	return nil
}

func (s *GameService) navigationFriendHelp(tx *gorm.DB, p *pb.Player, now time.Time, flags map[int32]bool) error {
	if !s.P10VoiceReady || !tx.Migrator().HasTable(&db.P10ExpeditionHelpPolicy{}) {
		return nil
	}
	var policy db.P10ExpeditionHelpPolicy
	err := tx.First(&policy, 1).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if !policy.Enabled {
		return nil
	}
	level, err := s.p7ExpeditionSetting("SailingLevel")
	if err != nil {
		return err
	}
	if p.GetLevel() < level {
		return nil
	}
	// P10 daily limits deliberately retain their real owner day contract.
	daily, err := s.p10ExpeditionHelpQuote(tx, p.GetPlayerId(), p1Day(LocalPlayAt(tx.Statement.Context, now)), false)
	if err != nil {
		return err
	}
	actions, err := p10ExpeditionHelpActions(tx, daily)
	if err != nil {
		return err
	}
	limit, err := p10ExpeditionHelpLimit(daily, p.GetVip())
	if err != nil {
		return err
	}
	if int64(len(actions)) >= limit {
		return nil
	}
	friends, err := p10ExpeditionHelpFriends(tx, p.GetPlayerId())
	if err != nil {
		return err
	}
	pending, err := s.ListP7ExpeditionHelp(tx, friends, now)
	if err != nil {
		return err
	}
	flags[9] = len(pending) > 0
	return nil
}
