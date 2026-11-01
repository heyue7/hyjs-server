package service

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func (s *GameService) p10SceneTitle(game *model.GameState, now time.Time) string {
	id := p5KV(game, "p5.current-title")
	if id == "" || s.table("Title", "titleId", id) == nil {
		return ""
	}
	var title p5TitleState
	if json.Unmarshal([]byte(p5KV(game, "p5.title:"+id)), &title) != nil || title.Source == "" || title.DeadTime > 0 && title.DeadTime <= now.UnixMilli() {
		return ""
	}
	return id
}

func (s *GameService) p10SceneGuildName(tx *gorm.DB, playerID int64) (string, error) {
	if !s.p10ClientOwner().guildCoreSchemaReady.Load() && (!gameSchemaHasTable(tx, &db.P13GuildMember{}) || !gameSchemaHasTable(tx, &db.P13Guild{})) {
		return "", nil
	}
	var row struct{ Name string }
	err := tx.Table("p13_guild_members AS gm").Select("g.name AS name").Joins("JOIN p13_guilds AS g ON g.id=gm.guild_id").Where("gm.player_id=? AND gm.guild_id<>?", playerID, "").Scan(&row).Error
	return row.Name, err
}

// 仅在进入、过期或自己的公会操作后查询；移动路径复用当前连接的值。
func (s *GameService) p10SceneCachedGuildName(tx *gorm.DB, sink *p10ClientSink, playerID int64, force bool) (string, error) {
	now := time.Now()
	if !force && now.Before(sink.sceneGuildUntil) {
		return sink.sceneGuildName, nil
	}
	name, err := s.p10SceneGuildName(tx, playerID)
	if err != nil {
		return "", err
	}
	sink.sceneGuildName, sink.sceneGuildUntil = name, now.Add(constUtil.P10SceneGuildRefreshInterval)
	return name, nil
}

func p10SceneGuildRefreshRequest(request any) bool {
	switch request.(type) {
	case *pb.CSUnionCreate, *pb.CSUnionApply, *pb.CSUnionAgreeApply, *pb.CSQuitUnion, *pb.CSDissolveUnion, *pb.CSRemoveUnionPlayer, *pb.CSGetUnionInfo:
		return true
	}
	return false
}
