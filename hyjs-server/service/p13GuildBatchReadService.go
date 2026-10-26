package service

import (
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"time"
)

type p13GuildReadPlayer struct {
	Profile db.Profile
	Player  *pb.Player
	Game    *model.GameState
	Power   int32
}

// p13GuildReadPlayers captures bounded list dependencies within the caller's transaction.
// No projection is persisted or shared with another request or player.
func (s *GameService) p13GuildReadPlayers(tx *gorm.DB, ids []int64, now time.Time) (map[int64]*p13GuildReadPlayer, error) {
	if tx == nil || now.UnixMilli() <= 0 {
		return nil, p5Error()
	}
	result := make(map[int64]*p13GuildReadPlayer, len(ids))
	unique := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if !seen[id] {
			seen[id] = true
			unique = append(unique, id)
		}
	}
	if len(unique) == 0 {
		return result, nil
	}
	var profiles []db.Profile
	if err := tx.Select("id", "player", "game", "session").Where("id IN ?", unique).Find(&profiles).Error; err != nil {
		return nil, err
	}
	if len(profiles) != len(unique) {
		return nil, gorm.ErrRecordNotFound
	}
	has := func(table any) bool { return tx.Migrator().HasTable(table) }
	memberSchema, guildSchema := has(&db.P13GuildMember{}), has(&db.P13Guild{})
	prayerSchema := has(&db.P13GuildPrayerRoll{})
	offerSchema, upgradeSchema := has(&db.P13GuildTechOffer{}), has(&db.P13GuildTechUpgrade{})
	prayerEnabled := memberSchema && guildSchema && prayerSchema
	techEnabled := memberSchema && guildSchema && offerSchema && upgradeSchema
	members := map[int64]db.P13GuildMember{}
	guilds := map[string]db.P13Guild{}
	offers := map[string]db.P13GuildTechOffer{}
	upgrades := map[string][]db.P13GuildTechUpgrade{}
	prayers := map[int64]db.P13GuildPrayerRoll{}
	if prayerEnabled || techEnabled {
		var rows []db.P13GuildMember
		if err := tx.Where("player_id IN ?", unique).Find(&rows).Error; err != nil {
			return nil, err
		}
		guildIDs := []string{}
		seenGuilds := map[string]bool{}
		for _, row := range rows {
			members[row.PlayerID] = row
			if row.GuildID != "" && !seenGuilds[row.GuildID] {
				seenGuilds[row.GuildID] = true
				guildIDs = append(guildIDs, row.GuildID)
			}
		}
		if len(guildIDs) > 0 {
			var rows []db.P13Guild
			if err := tx.Where("id IN ?", guildIDs).Find(&rows).Error; err != nil {
				return nil, err
			}
			for _, row := range rows {
				guilds[row.ID] = row
			}
			if techEnabled {
				var rows []db.P13GuildTechOffer
				if err := tx.Where("guild_id IN ?", guildIDs).Find(&rows).Error; err != nil {
					return nil, err
				}
				for _, row := range rows {
					offers[row.GuildID] = row
				}
				var steps []db.P13GuildTechUpgrade
				if err := tx.Where("guild_id IN ?", guildIDs).Order("guild_id,tech_type,level").Find(&steps).Error; err != nil {
					return nil, err
				}
				for _, row := range steps {
					upgrades[row.GuildID] = append(upgrades[row.GuildID], row)
				}
			}
		}
		if prayerEnabled {
			var rows []db.P13GuildPrayerRoll
			ranked := tx.Model(&db.P13GuildPrayerRoll{}).Select("*, ROW_NUMBER() OVER (PARTITION BY player_id ORDER BY applied_at DESC, ordinal DESC, stamp DESC) AS prayer_rank").Where("player_id IN ?", unique)
			if calendar75Adjusted(tx) {
				ranked = ranked.Where("applied_at<=?", now.UnixMilli())
			}
			if err := tx.Table("(?) AS latest_prayers", ranked).Where("prayer_rank = 1").Find(&rows).Error; err != nil {
				return nil, err
			}
			for _, row := range rows {
				prayers[row.PlayerID] = row
			}
		}
	}
	type techSource struct {
		prop *pb.BaseProp
		hash string
	}
	techs := map[string]techSource{}
	for i := range profiles {
		profile := profiles[i]
		player, game, err := decodeProfile(&profile)
		if err != nil {
			return nil, err
		}
		projection := &model.P5GuildPrayerProjection{PlayerID: player.GetPlayerId(), CapturedAt: now.UnixMilli(), Properties: &pb.BaseProp{}, TechProperties: &pb.BaseProp{}}
		member := members[profile.Id]
		guild, live := guilds[member.GuildID]
		if live {
			if row, ok := prayers[profile.Id]; prayerEnabled && ok {
				projection.Properties, projection.SourceHash, projection.ExpiresAt, err = p13GuildPrayerPropertiesRead(&row, &member, now)
				if err != nil {
					return nil, err
				}
			}
			if offer, ok := offers[member.GuildID]; techEnabled && ok {
				source, computed := techs[member.GuildID]
				if !computed {
					source.prop, source.hash, err = p13GuildTechPropertiesRead(&guild, &offer, upgrades[member.GuildID], calendar75SourceHorizon(tx, now))
					if err != nil {
						return nil, err
					}
					techs[member.GuildID] = source
				}
				projection.TechProperties, projection.TechSourceHash = proto.Clone(source.prop).(*pb.BaseProp), source.hash
			}
		}
		if err := p5GuildPrayerProjectionValid(projection, player.GetPlayerId()); err != nil {
			return nil, err
		}
		if err := p5GuildTechProjectionValid(projection, player.GetPlayerId()); err != nil {
			return nil, err
		}
		game.GuildPrayerProjection = projection
		power, err := s.formationFightPower(player, game)
		if err != nil {
			return nil, err
		}
		result[profile.Id] = &p13GuildReadPlayer{Profile: profile, Player: player, Game: game, Power: power}
	}
	return result, nil
}
