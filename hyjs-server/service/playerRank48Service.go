package service

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// playerRank48CopyStars counts each saved mission once without repairing the save.
func playerRank48CopyStars(game *model.GameState) int32 {
	stars := map[[2]string]int32{}
	for _, item := range game.Copy.GetPassedCopyInfo() {
		key := [2]string{item.GetChapterId(), item.GetMissionId()}
		if key[0] != "" && key[1] != "" {
			stars[key] = max(stars[key], item.GetStar())
		}
	}
	var total int64
	for _, star := range stars {
		total += int64(star)
	}
	return int32(min(total, math.MaxInt32))
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
		var profiles []db.Profile
		if err := tx.Select("id", "server_id", "player", "game").Where("server_id=? AND player IS NOT NULL AND game IS NOT NULL", caller.ServerID).Order("id").Find(&profiles).Error; err != nil {
			return err
		}
		ids := make([]int64, 0, len(profiles))
		for _, row := range profiles {
			ids = append(ids, row.Id)
		}
		names := map[int64]string{}
		if len(ids) > 0 {
			var members []db.P13GuildMember
			if err := tx.Where("player_id IN ? AND guild_id<>?", ids, "").Find(&members).Error; err != nil {
				return err
			}
			guildIDs := make([]string, 0, len(members))
			for _, member := range members {
				guildIDs = append(guildIDs, member.GuildID)
			}
			if len(guildIDs) > 0 {
				var guilds []db.P13Guild
				if err := tx.Select("id", "name").Where("id IN ?", guildIDs).Find(&guilds).Error; err != nil {
					return err
				}
				guildNames := map[string]string{}
				for _, guild := range guilds {
					guildNames[guild.ID] = guild.Name
				}
				for _, member := range members {
					names[member.PlayerID] = guildNames[member.GuildID]
				}
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
		wanted := map[int64]p12WantedStanding{}
		if kind == constUtil.PlayerRank48WantedDamage || kind == constUtil.PlayerRank48WantedPerformance {
			rows, _, _, _, err := p12WantedDailyTotals(tx, caller.ServerID, id, now)
			if err != nil {
				return err
			}
			for _, row := range rows {
				wanted[row.PlayerID] = row
			}
		}
		for _, row := range profiles {
			player, game, err := decodeProfile(&row)
			if err != nil {
				return err
			}
			if player.GetName() == "" || player.GetHeroWarId() == "" || len(game.Warriors) == 0 {
				continue
			}
			item := &pb.RankRowInfo{Pid: proto.Int64(row.Id), Name: player.Name, UnionName: proto.String(names[row.Id])}
			switch kind {
			case constUtil.PlayerRank48Level:
				item.Values = []int32{player.GetLevel()}
			case constUtil.PlayerRank48WantedDamage:
				value := wanted[row.Id].Damage
				if value <= 0 {
					continue
				}
				item.Values = []int32{int32(min(value, float64(math.MaxInt32)))}
			case constUtil.PlayerRank48WantedPerformance:
				value := wanted[row.Id].Performance
				if value <= 0 {
					continue
				}
				item.Values = []int32{int32(min(value, math.MaxInt32))}
			default:
				if err := s.PrepareP5GuildPrayerProperties(ctx, tx, player, game, now); err != nil {
					return err
				}
				power, err := s.formationFightPower(player, game)
				if err != nil {
					return err
				}
				switch kind {
				case constUtil.PlayerRank48Power:
					item.Values = []int32{power}
				case constUtil.PlayerRank48Copy:
					item.Values = []int32{playerRank48CopyStars(game), power}
				case constUtil.PlayerRank48Rush:
					item.Values = []int32{rush[row.Id], power}
				}
			}
			result.List = append(result.List, item)
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
