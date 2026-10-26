package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
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
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

// InitializeFieldBossDeath73 creates receipts only; historical battles retain their old rewards.
func (s *GameService) InitializeFieldBossDeath73(ctx context.Context) error {
	if err := db.GetIns().WithContext(ctx).AutoMigrate(&db.FieldBossContribution73{}); err != nil {
		return err
	}
	s.FieldBossDeath73Ready = true
	return nil
}

func (s *GameService) fieldBossSprouts73(tx *gorm.DB, id int64, scene string, maximum int64) (int64, error) {
	if maximum <= 0 {
		return 0, nil
	}
	if maximum > 2 {
		maximum = 2
	}
	own, err := p13TeamMember(tx, id)
	if err != nil {
		return 0, err
	}
	if own == nil || own.TeamID == "" {
		return 0, nil
	}
	members, err := p13TeamMembers(tx, own.TeamID)
	if err != nil {
		return 0, err
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for _, m := range members {
		if m.PlayerID != id && !seen[m.PlayerID] {
			seen[m.PlayerID] = true
			ids = append(ids, m.PlayerID)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	var ownPresence db.P10ScenePresence
	if err := tx.Where("player_id=? AND scene=?", id, scene).First(&ownPresence).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	var rows []db.P10ScenePresence
	if err := tx.Where("player_id IN ? AND scene=? AND line=? AND session_hash<>?", ids, scene, ownPresence.Line, "").Find(&rows).Error; err != nil {
		return 0, err
	}
	ids = nil
	presence := map[int64]db.P10ScenePresence{}
	for _, r := range rows {
		ids = append(ids, r.PlayerID)
		presence[r.PlayerID] = r
	}
	if len(ids) == 0 {
		return 0, nil
	}
	var profiles []db.Profile
	if err := tx.Where("id IN ? AND server_id=?", ids, s.OriginalServerID()).Find(&profiles).Error; err != nil {
		return 0, err
	}
	ids = nil
	for _, profile := range profiles {
		r := presence[profile.Id]
		value, ok := p10ClientSinks.Load(p10ClientKey{s.p10ClientOwner(), profile.Id})
		if !ok {
			continue
		}
		sink := value.(*p10ClientSink)
		if subtle.ConstantTimeCompare(profile.Session, sink.session) != 1 || r.SessionHash != p6Digest(sink.session) {
			continue
		}
		_, game, err := decodeProfile(&profile)
		if err != nil || game.Position.GetScene() != scene {
			continue
		}
		ids = append(ids, profile.Id)
	}
	active, err := s.SproutActivePlayers(tx, ids, time.Now())
	if err != nil {
		return 0, err
	}
	n := int64(0)
	for _, id := range ids {
		if active[id] {
			n++
			if n == maximum {
				break
			}
		}
	}
	return n, nil
}

// bossDropContent73 changes only the chance of a nonzero quantity; conditional quantities retain their weights.
func bossDropContent73(drops []model.AdminFieldDrop, sprouts, bps int64) (string, error) {
	if sprouts < 0 {
		sprouts = 0
	}
	if sprouts > 2 {
		sprouts = 2
	}
	parts := []string{}
	for _, d := range drops {
		var total, nonzero int64
		for _, q := range d.Quantities {
			if q.Amount < 0 || q.Weight <= 0 || q.Weight > math.MaxInt32-total {
				return "", errors.New("首领奖励权重无效")
			}
			total += q.Weight
			if q.Amount > 0 {
				nonzero += q.Weight
			}
		}
		if total == 0 {
			return "", errors.New("首领奖励为空")
		}
		if nonzero == 0 {
			continue
		}
		chance := math.Min(1, float64(nonzero)/float64(total)*(1+float64(sprouts*bps)/10000))
		draw, err := rand.Int(rand.Reader, big.NewInt(1000000000))
		if err != nil {
			return "", err
		}
		if float64(draw.Int64()) >= chance*1000000000 {
			continue
		}
		pick, err := rand.Int(rand.Reader, big.NewInt(nonzero))
		if err != nil {
			return "", err
		}
		v := pick.Int64()
		for _, q := range d.Quantities {
			if q.Amount == 0 {
				continue
			}
			if v < q.Weight {
				parts = append(parts, d.ResourceID+","+strconv.FormatInt(q.Amount, 10))
				break
			}
			v -= q.Weight
		}
	}
	return strings.Join(parts, ";"), nil
}

// fieldBossContribute73 runs with the shared spawn row locked and never locks another profile.
func (s *GameService) fieldBossContribute73(tx *gorm.DB, id int64, f *model.FieldState, damage float64, killed bool) error {
	fight := f.Fight
	if fight.BossDeathRules73 == nil || !fight.BossDeathRules73.DeathSettlement || damage <= 0 {
		return nil
	}
	var row db.FieldBossContribution73
	err := tx.Where("server_id=? AND spawn_uuid=? AND player_id=?", s.OriginalServerID(), fight.BossUUID, id).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		raw, e := json.Marshal(fight.BossDeathRules73)
		if e != nil {
			return e
		}
		limit := int64(3)
		if fight.BossRewardDailyLimit != nil && *fight.BossRewardDailyLimit >= 0 && *fight.BossRewardDailyLimit < limit {
			limit = *fight.BossRewardDailyLimit
		}
		row = db.FieldBossContribution73{ServerID: s.OriginalServerID(), SpawnUUID: fight.BossUUID, PlayerID: id, Rules: raw, Sprouts: fight.BossSprouts73, DailyLimit: limit}
		if err = tx.Create(&row).Error; err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	if row.Frozen {
		return nil
	}
	row.Damage += damage
	if err = tx.Model(&row).Update("damage", row.Damage).Error; err != nil {
		return err
	}
	if !killed {
		return nil
	}
	var rows []db.FieldBossContribution73
	if err = tx.Where("server_id=? AND spawn_uuid=? AND damage>0", s.OriginalServerID(), fight.BossUUID).Order("damage DESC, id ASC, player_id ASC").Find(&rows).Error; err != nil {
		return err
	}
	// Ordering uses committed first contribution sequence, then player identity.
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Damage != rows[j].Damage {
			return rows[i].Damage > rows[j].Damage
		}
		if rows[i].ID != rows[j].ID {
			return rows[i].ID < rows[j].ID
		}
		return rows[i].PlayerID < rows[j].PlayerID
	})
	day := LocalPlayNow(tx.Statement.Context).Format("2006-01-02")
	for i := range rows {
		r := &rows[i]
		if r.Frozen {
			continue
		}
		var rules model.AdminFieldBossGroup
		if err = json.Unmarshal(r.Rules, &rules); err != nil {
			return err
		}
		drops := append([]model.AdminFieldDrop(nil), rules.Drops...)
		rank := int64(i + 1)
		for _, band := range rules.RankDrops {
			if rank >= band.From && rank <= band.To {
				drops = append(drops, band.Drops...)
			}
		}
		if r.PlayerID == id {
			drops = append(drops, rules.KillDrops...)
		}
		reward, e := bossDropContent73(drops, r.Sprouts, rules.SproutBonusBps)
		if e != nil {
			return e
		}
		if err = tx.Model(r).Updates(map[string]any{"reward": reward, "reward_day": day, "reward_rank": rank, "frozen": true}).Error; err != nil {
			return err
		}
	}
	return nil
}

// CollectFieldBossPending consumes durable receipts under only the recipient's profile lock.
// Reconnect and repeated requests consume each spawn once, including zero-drop eligible rolls.
func (s *GameService) CollectFieldBossPending(ctx context.Context, id int64) error {
	_, err := s.CollectFieldBossPendingRewards(ctx, id)
	return err
}

// CollectFieldBossPendingRewards returns only newly committed gains for the original client reward message.
func (s *GameService) CollectFieldBossPendingRewards(ctx context.Context, id int64) (*pb.ResultObject, error) {
	gain := &pb.ResultObject{}
	if !s.FieldBossDeath73Ready {
		return nil, nil
	}
	var pending db.FieldBossContribution73
	if err := db.GetIns().WithContext(ctx).Select("id").Where("server_id=? AND player_id=? AND frozen=? AND claimed=?", s.OriginalServerID(), id, true, false).Take(&pending).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	err := updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
		if g.Field == nil {
			return nil
		}
		var rows []db.FieldBossContribution73
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("server_id=? AND player_id=? AND frozen=? AND claimed=?", s.OriginalServerID(), id, true, false).Order("id").Limit(200).Find(&rows).Error; err != nil {
			return err
		}
		today := LocalPlayNow(ctx).Format("2006-01-02")
		field := g.Field
		if err := fieldBossQuota75(tx, id, field, today); err != nil {
			return err
		}
		if field.BossRewardDay != today {
			field.BossRewardDay = today
			field.BossRewardClaims = 0
		}
		days := []string{today}
		seenDays := map[string]bool{today: true}
		for _, r := range rows {
			if !seenDays[r.RewardDay] {
				seenDays[r.RewardDay] = true
				days = append(days, r.RewardDay)
			}
		}
		usedByDay := map[string]int64{}
		if len(days) > 0 {
			var counts []struct {
				RewardDay string
				Used      int64
			}
			if err := tx.Model(&db.FieldBossContribution73{}).Select("reward_day, COUNT(*) AS used").Where("server_id=? AND player_id=? AND reward_day IN ? AND claimed=? AND reward_rank>0", s.OriginalServerID(), id, days, true).Group("reward_day").Scan(&counts).Error; err != nil {
				return err
			}
			for _, c := range counts {
				usedByDay[c.RewardDay] = c.Used
			}
		}
		cached, err := fieldBossQuota75Cached(tx, id, days)
		if err != nil {
			return err
		}
		for day, used := range cached {
			usedByDay[day] = max(usedByDay[day], used)
		}
		// The profile includes legacy victories without contribution rows; durable
		// claimed rows recover death-mode rewards after a forward/backward date.
		usedByDay[today] = max(usedByDay[today], field.BossRewardClaims)
		field.BossRewardClaims = usedByDay[today]
		for _, r := range rows {
			used := usedByDay[r.RewardDay]
			eligible := used < r.DailyLimit
			if eligible {
				if r.Reward != "" {
					awarded, err := s.openServerGain(p, g, r.Reward)
					if err != nil {
						return err
					}
					p1AppendGain(gain, awarded)
					if field.Fight != nil && field.Fight.Settled && field.Fight.BossUUID == r.SpawnUUID && field.Fight.BossResult != nil {
						if field.Fight.BossResult.Get == nil {
							field.Fight.BossResult.Get = &pb.ResultObject{}
						}
						p1AppendGain(field.Fight.BossResult.Get, proto.Clone(awarded).(*pb.ResultObject))
					}
				}
				if r.RewardDay == today {
					field.BossRewardClaims++
				}
				usedByDay[r.RewardDay]++
			}
			updates := map[string]any{"claimed": true}
			if !eligible {
				updates["reward_rank"] = int64(0)
			}
			if err := tx.Model(&r).Updates(updates).Error; err != nil {
				return err
			}
		}
		// Save changed day counts in one batch. Older pending rewards can share a
		// day with legacy grants, so retaining only claimed death rows would lose
		// the combined count on the next callback.
		if ServerGameClockContextBound(ctx) {
			archives := []db.LocalPlayActivityOperation{}
			for day, used := range usedByDay {
				if used == cached[day] {
					continue
				}
				raw, err := json.Marshal(used)
				if err != nil {
					return err
				}
				archives = append(archives, db.LocalPlayActivityOperation{PlayerID: id, RequestID: "calendar75:" + constUtil.CalendarQuota75FieldBoss + ":" + day, Action: constUtil.CalendarQuota75Action, ActivityID: constUtil.CalendarQuota75FieldBoss, Response: raw, CreatedAt: time.Now().UnixMilli()})
			}
			if len(archives) > 0 {
				if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "player_id"}, {Name: "request_id"}}, DoUpdates: clause.AssignmentColumns([]string{"response"})}).Create(&archives).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if proto.Size(gain) == 0 {
		return nil, nil
	}
	return gain, nil
}

// freezeFieldBossDeath73 binds one rule set to a shared generation before its first accepted fight.
func (s *GameService) freezeFieldBossDeath73(tx *gorm.DB, scene string, boss *pb.SideBoss, leader map[string]string) (*model.AdminFieldBossGroup, error) {
	if scene != "field_001" && scene != "field_002" {
		return nil, nil
	}
	var rules model.AdminFieldBossGroup
	if raw := leader["adminDeathRules73"]; raw != "" {
		if err := json.Unmarshal([]byte(raw), &rules); err != nil {
			return nil, err
		}
	}
	if !s.SharedFieldBoss55Ready {
		if rules.DeathSettlement {
			return nil, errors.New("死亡结算需要共享首领初始化")
		}
		return nil, nil
	}
	var shared db.SharedFieldBoss55
	if err := sharedFieldBoss55Query(tx.Clauses(clause.Locking{Strength: "UPDATE"}), s.OriginalServerID(), scene, boss.GetBossPosId()).First(&shared).Error; err != nil {
		return nil, err
	}
	if shared.UUID != boss.GetUuid() {
		return nil, errors.New("共享首领实例已更新")
	}
	if len(shared.DeathRules73) > 0 {
		if err := json.Unmarshal(shared.DeathRules73, &rules); err != nil {
			return nil, err
		}
	} else {
		raw, err := json.Marshal(rules)
		if err != nil {
			return nil, err
		}
		if err = tx.Model(&shared).Update("death_rules73", raw).Error; err != nil {
			return nil, err
		}
	}
	return &rules, nil
}
