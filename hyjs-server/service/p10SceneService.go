package service

import (
	"bytes"
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

var p10SceneMutation sync.Mutex

func (s *GameService) InitializeP10Scene(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P10ScenePolicy{}, &db.P10ScenePresence{}); err != nil {
		return err
	}
	cfg := db.P10ScenePolicy{ID: constUtil.P10ScenePolicyID, LineCount: 2, MaxPlayers: 100, Source: constUtil.P10SceneSource}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&cfg).Error
}
func p10SceneConfig(tx *gorm.DB) (*db.P10ScenePolicy, error) {
	if !gameSchemaHasTable(tx, &db.P10ScenePolicy{}) {
		return nil, nil
	}
	var cfg db.P10ScenePolicy
	if err := tx.First(&cfg, constUtil.P10ScenePolicyID).Error; err != nil {
		return nil, err
	}
	if cfg.LineCount < 1 || cfg.LineCount > 32 || cfg.MaxPlayers < 1 || cfg.MaxPlayers > 1000 || cfg.Source == "" {
		return nil, errors.New("有限分线运营参数无效")
	}
	if !cfg.Enabled {
		return nil, nil
	}
	return &cfg, nil
}
func p10SceneScope(g *model.GameState) string {
	if g.Run != nil && !g.Run.Complete || g.Field != nil && g.Field.Fight != nil && !g.Field.Fight.Settled {
		return ""
	}
	switch g.Position.GetScene() {
	case "town", "town_main":
		return "town_main"
	case "field_001", "field_002":
		if !g.PositionLoading {
			return g.Position.GetScene()
		}
	case constUtil.GGOFieldScene:
		// A V4 terminal proves this character entered the published desert path.
		if g.GGOField != nil && !g.PositionLoading {
			return g.Position.GetScene()
		}
	}
	return ""
}
func (s *GameService) p10SceneMovePlayer(p *pb.Player, g *model.GameState, projectionTx ...*gorm.DB) (*pb.MovePlayer, error) {
	if len(projectionTx) == 1 {
		tx := projectionTx[0]
		if err := s.PrepareP5GuildPrayerProperties(tx.Statement.Context, tx, p, g, p13UFNow(tx.Statement.Context)); err != nil {
			return nil, err
		}
	}
	info, err := s.battlePlayer(p, g)
	if err != nil {
		return nil, err
	}
	power, err := s.formationFightPowerFromParty(info)
	if err != nil {
		return nil, err
	}
	heroID, err := P5WorldHeroID(p, g)
	if err != nil {
		return nil, err
	}
	weaponID := ""
	awake := int32(0)
	for _, hero := range info.GetFormation() {
		if hero.GetId() == heroID {
			weaponID = hero.GetWeaponId()
			awake = hero.GetAwakeStage()
			break
		}
	}
	pet, err := s.P9FollowPetInfo(g)
	if err != nil {
		return nil, err
	}
	var side *pb.PlayerSide
	fieldScope := p10SceneScope(g)
	if g.Field != nil && g.Field.Side != nil && (fieldScope == "field_001" || fieldScope == "field_002" || fieldScope == constUtil.GGOFieldScene) && g.Field.Scene == fieldScope {
		side = proto.Clone(g.Field.Side).(*pb.PlayerSide)
	} else {
		side = &pb.PlayerSide{PlayerId: p.PlayerId}
	}
	if s.P12FieldPVPReady {
		if prison, ok := P12FieldPVPPrisonSide(p, g, time.Now()); ok {
			side = prison
		}
	}
	if s.P12FieldPVPReady {
		tx := db.GetIns()
		if len(projectionTx) == 1 {
			tx = projectionTx[0]
		}
		side, err = s.ProjectP12FieldPVPSide(tx, p.GetPlayerId(), side, LocalPlayNow(tx.Statement.Context))
		if err != nil {
			return nil, err
		}
	}
	// 同屏身份沿可信启动区服，保持与公开资料/组队的实际归属一致。
	position := proto.Clone(g.Position).(*pb.Position)
	if constUtil.P8OriginalFieldScene(fieldScope) {
		// RoomNetBase.SolveRoleAppear accepts town_side_<StaticData.fieldId>.
		// Keep the persisted scene and line scope canonical; only project the wire frame.
		position.Scene = proto.String(constUtil.P10FieldPlayerScenePrefix + fieldScope)
	}
	return &pb.MovePlayer{Position: position, Info: &pb.MovePlayerInfo{ServerId: proto.Int32(int32(s.OriginalServerID())), PlayerId: p.PlayerId, PlayerName: p.Name, Level: p.Level, WarId: proto.String(heroID), EquipId: proto.String(weaponID), Power: proto.Int64(int64(power)), PlayerSide: side, ServerName: proto.String("本地测试"), AwakeStage: proto.Int32(awake), PetId: proto.String(pet.GetPetId())}}, nil
}

// p10CurrentScenePeers restores actors cleared by the client during room reload,
// including stationary peers whose presence identity has not changed.
func (s *GameService) p10CurrentScenePeers(ctx context.Context, id int64) ([]*pb.MovePlayer, error) {
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	tx := db.GetIns().WithContext(ctx)
	cfg, err := p10SceneConfig(tx)
	if err != nil || cfg == nil {
		return nil, err
	}
	var own db.P10ScenePresence
	err = tx.First(&own, "player_id=?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if own.Scene != "town_main" && !constUtil.P8OriginalFieldScene(own.Scene) {
		return nil, nil
	}
	if _, live := s.p10SceneLive(tx, own); !live {
		return nil, nil
	}
	return s.p10ScenePeers(tx, own.Scene, own.Line, id)
}

// p10SceneLiveRows reads each current profile and offline account by indexed ID
// in bounded batches. The directory lives only for this call; each invocation
// reads the current session, scene and account state again.
func (s *GameService) p10SceneLiveRows(tx *gorm.DB, rows []db.P10ScenePresence) ([]*pb.MovePlayer, error) {
	type candidate struct {
		row     db.P10ScenePresence
		sink    *p10ClientSink
		offline bool
	}
	var candidates []candidate
	var ids []int64
	for _, row := range rows {
		value, ok := p10LiveClient(p10ClientKey{s.p10ClientOwner(), row.PlayerID})
		offline := !ok && row.Scene == constUtil.GGOFieldScene && row.SessionHash == "" && row.OfflineUntil > time.Now().UnixMilli()
		if !offline && (!ok || row.SessionHash == "") {
			continue
		}
		entry := candidate{row: row, offline: offline}
		if !offline {
			entry.sink = value.(*p10ClientSink)
		}
		candidates = append(candidates, entry)
		ids = append(ids, row.PlayerID)
	}
	if len(ids) == 0 {
		return nil, nil
	}
	profiles := make(map[int64]db.Profile, len(ids))
	// Limit IN parameters for both SQLite and MySQL, including stale directories.
	const batchSize = 400
	for start := 0; start < len(ids); start += batchSize {
		end := min(start+batchSize, len(ids))
		var batch []db.Profile
		if err := tx.Select("id", "session", "account_id", "account_auth_revision", "player", "game", "ggo_credit", "ggo_credit_migrated").Where("id IN ?", ids[start:end]).Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, profile := range batch {
			profiles[profile.Id] = profile
		}
	}
	var accountIDs []int64
	accountSeen := make(map[int64]bool)
	for _, entry := range candidates {
		profile, exists := profiles[entry.row.PlayerID]
		if entry.offline && exists && profile.AccountID > 0 && !accountSeen[profile.AccountID] {
			accountSeen[profile.AccountID] = true
			accountIDs = append(accountIDs, profile.AccountID)
		}
	}
	accounts := make(map[int64]db.GameAccount, len(accountIDs))
	for start := 0; start < len(accountIDs); start += batchSize {
		end := min(start+batchSize, len(accountIDs))
		var batch []db.GameAccount
		if err := db.AccountsTable(tx, "game_accounts").Select("id", "enabled", "auth_revision").Where("id IN ?", accountIDs[start:end]).Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, account := range batch {
			accounts[account.ID] = account
		}
	}
	var peers []*pb.MovePlayer
	for _, entry := range candidates {
		row := entry.row
		profile, exists := profiles[row.PlayerID]
		if !exists {
			continue
		}
		if !entry.offline {
			if subtle.ConstantTimeCompare(profile.Session, entry.sink.session) != 1 || row.SessionHash != p6Digest(entry.sink.session) {
				continue
			}
		} else if row.OfflineUntil <= time.Now().UnixMilli() {
			continue
		} else if profile.AccountID > 0 {
			account, exists := accounts[profile.AccountID]
			if !exists || !account.Enabled || account.AuthRevision != profile.AccountAuthRevision {
				continue
			}
		}
		_, game, err := decodeProfile(&profile)
		if err != nil {
			continue
		}
		if entry.offline && (game.GGOField == nil || game.Position.GetScene() != constUtil.GGOFieldScene || game.PositionLoading) {
			continue
		}
		expected, scopeErr := s.P13GuildRoomPresenceScope(tx, row.PlayerID, game)
		if scopeErr != nil || expected != row.Scene {
			continue
		}
		frame := &pb.MovePlayer{}
		if proto.Unmarshal(row.Frame, frame) != nil || frame.GetInfo().GetPlayerId() != row.PlayerID {
			continue
		}
		peers = append(peers, frame)
	}
	return peers, nil
}

func (s *GameService) p10SceneLive(tx *gorm.DB, row db.P10ScenePresence) (*pb.MovePlayer, bool) {
	peers, err := s.p10SceneLiveRows(tx, []db.P10ScenePresence{row})
	if err != nil || len(peers) != 1 {
		return nil, false
	}
	return peers[0], true
}
func (s *GameService) p10ScenePeers(tx *gorm.DB, scene string, line int32, exclude int64) ([]*pb.MovePlayer, error) {
	var rows []db.P10ScenePresence
	base := func() *gorm.DB { return tx.Where("scene=? AND line=? AND player_id<>?", scene, line, exclude) }
	if err := base().Where("session_hash<>?", "").Order("player_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	if scene == constUtil.GGOFieldScene {
		var offline []db.P10ScenePresence
		if err := base().Where("session_hash=? AND offline_until>?", "", time.Now().UnixMilli()).Order("player_id").Find(&offline).Error; err != nil {
			return nil, err
		}
		rows = append(rows, offline...)
	}
	return s.p10SceneLiveRows(tx, rows)
}

func (s *GameService) p10SceneSend(ctx context.Context, id int64, message proto.Message) {
	s.publishP10Private(ctx, id, message)
}
func (s *GameService) p10SceneBroadcast(ctx context.Context, scene string, line int32, exclude int64, message proto.Message) {
	peers, err := s.p10ScenePeers(db.GetIns().WithContext(ctx), scene, line, exclude)
	if err != nil {
		return
	}
	s.p10SceneSendPeers(ctx, peers, message)
}
func (s *GameService) p10SceneSendPeers(ctx context.Context, peers []*pb.MovePlayer, message proto.Message) {
	for _, peer := range peers {
		// Sending still rechecks the current account, session and client-update gate.
		s.p10SceneSend(ctx, peer.GetInfo().GetPlayerId(), message)
	}
}

// SyncP10Scene 必须在公共位置事务提交后调用。它从当前真实TCP会话和profile恢复同屏，不造离线玩家。
func (s *GameService) SyncP10Scene(ctx context.Context, id int64, movement bool) error {
	if handled, err := s.SyncP13UnionFightField(ctx, id, movement); handled || err != nil {
		return err
	}
	if handled, err := s.SyncP13GuildRoom(ctx, id, movement); handled || err != nil {
		return err
	}
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	tx := db.GetIns().WithContext(ctx)
	cfg, err := p10SceneConfig(tx)
	if err != nil || cfg == nil {
		return err
	}
	value, ok := p10LiveClient(p10ClientKey{s.p10ClientOwner(), id})
	if !ok {
		return nil
	}
	sink := value.(*p10ClientSink)
	profile, err := AuthorizeAccount(ctx, id, sink.session)
	if err != nil {
		return err
	}
	p, g, err := decodeProfile(profile)
	if err != nil {
		return err
	}
	var old db.P10ScenePresence
	err = tx.Where("player_id=?", id).First(&old).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	existed := err == nil
	scope := p10SceneScope(g)
	if scope == "" {
		if old.SessionHash != "" {
			if err = tx.Model(&db.P10ScenePresence{}).Where("player_id=?", id).Update("session_hash", "").Error; err != nil {
				return err
			}
			s.p10SceneBroadcast(ctx, old.Scene, old.Line, id, &pb.SCPlayerDisappear{PlayerId: []int64{id}})
		}
		return nil
	}
	// Per-map capacity shares the original line protocol and preserves stable socket ownership.
	cfg, err = s.adminFieldSceneConfig(tx, scope, cfg)
	if err != nil {
		return err
	}
	line := old.Line
	if line < 1 || line > cfg.LineCount {
		line = 1
	}
	linePeers := make(map[int32][]*pb.MovePlayer)
	loadLine := func(candidate int32) ([]*pb.MovePlayer, error) {
		if peers, ok := linePeers[candidate]; ok {
			return peers, nil
		}
		peers, err := s.p10ScenePeers(tx, scope, candidate, id)
		if err == nil {
			linePeers[candidate] = peers
		}
		return peers, err
	}
	peers, err := loadLine(line)
	if err != nil {
		return err
	}
	// New desert arrivals prefer a line below 16 players, preserving the hard 20 cap.
	if scope == constUtil.GGOFieldScene && old.Scene != scope && len(peers) >= 16 {
		bestLine, bestPeers := line, peers
		for candidate := int32(1); candidate <= cfg.LineCount; candidate++ {
			if candidate == line {
				continue
			}
			available, e := loadLine(candidate)
			if e != nil {
				return e
			}
			if len(available) < len(bestPeers) {
				bestLine, bestPeers = candidate, available
			}
			if len(bestPeers) < 16 {
				break
			}
		}
		line, peers = bestLine, bestPeers
	}
	if len(peers) >= int(cfg.MaxPlayers) {
		if scope == constUtil.GGOFieldScene && old.Scene == scope {
			return errors.New("原沙漠线路已满，请稍后重连")
		}
		found := false
		for candidate := int32(1); candidate <= cfg.LineCount; candidate++ {
			available, err := loadLine(candidate)
			if err != nil {
				return err
			}
			if len(available) < int(cfg.MaxPlayers) {
				line = candidate
				peers = available
				found = true
				break
			}
		}
		if !found {
			if old.SessionHash != "" && old.Scene != scope {
				if err = tx.Model(&db.P10ScenePresence{}).Where("player_id=?", id).Update("session_hash", "").Error; err != nil {
					return err
				}
				s.p10SceneBroadcast(ctx, old.Scene, old.Line, id, &pb.SCPlayerDisappear{PlayerId: []int64{id}})
			}
			return errors.New("所有有限分线已满")
		}
	}
	frame, err := s.p10SceneMovePlayer(p, g, tx)
	if err != nil {
		return err
	}
	raw, err := proto.Marshal(frame)
	if err != nil {
		return err
	}
	current := db.P10ScenePresence{PlayerID: id, Scene: scope, Line: line, SessionHash: p6Digest(sink.session), Frame: raw, UpdatedAt: time.Now().UnixMilli()}
	changed := old.Scene != scope || old.Line != line || old.SessionHash != current.SessionHash
	wrote := !existed
	if !existed {
		if err = tx.Save(&current).Error; err != nil {
			return err
		}
	} else {
		updates := make(map[string]any)
		if old.Scene != current.Scene {
			updates["scene"] = current.Scene
		}
		if old.Line != current.Line {
			updates["line"] = current.Line
		}
		if old.SessionHash != current.SessionHash {
			updates["session_hash"] = current.SessionHash
		}
		if !bytes.Equal(old.Frame, current.Frame) {
			updates["frame"] = current.Frame
		}
		if old.OfflineUntil != 0 {
			updates["offline_until"] = 0
		}
		if old.OfflinePKUsed {
			updates["offline_pk_used"] = false
		}
		// Return-sprout eligibility uses this exact last-active timestamp.
		if old.UpdatedAt != current.UpdatedAt {
			updates["updated_at"] = current.UpdatedAt
		}
		if len(updates) > 0 {
			wrote = true
			result := tx.Model(&db.P10ScenePresence{}).Where("player_id=?", id).Updates(updates)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				// Match Save's existing-row fallback if the row disappeared concurrently.
				if err = tx.Save(&current).Error; err != nil {
					return err
				}
			}
		}
	}
	if changed && old.SessionHash != "" {
		s.p10SceneBroadcast(ctx, old.Scene, old.Line, id, &pb.SCPlayerDisappear{PlayerId: []int64{id}})
	}
	broadcast := func(message proto.Message) {
		if wrote {
			// The frame write may have waited while another profile changed scope.
			s.p10SceneBroadcast(ctx, scope, line, id, message)
			return
		}
		s.p10SceneSendPeers(ctx, peers, message)
	}
	if changed {
		broadcast(&pb.SCPlayerAppear{Player: []*pb.MovePlayer{frame}})
		if len(peers) > 0 {
			s.p10SceneSend(ctx, id, &pb.SCPlayerAppear{Player: peers})
		}
	} else if movement {
		broadcast(&pb.SCMovePush{Position: []*pb.Position{frame.Position}})
	} else if !bytes.Equal(old.Frame, current.Frame) {
		broadcast(&pb.SCMovePlayerInfo{PlayerInfos: []*pb.MovePlayer{frame}})
	}
	return nil
}
func (s *GameService) DisconnectP10Scene(ctx context.Context, id int64, session []byte) error {
	return s.p10DisconnectScene(ctx, id, session, time.Now().UnixMilli())
}

func (s *GameService) p10DisconnectScene(ctx context.Context, id int64, session []byte, disconnectedAt int64) error {
	if handled, err := s.DisconnectP13UnionFightField(ctx, id, session); handled || err != nil {
		return err
	}
	if handled, err := s.DisconnectP13GuildRoom(ctx, id, session); handled || err != nil {
		return err
	}
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	// A replacement can only be installed after its predecessor's lifecycle
	// cleanup. Direct callers must also leave any live replacement untouched.
	if _, ok := p10LiveClient(p10ClientKey{s.p10ClientOwner(), id}); ok {
		return nil
	}
	tx := db.GetIns().WithContext(ctx)
	var probe int
	if err := tx.Raw("SELECT 1").Scan(&probe).Error; err != nil {
		return err
	}
	cfg, err := p10SceneConfig(tx)
	if err != nil || cfg == nil {
		return err
	}
	var row db.P10ScenePresence
	if err := tx.Where("player_id=? AND session_hash=?", id, p6Digest(session)).First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	if row.Scene == constUtil.GGOFieldScene {
		if err := s.ggoTradeCancelSession(ctx, id, session); err != nil {
			return err
		}
		completed, err := s.ggoExtractionDisconnect(ctx, id, session, disconnectedAt)
		if err != nil {
			return err
		}
		if completed {
			result := tx.Model(&db.P10ScenePresence{}).Where("player_id=? AND session_hash=?", id, p6Digest(session)).Updates(map[string]any{"session_hash": "", "offline_until": 0, "offline_pk_used": false})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 1 {
				s.p10SceneBroadcast(ctx, row.Scene, row.Line, id, &pb.SCPlayerDisappear{PlayerId: []int64{id}})
			}
			return nil
		}
		until := time.Now().UnixMilli() + 60000
		result := tx.Model(&db.P10ScenePresence{}).Where("player_id=? AND session_hash=?", id, p6Digest(session)).Updates(map[string]any{"session_hash": "", "offline_until": until, "offline_pk_used": false, "updated_at": time.Now().UnixMilli()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 1 {
			s.ggoScheduleOfflineExpiry(id, until)
		}
		return nil
	}
	if err := tx.Model(&db.P10ScenePresence{}).Where("player_id=? AND session_hash=?", id, p6Digest(session)).Update("session_hash", "").Error; err != nil {
		return err
	}
	s.p10SceneBroadcast(ctx, row.Scene, row.Line, id, &pb.SCPlayerDisappear{PlayerId: []int64{id}})
	return nil
}
func (s *GameService) AfterP10SceneRequest(ctx context.Context, id int64, request proto.Message) {
	switch request.(type) {
	case *pb.CSMove:
		_ = s.SyncP10Scene(ctx, id, true)
	case *pb.CSCurrentPosition, *pb.CSLoginScene, *pb.CSTransferScene, *pb.CSCopyEnter, *pb.CSCopyLeave, *pb.CSSidePveFightStart, *pb.CSSidePveFightEnd, *pb.CSSidePveBossFightStart, *pb.CSSidePveBossFightEnd, *pb.CSPingTcp, *pb.CSFormationOn, *pb.CSFormationOff, *pb.CSFormationExChange:
		_ = s.SyncP10Scene(ctx, id, false)
	}
}
func (s *GameService) DispatchP10Scene(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request.(type) {
	case *pb.CSGetLineInfo, *pb.CSTransferLine, *pb.CSMovePlayerInfo, *pb.CSRefreshMovePlayerInfo:
	default:
		return nil, false, nil
	}
	initial, err := s.P13GuildRoomSceneConfig(db.GetIns().WithContext(ctx), id)
	if err != nil {
		return nil, true, err
	}
	selfOnly := false
	if info, ok := request.(*pb.CSMovePlayerInfo); ok {
		selfOnly = len(info.GetPlayerIds()) == 0 || len(info.GetPlayerIds()) == 1 && info.GetPlayerIds()[0] == id
	}
	if _, ok := request.(*pb.CSRefreshMovePlayerInfo); ok {
		selfOnly = true
	}
	if initial == nil {
		if selfOnly {
			return nil, false, nil
		}
		return nil, true, errors.New(constUtil.P10SceneClosed)
	}
	if err := s.SyncP10Scene(ctx, id, false); err != nil {
		return nil, true, err
	}
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	tx := db.GetIns().WithContext(ctx)
	cfg, err := s.P13GuildRoomSceneConfig(tx, id)
	if err != nil {
		return nil, true, err
	}
	if cfg == nil {
		return nil, true, errors.New(constUtil.P10SceneClosed)
	}
	var own db.P10ScenePresence
	if err = tx.Where("player_id=?", id).First(&own).Error; err != nil {
		if selfOnly {
			return nil, false, nil
		}
		return nil, true, errors.New("请先完成真实场景TCP登录")
	}
	cfg, err = s.adminFieldSceneConfig(tx, own.Scene, cfg)
	if err != nil {
		return nil, true, err
	}
	if _, ok := s.p10SceneLive(tx, own); !ok {
		if selfOnly {
			return nil, false, nil
		}
		return nil, true, errors.New("当前角色不在有限公共场景")
	}
	switch b := request.(type) {
	case *pb.CSGetLineInfo:
		reply := &pb.SCGetLineInfo{Line: proto.Int32(own.Line)}
		for line := int32(1); line <= cfg.LineCount; line++ {
			peers, err := s.p10ScenePeers(tx, own.Scene, line, 0)
			if err != nil {
				return nil, true, err
			}
			state := int32(1)
			if len(peers) >= int(cfg.MaxPlayers) {
				state = 2
			}
			reply.LineInfos = append(reply.LineInfos, &pb.LineInfo{Line: proto.Int32(line), LineId: proto.String(fmt.Sprintf("p10:%s:%d", own.Scene, line)), State: proto.Int32(state)})
		}
		return []proto.Message{reply}, true, nil
	case *pb.CSTransferLine:
		if b.GetLine() < 1 || b.GetLine() > cfg.LineCount || stamp < 0 {
			return nil, true, errors.New("有限线路不存在")
		}
		var reply pb.SCTransferLine
		fresh := false
		oldLine := own.Line
		err = tx.Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var row db.Profile
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
				return err
			}
			// Original success clears actors before requesting CurrentPosition. Do not tear down an accepted action.
			player, game, decodeErr := decodeProfile(&row)
			if decodeErr != nil {
				return decodeErr
			}
			if game.Run != nil && !game.Run.Complete || game.Field != nil && game.Field.Fight != nil && !game.Field.Fight.Settled {
				return errors.New("请先结束当前战斗再换线")
			}
			if gameSchemaHasTable(tx, &db.P8GatherAttempt{}) {
				var attempt db.P8GatherAttempt
				gatherErr := tx.First(&attempt, "player_id=? AND active=?", id, true).Error
				if gatherErr == nil {
					return errors.New("请先结束当前采集再换线")
				}
				if !errors.Is(gatherErr, gorm.ErrRecordNotFound) {
					return gatherErr
				}
			}
			// A line change tears down visible actors, including its empty replay ack.
			if s.P12FieldPVPReady {
				if err := CheckP12FieldPVPAction(tx, id, time.Now()); err != nil {
					return err
				}
			}
			// 原 TCP 头没有 timestamp；只对实际携带正时间戳的原 HTTP 帧使用持久回执。
			if stamp > 0 {
				if found, err := p10ReadReceipt(tx, id, "line-transfer", stamp, request, &reply); err != nil || found {
					if err != nil {
						return err
					}
					// 原空回包会清场景对象并重拉当前线路；旧帧不能在已换线后再次触发该成功回调。
					if own.Line != b.GetLine() {
						return errors.New("换线回执对应线路已失效，请刷新当前线路")
					}
					return nil
				}
			}
			peers, err := s.p10ScenePeers(tx, own.Scene, b.GetLine(), id)
			if err != nil {
				return err
			}
			if len(peers) >= int(cfg.MaxPlayers) {
				return errors.New("目标分线已满")
			}
			if oldLine != b.GetLine() && game.Position.GetScene() == constUtil.GGOFieldScene && game.GGOField != nil {
				var locked db.P10ScenePresence
				if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "player_id=?", id).Error; err != nil {
					return err
				}
				if locked.Scene != own.Scene || locked.Line != oldLine || locked.SessionHash != p6Digest(row.Session) {
					return errors.New("场景会话已变化")
				}
				if err = ggoCommitLeave(tx, s, id, game, "line", "", b.GetLine(), time.Now().UnixMilli(), b, &reply); err != nil {
					return err
				}
				game.PositionLoading = true
				if err = saveProfile(tx, &row, player, game); err != nil {
					return err
				}
				own = locked
			}
			if oldLine != b.GetLine() && game.GGOField != nil && game.Position.GetScene() != constUtil.GGOFieldScene {
				if err = ggoPositionBump(game); err != nil {
					return err
				}
				game.PositionLoading = true
				if err = saveProfile(tx, &row, player, game); err != nil {
					return err
				}
			}
			own.Line = b.GetLine()
			if err = tx.Save(&own).Error; err != nil {
				return err
			}
			fresh = oldLine != own.Line
			if stamp == 0 {
				return nil
			}
			return p10SaveReceipt(tx, id, "line-transfer", stamp, request, &reply)
		})
		if err != nil {
			return nil, true, err
		}
		if fresh {
			s.p10SceneBroadcast(ctx, own.Scene, oldLine, id, &pb.SCPlayerDisappear{PlayerId: []int64{id}})
			oldPeers, err := s.p10ScenePeers(tx, own.Scene, oldLine, id)
			if err != nil {
				return nil, true, err
			}
			var ids []int64
			for _, peer := range oldPeers {
				ids = append(ids, peer.GetInfo().GetPlayerId())
			}
			if len(ids) > 0 {
				s.p10SceneSend(ctx, id, &pb.SCPlayerDisappear{PlayerId: ids})
			}
			frame, ok := s.p10SceneLive(tx, own)
			if ok {
				s.p10SceneBroadcast(ctx, own.Scene, own.Line, id, &pb.SCPlayerAppear{Player: []*pb.MovePlayer{frame}})
			}
			peers, err := s.p10ScenePeers(tx, own.Scene, own.Line, id)
			if err != nil {
				return nil, true, err
			}
			if len(peers) > 0 {
				s.p10SceneSend(ctx, id, &pb.SCPlayerAppear{Player: peers})
			}
		}
		return []proto.Message{&reply}, true, nil
	case *pb.CSRefreshMovePlayerInfo:
		peers, err := s.p10ScenePeers(tx, own.Scene, own.Line, 0)
		if err != nil {
			return nil, true, err
		}
		return []proto.Message{&pb.SCMovePlayerInfo{PlayerInfos: peers}}, true, nil
	case *pb.CSMovePlayerInfo:
		if len(b.GetPlayerIds()) > int(cfg.MaxPlayers) {
			return nil, true, errors.New("角色资料批量超过有限人数")
		}
		reply := &pb.SCMovePlayerInfo{}
		seen := map[int64]bool{}
		ids := b.GetPlayerIds()
		if len(ids) == 0 {
			ids = []int64{id}
		}
		for _, pid := range ids {
			if seen[pid] {
				continue
			}
			seen[pid] = true
			var row db.P10ScenePresence
			if tx.Where("player_id=? AND scene=? AND line=?", pid, own.Scene, own.Line).First(&row).Error != nil {
				continue
			}
			if frame, ok := s.p10SceneLive(tx, row); ok {
				reply.PlayerInfos = append(reply.PlayerInfos, frame)
			}
		}
		return []proto.Message{reply}, true, nil
	}
	return nil, true, errors.New("有限同屏请求未恢复")
}
