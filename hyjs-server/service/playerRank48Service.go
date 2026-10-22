package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// playerRank48CopyStars decodes only saved mission stars and counts each mission once.
func playerRank48CopyStars(raw string) (int32, error) {
	var missions []*pb.CopySimpleInfo
	if raw != "" && raw != "null" {
		if err := json.Unmarshal([]byte(raw), &missions); err != nil {
			return 0, err
		}
	}
	stars := map[[2]string]int32{}
	for _, item := range missions {
		key := [2]string{item.GetChapterId(), item.GetMissionId()}
		if key[0] != "" && key[1] != "" {
			stars[key] = max(stars[key], item.GetStar())
		}
	}
	var total int64
	for _, star := range stars {
		total += int64(star)
	}
	return int32(min(total, math.MaxInt32)), nil
}

type playerRank48Projection struct {
	ID         int64
	Name       string
	HeroWarID  string
	Level      int32
	FightPower int32
	CopyStars  string
}

// playerRank48ProjectionSQL keeps profile reads bounded to public scalars and optional mission stars.
func playerRank48ProjectionSQL(tx *gorm.DB, copyStars, identityOnly bool) string {
	scalar := func(path string) string {
		if tx.Dialector.Name() == "mysql" {
			return fmt.Sprintf("NULLIF(JSON_UNQUOTE(JSON_EXTRACT(player, '%s')), 'null')", path)
		}
		return fmt.Sprintf("json_extract(player, '%s')", path)
	}
	integer := "INTEGER"
	if tx.Dialector.Name() == "mysql" {
		integer = "SIGNED"
	}
	fields := "id, COALESCE(" + scalar("$.name") + ", '') AS name, COALESCE(" + scalar("$.heroWarId") + ", '') AS hero_war_id"
	if identityOnly {
		return fields
	}
	fields += ", CAST(COALESCE(" + scalar("$.level") + ", '0') AS " + integer + ") AS level, CAST(COALESCE(" + scalar("$.fightPower") + ", '0') AS " + integer + ") AS fight_power"
	if copyStars {
		fields += ", COALESCE(JSON_EXTRACT(game, '$.copy.passedCopyInfo'), '[]') AS copy_stars"
	}
	return fields
}

// playerRank48 reads current same-zone roles; ranking never creates progress or rewards.
func (s *GameService) playerRank48(ctx context.Context, id int64, kind int32, now time.Time) ([]proto.Message, error) {
	if kind < constUtil.PlayerRank48Copy || kind > constUtil.PlayerRank48Rush {
		return nil, errors.New(constUtil.PlayerRank48Invalid)
	}
	result := &pb.RankInfo{Type: proto.Int32(kind), List: []*pb.RankRowInfo{}}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var caller db.Profile
		if err := tx.Select("id", "server_id").First(&caller, id).Error; err != nil {
			return err
		}
		wanted := map[int64]p12WantedStanding{}
		wantedIDs := []int64{}
		isWanted := kind == constUtil.PlayerRank48WantedDamage || kind == constUtil.PlayerRank48WantedPerformance
		if isWanted {
			rows, _, _, _, err := p12WantedDailyTotals(tx, caller.ServerID, id, now)
			if err != nil {
				return err
			}
			for _, row := range rows {
				if kind == constUtil.PlayerRank48WantedDamage && row.Damage > 0 || kind == constUtil.PlayerRank48WantedPerformance && row.Performance > 0 {
					wanted[row.PlayerID] = row
					wantedIDs = append(wantedIDs, row.PlayerID)
				}
			}
			if len(wantedIDs) == 0 {
				return nil
			}
		}
		sort.Slice(wantedIDs, func(i, j int) bool { return wantedIDs[i] < wantedIDs[j] })
		var lastID int64
		wantedOffset := 0
		for {
			var profiles []playerRank48Projection
			query := tx.Table("profiles").Select(playerRank48ProjectionSQL(tx, kind == constUtil.PlayerRank48Copy, isWanted)).Where("server_id=? AND id>? AND player IS NOT NULL AND game IS NOT NULL", caller.ServerID, lastID)
			if isWanted {
				if wantedOffset >= len(wantedIDs) {
					break
				}
				end := min(wantedOffset+constUtil.PlayerRank48ReadPageSize, len(wantedIDs))
				query = query.Where("id IN ?", wantedIDs[wantedOffset:end])
				wantedOffset = end
			}
			if err := query.Order("id").Limit(constUtil.PlayerRank48ReadPageSize).Scan(&profiles).Error; err != nil {
				return err
			}
			if len(profiles) == 0 {
				if isWanted {
					continue
				}
				break
			}
			lastID = profiles[len(profiles)-1].ID
			ids := make([]int64, 0, len(profiles))
			for _, row := range profiles {
				if row.Name != "" && row.HeroWarID != "" {
					ids = append(ids, row.ID)
				}
			}
			names := map[int64]string{}
			if len(ids) > 0 {
				var memberships []struct {
					PlayerID int64
					Name     string
				}
				if err := tx.Table("p13_guild_members AS m").Select("m.player_id,g.name").Joins("JOIN p13_guilds AS g ON g.id=m.guild_id").Where("m.player_id IN ?", ids).Scan(&memberships).Error; err != nil {
					return err
				}
				for _, member := range memberships {
					names[member.PlayerID] = member.Name
				}
			}
			rush := map[int64]int32{}
			if kind == constUtil.PlayerRank48Rush && len(ids) > 0 {
				var states []db.P7RushState
				if err := tx.Select("player_id", "record_stars").Where("player_id IN ?", ids).Find(&states).Error; err != nil {
					return err
				}
				for _, state := range states {
					rush[state.PlayerID] = state.RecordStars
				}
			}
			for _, row := range profiles {
				if row.Name == "" || row.HeroWarID == "" {
					continue
				}
				item := &pb.RankRowInfo{Pid: proto.Int64(row.ID), Name: proto.String(row.Name), UnionName: proto.String(names[row.ID])}
				switch kind {
				case constUtil.PlayerRank48Level:
					item.Values = []int32{row.Level}
				case constUtil.PlayerRank48Power:
					item.Values = []int32{row.FightPower}
				case constUtil.PlayerRank48Copy:
					stars, err := playerRank48CopyStars(row.CopyStars)
					if err != nil {
						return err
					}
					item.Values = []int32{stars, row.FightPower}
				case constUtil.PlayerRank48Rush:
					item.Values = []int32{rush[row.ID], row.FightPower}
				case constUtil.PlayerRank48WantedDamage:
					item.Values = []int32{int32(min(wanted[row.ID].Damage, float64(math.MaxInt32)))}
				case constUtil.PlayerRank48WantedPerformance:
					item.Values = []int32{int32(min(wanted[row.ID].Performance, math.MaxInt32))}
				}
				result.List = append(result.List, item)
			}
			if !isWanted && len(profiles) < constUtil.PlayerRank48ReadPageSize {
				break
			}
		}
		sort.Slice(result.List, func(i, j int) bool {
			a, b := result.List[i], result.List[j]
			// Preserve P12's full precision ordering before int32 display conversion.
			if kind == constUtil.PlayerRank48WantedDamage && wanted[a.GetPid()].Damage != wanted[b.GetPid()].Damage {
				return wanted[a.GetPid()].Damage > wanted[b.GetPid()].Damage
			}
			if kind == constUtil.PlayerRank48WantedPerformance && wanted[a.GetPid()].Performance != wanted[b.GetPid()].Performance {
				return wanted[a.GetPid()].Performance > wanted[b.GetPid()].Performance
			}
			if a.Values[0] != b.Values[0] {
				return a.Values[0] > b.Values[0]
			}
			return a.GetPid() < b.GetPid()
		})
		for i, item := range result.List {
			item.Rank = proto.Int32(int32(i + 1))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{&pb.SCRank{RankIno: result}}, nil
}
