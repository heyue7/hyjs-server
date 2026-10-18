package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

func p13UFWeek(start time.Time) string {
	offset := (int(start.Weekday()) + 6) % 7
	return start.AddDate(0, 0, -offset).Format("2006-01-02")
}

// p13UFGroupCounts 读取原15/30/30/25比例；不足整人数采用具名本地最大余数法且保留所有公会。
func (s *GameService) p13UFGroupCounts(count int) ([4]int, error) {
	var result [4]int
	raw := s.table("GameConfigure", "StaticName", "uFGroupConfig")["NameValue"]
	pieces := strings.Split(raw, ",")
	if len(pieces) != 4 {
		return result, errors.New("原四组比例配置无效")
	}
	type fraction struct{ index, remainder int }
	var fractions []fraction
	total, assigned := 0, 0
	for i, text := range pieces {
		ratio, err := strconv.Atoi(text)
		if err != nil || ratio < 0 || ratio > 100 {
			return result, errors.New("原四组比例配置无效")
		}
		total += ratio
		result[i] = count * ratio / 100
		assigned += result[i]
		fractions = append(fractions, fraction{i, count * ratio % 100})
	}
	if total != 100 {
		return result, errors.New("原四组比例总和无效")
	}
	sort.SliceStable(fractions, func(i, j int) bool { return fractions[i].remainder > fractions[j].remainder })
	for i := 0; i < count-assigned; i++ {
		result[fractions[i%4].index]++
	}
	return result, nil
}
func (s *GameService) p13UFGuildPreview(tx *gorm.DB, guild db.P13Guild, power int64) ([]byte, error) {
	var chairman db.P13GuildMember
	if err := tx.First(&chairman, "guild_id=? AND role=?", guild.ID, constUtil.P13GuildChairman).Error; err != nil {
		return nil, err
	}
	var profile db.Profile
	if err := tx.First(&profile, chairman.PlayerID).Error; err != nil {
		return nil, err
	}
	player, _, err := decodeProfile(&profile)
	if err != nil {
		return nil, err
	}
	icon := s.table("UnionLogo", "id", strconv.Itoa(int(guild.Flag)))["iconName"]
	if icon == "" {
		return nil, errors.New("公会旗帜原资源缺失")
	}
	return proto.Marshal(&pb.UFInfoPreView{UnionId: proto.String(guild.ID), UnionName: proto.String(guild.Name), ServerId: proto.Int32(int32(s.OriginalServerID())), FightPower: proto.Int64(power), MasterName: player.Name, UnionLogo: proto.String(icon), UnionPrestige: proto.Int32(0)})
}

type p13UFMatchCandidate struct {
	entry    db.P13UnionFightEntry
	guild    db.P13Guild
	fighters []db.P13UnionFightFighter
	power    int64
	group    string
}

// p13UFMatchFighter 显式存储内部Battle字节；DB模型json:"-"不得把真实阵容从比赛快照中省略。
type p13UFMatchFighter struct {
	db.P13UnionFightFighter
	BattlePayload []byte `json:"battlePayload"`
}

type p13UFMatchRoster struct {
	Left  []p13UFMatchFighter
	Right []p13UFMatchFighter
}

// p13UFReadMatchFighter 旧实例按其已保存的SHA读取冻结归档，只恢复同版本字节，禁止从现在角色重造。
func p13UFReadMatchFighter(tx *gorm.DB, item p13UFMatchFighter) (*db.P13UnionFightFighter, error) {
	fighter := item.P13UnionFightFighter
	fighter.Battle = append([]byte(nil), item.BattlePayload...)
	if len(fighter.Battle) == 0 {
		var archived db.P13UnionFightFighter
		if err := tx.First(&archived, "guild_id=? AND day=? AND player_id=?", fighter.GuildID, fighter.Day, fighter.PlayerID).Error; err != nil {
			return nil, err
		}
		var entry db.P13UnionFightEntry
		if err := tx.First(&entry, "guild_id=? AND day=? AND frozen_at>0", fighter.GuildID, fighter.Day).Error; err != nil {
			return nil, err
		}
		if archived.SnapshotHash != fighter.SnapshotHash {
			return nil, errors.New("公会战旧实例冻结归档版本不一致")
		}
		fighter.Battle = append([]byte(nil), archived.Battle...)
	}
	if p6Digest(fighter.Battle) != fighter.SnapshotHash {
		return nil, errors.New("公会战匹配冻结Battle字节损坏")
	}
	return &fighter, nil
}

// MatchP13UnionFight 只把真实冻结名单与明确日历配成待开赛实例，不使用诊断胜负产生战果。
func (s *GameService) MatchP13UnionFight(ctx context.Context, now time.Time) error {
	start, matchDay, _, _ := p13UFClock(now)
	switch start.Weekday() {
	case time.Tuesday, time.Thursday, time.Saturday:
	default:
		return nil
	}
	registrationDay := start.AddDate(0, 0, -1).Format("2006-01-02")
	week := p13UFWeek(start)
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var gate db.P13GuildPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		var policy db.P13UnionFightPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if !gate.ManagementEnabled || !policy.EnrollmentEnabled || !policy.MatchEnabled {
			return nil
		}
		// 日历为运营发布事实；没有原时刻来源时保留冻结名单，绝不生成假开赛时刻。
		var schedule db.P13UnionFightSchedule
		err := tx.First(&schedule, "day=?", matchDay).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if schedule.FirstStartAt <= start.Add(30*time.Minute).UnixMilli() || schedule.SecondStartAt <= schedule.FirstStartAt || schedule.DurationMillis <= 0 || schedule.DurationMillis > 86400000 || schedule.FirstStartAt > math.MaxInt64-schedule.DurationMillis || schedule.FirstStartAt+schedule.DurationMillis > schedule.SecondStartAt || schedule.SecondStartAt > math.MaxInt64-schedule.DurationMillis || schedule.SecondStartAt+schedule.DurationMillis > start.AddDate(0, 0, 1).UnixMilli() || schedule.Source == "" {
			return errors.New("公会战已发布日历不完整")
		}
		var entries []db.P13UnionFightEntry
		if err = tx.Where("day=? AND frozen_at>0 AND match_processed=0", registrationDay).Find(&entries).Error; err != nil {
			return err
		}
		var candidates []*p13UFMatchCandidate
		for _, entry := range entries {
			if entry.EligibleCount == 0 {
				if err = tx.Model(&entry).Update("match_processed", true).Error; err != nil {
					return err
				}
				continue
			}
			var guild db.P13Guild
			err = tx.First(&guild, "id=?", entry.GuildID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if p6Digest(entry.FrozenTables) != entry.ConfigurationHash || entry.EngineVersion == "" || entry.ClientVersion == "" {
				return errors.New("公会战冻结配置损坏")
			}
			var fighters []db.P13UnionFightFighter
			if err = tx.Where("guild_id=? AND day=? AND applied_at>0", entry.GuildID, registrationDay).Order("player_id").Find(&fighters).Error; err != nil {
				return err
			}
			candidate := &p13UFMatchCandidate{entry: entry, guild: guild}
			for _, fighter := range fighters {
				member, err := p13GuildMember(tx, fighter.PlayerID)
				if err != nil {
					return err
				}
				if member.GuildID != entry.GuildID {
					continue
				}
				if p6Digest(fighter.Battle) != fighter.SnapshotHash || fighter.FightPower < 0 || candidate.power > math.MaxInt64-fighter.FightPower {
					return errors.New("公会战冻结成员阵容或战力无效")
				}
				candidate.power += fighter.FightPower
				candidate.fighters = append(candidate.fighters, fighter)
			}
			if len(candidate.fighters) > 0 {
				candidates = append(candidates, candidate)
			}
		}
		sort.SliceStable(candidates, func(i, j int) bool {
			if candidates[i].power == candidates[j].power {
				return candidates[i].guild.ID < candidates[j].guild.ID
			}
			return candidates[i].power > candidates[j].power
		})
		var fresh []*p13UFMatchCandidate
		for _, candidate := range candidates {
			var group db.P13UnionFightGroup
			err = tx.First(&group, "week_start=? AND guild_id=?", week, candidate.guild.ID).Error
			if err == nil {
				candidate.group = group.Group
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			fresh = append(fresh, candidate)
		}
		counts, err := s.p13UFGroupCounts(len(fresh))
		if err != nil {
			return err
		}
		index := 0
		for group, n := range counts {
			for i := 0; i < n; i++ {
				candidate := fresh[index]
				candidate.group = string(rune('A' + group))
				index++
				if err = tx.Create(&db.P13UnionFightGroup{WeekStart: week, GuildID: candidate.guild.ID, Group: candidate.group, FrozenPower: candidate.power, AssignedAt: now.UnixMilli(), Source: constUtil.P13UnionFightMatchSource}).Error; err != nil {
					return err
				}
			}
		}
		// 目前没有权威历史胜率，使用原冻结总战力邻近配对，历史胜率权重仍作为待恢复范围。
		for _, group := range []string{"A", "B", "C", "D"} {
			var bucket []*p13UFMatchCandidate
			for _, candidate := range candidates {
				if candidate.group == group {
					bucket = append(bucket, candidate)
				}
			}
			for i := 0; i+1 < len(bucket); i += 2 {
				left, right := bucket[i], bucket[i+1]
				if left.entry.ConfigurationHash != right.entry.ConfigurationHash || left.entry.EngineVersion != right.entry.EngineVersion || left.entry.ClientVersion != right.entry.ClientVersion {
					continue
				}
				leftPreview, err := s.p13UFGuildPreview(tx, left.guild, left.power)
				if err != nil {
					return err
				}
				rightPreview, err := s.p13UFGuildPreview(tx, right.guild, right.power)
				if err != nil {
					return err
				}
				payload := p13UFMatchRoster{}
				for _, fighter := range left.fighters {
					payload.Left = append(payload.Left, p13UFMatchFighter{P13UnionFightFighter: fighter, BattlePayload: append([]byte(nil), fighter.Battle...)})
				}
				for _, fighter := range right.fighters {
					payload.Right = append(payload.Right, p13UFMatchFighter{P13UnionFightFighter: fighter, BattlePayload: append([]byte(nil), fighter.Battle...)})
				}
				roster, err := json.Marshal(payload)
				if err != nil {
					return err
				}
				for round, at := range []int64{schedule.FirstStartAt, schedule.SecondStartAt} {
					id := p6Digest([]byte(fmt.Sprintf("p13-match/%s/%s/%s/%d", matchDay, left.guild.ID, right.guild.ID, round+1)))
					record := db.P13UnionFightMatch{ID: id, Day: matchDay, Group: group, Round: int32(round + 1), LeftGuildID: left.guild.ID, RightGuildID: right.guild.ID, StartAt: at, EndAt: at + schedule.DurationMillis, LeftPreview: leftPreview, RightPreview: rightPreview, Roster: roster, SnapshotHash: p6Digest(roster), ConfigurationHash: left.entry.ConfigurationHash, EngineVersion: left.entry.EngineVersion, ClientVersion: left.entry.ClientVersion, CreatedAt: now.UnixMilli(), Source: constUtil.P13UnionFightMatchSource + "；日历：" + schedule.Source}
					if err = tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&record).Error; err != nil {
						return err
					}
				}
			}
		}
		// 单个余数公会仅记录已处理配对批次，继续待对手/轮空规则审计，不颁布轮空胜利。
		for _, candidate := range candidates {
			if err = tx.Model(&candidate.entry).Update("match_processed", true).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *GameService) p13UFMatchPreviews(tx *gorm.DB, guild string, day string, playerID int64) ([]*pb.UFUnionBattlePreView, string, error) {
	var rows []db.P13UnionFightMatch
	if err := tx.Where("day=? AND (left_guild_id=? OR right_guild_id=?)", day, guild, guild).Order("round,id").Find(&rows).Error; err != nil {
		return nil, "", err
	}
	var previews []*pb.UFUnionBattlePreView
	group := ""
	if len(rows) > 2 {
		return nil, "", errors.New("公会战原两轮实例重复")
	}
	for _, row := range rows {
		if p6Digest(row.Roster) != row.SnapshotHash || row.Round != int32(len(previews)+1) || row.StartAt <= 0 || row.EndAt <= row.StartAt || !strings.Contains("ABCD", row.Group) || len(row.Group) != 1 {
			return nil, "", errors.New("公会战匹配名单快照损坏")
		}
		ownRaw, enemyRaw := row.LeftPreview, row.RightPreview
		enemyID := row.RightGuildID
		if row.RightGuildID == guild {
			ownRaw, enemyRaw = enemyRaw, ownRaw
			enemyID = row.LeftGuildID
		}
		own, enemy := &pb.UFInfoPreView{}, &pb.UFInfoPreView{}
		if proto.Unmarshal(ownRaw, own) != nil || proto.Unmarshal(enemyRaw, enemy) != nil || own.GetUnionId() != guild || enemy.GetUnionId() != enemyID || enemyID == guild {
			return nil, "", errors.New("公会战原预览联合身份损坏")
		}
		preview := &pb.UFUnionBattlePreView{OurUnion: own, EnemyUnion: enemy, StartFightTime: proto.Int64(row.StartAt), EndFightTime: proto.Int64(row.EndAt), IsRoundBye: proto.Int32(0)}
		// 原SignUpItem只认识1胜/2负；仅有已保存的真实终局才给出结果。
		var settled db.P13UnionFightSettlementResult
		err := tx.First(&settled, "match_id=?", row.ID).Error
		if err == nil {
			value := int32(2)
			if settled.WinnerGuildID == guild {
				value = 1
			}
			preview.FightResult = proto.Int32(value)
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", err
		}
		var loadout db.P13UnionFightLoadout
		err = tx.First(&loadout, "match_id=? AND player_id=?", row.ID, playerID).Error
		if err == nil {
			if err = json.Unmarshal([]byte(loadout.ItemIDs), &preview.ItemIds); err != nil {
				return nil, "", err
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", err
		}
		previews = append(previews, preview)
		group = row.Group
	}
	return previews, group, nil
}
