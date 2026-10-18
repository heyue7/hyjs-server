package service

import (
	"context"
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

// P13GuildRoomEntryPoint 将原零点请求落到原大厅锚点；明确的合法位置保持原请求。
func (s *GameService) P13GuildRoomEntryPoint(requested *pb.PointNew) (*pb.PointNew, error) {
	if requested != nil && (requested.GetX() != 0 || requested.GetY() != 0 || requested.GetZ() != 0) {
		return requested, nil
	}
	row := s.table("NPC", "npcId", constUtil.P13GuildRoomEntryNPC)
	if row == nil || row["sceneName"] != "union" || row["type"] != "4" || row["pageName"] != "UnionPage" || row["pageOption"] != "tabIndex=0" {
		return nil, errors.New("原公会大厅入口配置缺失或归属错误")
	}
	var coordinates [3]float32
	for i, key := range []string{"bornX", "bornY", "bornZ"} {
		value, err := strconv.ParseFloat(row[key], 32)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("原公会大厅入口坐标无效")
		}
		coordinates[i] = float32(value)
	}
	return &pb.PointNew{X: proto.Float32(coordinates[0]), Y: proto.Float32(coordinates[1]), Z: proto.Float32(coordinates[2])}, nil
}

// P13GuildRoomEnter 只给真实会员开放原town_union领地，既有建筑仍以未创建状态返回。
func (s *GameService) P13GuildRoomEnter(tx *gorm.DB, id int64, p *pb.Player) error {
	m, _, err := p13GuildNeedMember(tx, id)
	if err != nil {
		return err
	}
	if m.GuildID == "" {
		return errors.New("公会领地需要真实会员")
	}
	var cfg db.P13GuildPolicy
	if err = tx.First(&cfg, 1).Error; err != nil {
		return err
	}
	if !cfg.ManagementEnabled {
		return errors.New(constUtil.P13GuildClosed)
	}
	level, err := s.p10Setting("UnionOpenLevel")
	if err != nil {
		return err
	}
	if p.GetLevel() < level {
		return errors.New("公会领地未达到原30级")
	}
	return nil
}
func (s *GameService) P13GuildRoomPresenceScope(tx *gorm.DB, id int64, g *model.GameState) (string, error) {
	if g.Position.GetScene() != constUtil.P13GuildRoomTarget {
		return p10SceneScope(g), nil
	}
	var membership struct{ GuildID string }
	// 真实成员、公会与管理开关一次查询，移动广播不反复扫描迁移元数据。
	err := tx.Table("p13_guild_members AS m").Select("m.guild_id").Joins("JOIN p13_guilds AS g ON g.id=m.guild_id").Joins("JOIN p13_guild_policies AS c ON c.id=1 AND c.management_enabled=1").Where("m.player_id=?", id).Scan(&membership).Error
	if err != nil {
		return "", err
	}
	if membership.GuildID == "" {
		return "", nil
	}
	return constUtil.P13GuildRoomScopePrefix + membership.GuildID, nil
}

// P13GuildRoomNormalize 退会/解散/关闭管理后持久化回主城；原SCRefreshScene会请求SCLoginScene完成切房。
func (s *GameService) P13GuildRoomNormalize(tx *gorm.DB, id int64, p *pb.Player, g *model.GameState) (bool, error) {
	if g.Position.GetScene() != constUtil.P13GuildRoomTarget {
		return false, nil
	}
	scope, err := s.P13GuildRoomPresenceScope(tx, id, g)
	if err != nil {
		return false, err
	}
	if scope != "" {
		return false, nil
	}
	g.Position = &pb.Position{PlayerId: proto.Int64(id), Scene: proto.String("town_main"), Current: &pb.PointNew{X: proto.Float32(0), Y: proto.Float32(0), Z: proto.Float32(0)}}
	g.PositionLoading = true
	return true, nil
}

// P13GuildRoomRevoke 在已持有成员变更角色行锁的事务内保存退场位置，不创建另一人的公会实例。
func (s *GameService) P13GuildRoomRevoke(tx *gorm.DB, id int64) error {
	var row db.Profile
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
		return err
	}
	p, g, err := decodeProfileForCalendarUpdate(tx, &row)
	if err != nil {
		return err
	}
	changed, err := s.P13GuildRoomNormalize(tx, id, p, g)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return saveProfile(tx, &row, p, g)
}

// SyncP13GuildRoom 不依赖世界/野外分线开关，复用原同屏帧但按真实guildId建立专属实例。
func (s *GameService) SyncP13GuildRoom(ctx context.Context, id int64, movement bool) (bool, error) {
	tx := db.GetIns().WithContext(ctx)
	if !tx.Migrator().HasTable(&db.P10ScenePresence{}) {
		return false, nil
	}
	var initial db.Profile
	if tx.First(&initial, id).Error != nil || initial.Game == nil || initial.Player == nil {
		return false, nil
	}
	_, game, err := decodeProfile(&initial)
	if err != nil {
		return false, err
	}
	var old db.P10ScenePresence
	err = tx.First(&old, "player_id=?", id).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	privateOld := strings.HasPrefix(old.Scene, constUtil.P13GuildRoomScopePrefix)
	if game.Position.GetScene() != constUtil.P13GuildRoomTarget && !privateOld {
		return false, nil
	}
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	// 加锁后重新读取当前会话/位置，避免前面的轻量路由观察变成权限快照。
	value, ok := p10ClientSinks.Load(p10ClientKey{s, id})
	if !ok {
		return game.Position.GetScene() == constUtil.P13GuildRoomTarget, nil
	}
	sink := value.(*p10ClientSink)
	profile, err := AuthorizeAccount(ctx, id, sink.session)
	if err != nil {
		return true, err
	}
	p, g, err := decodeProfile(profile)
	if err != nil {
		return true, err
	}
	err = tx.First(&old, "player_id=?", id).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return true, err
	}
	privateOld = strings.HasPrefix(old.Scene, constUtil.P13GuildRoomScopePrefix)
	scope, err := s.P13GuildRoomPresenceScope(tx, id, g)
	if err != nil {
		return true, err
	}
	if g.Position.GetScene() != constUtil.P13GuildRoomTarget || scope == "" {
		if privateOld && old.SessionHash != "" {
			if err = tx.Model(&db.P10ScenePresence{}).Where("player_id=?", id).Update("session_hash", "").Error; err != nil {
				return true, err
			}
			s.p10SceneBroadcast(ctx, old.Scene, old.Line, id, &pb.SCPlayerDisappear{PlayerId: []int64{id}})
		}
		if g.Position.GetScene() == constUtil.P13GuildRoomTarget {
			if err = updateProfileWithTx(ctx, id, func(tx *gorm.DB, p *pb.Player, g *model.GameState) error {
				_, err := s.P13GuildRoomNormalize(tx, id, p, g)
				return err
			}); err != nil {
				return true, err
			}
			s.p10SceneSend(ctx, id, &pb.SCRefreshScene{})
			return true, nil
		}
		return false, nil
	}
	guild, _, err := p13GuildNeedMember(tx, id)
	if err != nil {
		return true, err
	}
	var row db.P13Guild
	if err = tx.First(&row, "id=?", guild.GuildID).Error; err != nil {
		return true, err
	}
	capacity, err := s.p13GuildCapacity(row.Level)
	if err != nil {
		return true, err
	}
	peers, err := s.p10ScenePeers(tx, scope, 1, id)
	if err != nil {
		return true, err
	}
	if len(peers) >= int(capacity) {
		return true, errors.New("实际公会领地人数超过原会员上限")
	}
	frame, err := s.p10SceneMovePlayer(p, g, tx)
	if err != nil {
		return true, err
	}
	raw, err := proto.Marshal(frame)
	if err != nil {
		return true, err
	}
	current := db.P10ScenePresence{PlayerID: id, Scene: scope, Line: 1, SessionHash: p6Digest(sink.session), Frame: raw, UpdatedAt: time.Now().UnixMilli()}
	if err = tx.Save(&current).Error; err != nil {
		return true, err
	}
	changed := old.Scene != scope || old.Line != 1 || old.SessionHash != current.SessionHash
	if changed && old.SessionHash != "" {
		s.p10SceneBroadcast(ctx, old.Scene, old.Line, id, &pb.SCPlayerDisappear{PlayerId: []int64{id}})
	}
	if changed {
		s.p10SceneBroadcast(ctx, scope, 1, id, &pb.SCPlayerAppear{Player: []*pb.MovePlayer{frame}})
		if len(peers) > 0 {
			s.p10SceneSend(ctx, id, &pb.SCPlayerAppear{Player: peers})
		}
	} else if movement {
		s.p10SceneBroadcast(ctx, scope, 1, id, &pb.SCMovePush{Position: []*pb.Position{frame.Position}})
	} else if p6Digest(old.Frame) != p6Digest(current.Frame) {
		s.p10SceneBroadcast(ctx, scope, 1, id, &pb.SCMovePlayerInfo{PlayerInfos: []*pb.MovePlayer{frame}})
	}
	return true, nil
}
func (s *GameService) DisconnectP13GuildRoom(ctx context.Context, id int64, session []byte) bool {
	tx := db.GetIns().WithContext(ctx)
	if !tx.Migrator().HasTable(&db.P10ScenePresence{}) {
		return false
	}
	var row db.P10ScenePresence
	if tx.First(&row, "player_id=? AND session_hash=?", id, p6Digest(session)).Error != nil || !strings.HasPrefix(row.Scene, constUtil.P13GuildRoomScopePrefix) {
		return false
	}
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	if tx.Model(&db.P10ScenePresence{}).Where("player_id=? AND session_hash=?", id, p6Digest(session)).Update("session_hash", "").Error == nil {
		s.p10SceneBroadcast(ctx, row.Scene, row.Line, id, &pb.SCPlayerDisappear{PlayerId: []int64{id}})
	}
	return true
}
func (s *GameService) AfterP13GuildRoomRevoke(ctx context.Context, ids []int64) {
	tx := db.GetIns().WithContext(ctx)
	if !tx.Migrator().HasTable(&db.P10ScenePresence{}) {
		return
	}
	for _, id := range ids {
		var presence db.P10ScenePresence
		if tx.First(&presence, "player_id=?", id).Error != nil || !strings.HasPrefix(presence.Scene, constUtil.P13GuildRoomScopePrefix) {
			continue
		}
		_ = s.SyncP10Scene(ctx, id, false)
		s.p10SceneSend(ctx, id, &pb.SCRefreshScene{})
	}
}

// DispatchP13GuildRoom 提供实际建筑状态；请求者必须正在真实属于本人的公会领地。
func (s *GameService) DispatchP13GuildRoom(ctx context.Context, id int64, request proto.Message) ([]proto.Message, bool, error) {
	if _, ok := request.(*pb.CSRefreshUnionBuilding); !ok {
		return nil, false, nil
	}
	tx := db.GetIns().WithContext(ctx)
	var row db.Profile
	if err := tx.First(&row, id).Error; err != nil {
		return nil, true, err
	}
	p, g, err := decodeProfile(&row)
	if err != nil {
		return nil, true, err
	}
	if g.Position.GetScene() != constUtil.P13GuildRoomTarget {
		return nil, true, errors.New("请先进入本人公会领地")
	}
	if err = s.P13GuildRoomEnter(tx, id, p); err != nil {
		return nil, true, err
	}
	building, err := s.P13GuildBuildingState(tx, id)
	if err != nil {
		return nil, true, err
	}
	return []proto.Message{building}, true, nil
}

// P13GuildRoomSceneConfig 领地仅有本人公会实例的一条线路，公共场景仍使用原P10分线政策。
func (s *GameService) P13GuildRoomSceneConfig(tx *gorm.DB, id int64) (*db.P10ScenePolicy, error) {
	var profile db.Profile
	if err := tx.First(&profile, id).Error; err != nil {
		return nil, err
	}
	if profile.Player == nil || profile.Game == nil {
		return p10SceneConfig(tx)
	}
	_, g, err := decodeProfile(&profile)
	if err != nil {
		return nil, err
	}
	if g.Position.GetScene() != constUtil.P13GuildRoomTarget {
		return p10SceneConfig(tx)
	}
	scope, err := s.P13GuildRoomPresenceScope(tx, id, g)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		return nil, nil
	}
	m, guild, err := p13GuildNeedMember(tx, id)
	if err != nil {
		return nil, err
	}
	if m.GuildID == "" {
		return nil, nil
	}
	capacity, err := s.p13GuildCapacity(guild.Level)
	if err != nil {
		return nil, err
	}
	return &db.P10ScenePolicy{ID: 1, Enabled: true, LineCount: 1, MaxPlayers: capacity, Source: constUtil.P13GuildRoomSource}, nil
}
