package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
)

var clientUpdateReady sync.Map

func clientUpdateDBKey(d *gorm.DB) any {
	q, err := d.DB()
	if err != nil {
		return d
	}
	return q
}
func clientUpdatesEnabled() bool {
	_, ok := clientUpdateReady.Load(clientUpdateDBKey(db.GetIns()))
	return ok
}

// InitializeClientUpdates 只建立零门槛策略；重启后沿最后活跃时刻计算重连宽限。
func (s *GameService) InitializeClientUpdates(ctx context.Context) error {
	d := db.GetIns().WithContext(ctx)
	if _, ready := clientUpdateReady.Load(clientUpdateDBKey(d)); ready {
		_, err := clientUpdatePolicy(ctx)
		return err
	}
	if err := d.AutoMigrate(&db.ClientUpdateRelease{}, &db.ClientUpdatePolicy{}, &db.ClientUpdateSession{}, &db.ClientUpdateReceipt{}, &db.ClientUpdateAction{}); err != nil {
		return err
	}
	if err := d.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.ClientUpdatePolicy{ID: 1}).Error; err != nil {
		return err
	}
	if err := d.Model(&db.ClientUpdateSession{}).Where("connected = ?", true).Updates(map[string]any{"connected": false, "disconnected_at": gorm.Expr("last_seen_at")}).Error; err != nil {
		return err
	}
	clientUpdateReady.Store(clientUpdateDBKey(d), s)
	_, err := clientUpdatePolicy(ctx)
	return err
}

// clientUpdatePolicy 在权威读取时推进到期维护，结束时间只作为预计信息。
func clientUpdatePolicy(ctx context.Context) (db.ClientUpdatePolicy, error) {
	d := db.GetIns().WithContext(ctx)
	var p db.ClientUpdatePolicy
	if err := d.First(&p, 1).Error; err != nil {
		return p, err
	}
	if !p.Maintenance && p.MaintenanceStartAt > 0 && p.MaintenanceStartAt <= time.Now().UnixMilli() {
		updated := d.Model(&db.ClientUpdatePolicy{}).Where("id=1 AND maintenance = ? AND maintenance_start_at > 0 AND maintenance_start_at <= ?", false, time.Now().UnixMilli()).Updates(map[string]any{"maintenance": true, "maintenance_epoch": gorm.Expr("maintenance_epoch + 1"), "revision": gorm.Expr("revision + 1")})
		if updated.Error != nil {
			return p, updated.Error
		}
		// 只有首次推进维护的进程通知并关闭本服务连接，重复查询不重复踢线。
		if updated.RowsAffected > 0 {
			if owner, ok := clientUpdateReady.Load(clientUpdateDBKey(d)); ok {
				owner.(*GameService).clientUpdateMaintenancePush()
			}
		}
		if err := d.First(&p, 1).Error; err != nil {
			return p, err
		}
	}
	return p, nil
}

func clientUpdateVersion(p db.ClientUpdatePolicy) string { return strconv.FormatInt(p.Revision, 10) }
func parseClientUpdateVersion(v string) (int64, string) {
	parts := strings.Split(v, "/")
	if len(parts) != 3 || parts[0] != "hyjs" {
		return 0, ""
	}
	n, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || n <= 0 || n > 2100000000 {
		return 0, ""
	}
	startup := strings.ToLower(strings.ReplaceAll(parts[2], "-", ""))
	if len(startup) != 32 {
		return 0, ""
	}
	if _, err = hex.DecodeString(startup); err != nil {
		return 0, ""
	}
	return n, startup
}
func clientUpdateSessionHash(session []byte) string {
	sum := sha256.Sum256(session)
	return hex.EncodeToString(sum[:])
}
func clientUpdateLoginError(p db.ClientUpdatePolicy, build int64) error {
	if p.Maintenance || (p.MaintenanceStartAt > 0 && p.MaintenanceStartAt <= time.Now().UnixMilli()) {
		return constUtil.ErrClientUpdateMaintenance
	}
	if build < p.MinLoginBuild {
		return constUtil.ErrClientUpdateRequired
	}
	return nil
}

// CheckClientLogin 在原登录生成新会话之前执行版本与维护门槛。
func (s *GameService) CheckClientLogin(ctx context.Context, appVersion string) error {
	if !clientUpdatesEnabled() {
		return nil
	}
	p, err := clientUpdatePolicy(ctx)
	if err != nil {
		return err
	}
	build, _ := parseClientUpdateVersion(appVersion)
	return clientUpdateLoginError(p, build)
}

// BindClientLogin 再次读取策略并原子保存服务端验证过的会话绑定。
func (s *GameService) BindClientLogin(ctx context.Context, row *db.Profile, appVersion string) error {
	if !clientUpdatesEnabled() {
		return nil
	}
	if _, err := clientUpdatePolicy(ctx); err != nil {
		return err
	}
	build, startup := parseClientUpdateVersion(appVersion)
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var p db.ClientUpdatePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, 1).Error; err != nil {
			return err
		}
		if err := clientUpdateLoginError(p, build); err != nil {
			return err
		}
		// 重新核对已落库会话，较早登录请求不能覆盖后一次登录的绑定。
		var current db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id", "session").First(&current, row.Id).Error; err != nil {
			return err
		}
		if clientUpdateSessionHash(current.Session) != clientUpdateSessionHash(row.Session) {
			return constUtil.ErrClientUpdateRequired
		}
		now := time.Now().UnixMilli()
		r := db.ClientUpdateSession{PlayerID: row.Id, SessionHash: clientUpdateSessionHash(row.Session), BuildCode: build, StartupID: startup, LoginAt: now, LastSeenAt: now, MaintenanceEpoch: p.MaintenanceEpoch}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "player_id"}}, UpdateAll: true}).Create(&r).Error
	})
}
func clientUpdateBound(ctx context.Context, row *db.Profile) (db.ClientUpdateSession, error) {
	var r db.ClientUpdateSession
	err := db.GetIns().WithContext(ctx).First(&r, "player_id=?", row.Id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return db.ClientUpdateSession{}, nil
	}
	if err != nil {
		return r, err
	}
	if r.SessionHash != clientUpdateSessionHash(row.Session) {
		return db.ClientUpdateSession{}, nil
	}
	return r, nil
}
func clientUpdateRunError(p db.ClientUpdatePolicy, r db.ClientUpdateSession) error {
	if p.Maintenance || (p.MaintenanceStartAt > 0 && p.MaintenanceStartAt <= time.Now().UnixMilli()) {
		return constUtil.ErrClientUpdateMaintenance
	}
	if r.MaintenanceEpoch < p.MaintenanceEpoch || r.BuildCode < p.MinRunBuild {
		return constUtil.ErrClientUpdateRequired
	}
	return nil
}

// CheckClientUpdateSession 校验所有已登录业务请求，HTTP轮询不延长断线窗口。
func CheckClientUpdateSession(ctx context.Context, row *db.Profile) error {
	if !clientUpdatesEnabled() {
		return nil
	}
	p, err := clientUpdatePolicy(ctx)
	if err != nil {
		return err
	}
	r, err := clientUpdateBound(ctx, row)
	if err != nil {
		return err
	}
	if err = clientUpdateRunError(p, r); err != nil {
		return err
	}
	if r.BuildCode < p.MinLoginBuild && (r.PlayerID == 0 || r.LoginAt > p.LoginGateAt || (!r.Connected && (r.DisconnectedAt == 0 || r.DisconnectedAt < p.LoginGateAt || time.Now().UnixMilli()-r.DisconnectedAt > constUtil.ClientUpdateReconnectMS))) {
		return constUtil.ErrClientUpdateRequired
	}
	// 在线活跃时间最多每十秒落库一次，断连轮询始终不能延长重连宽限。
	now := time.Now().UnixMilli()
	if r.Connected && now-r.LastSeenAt >= constUtil.ClientUpdateSeenIntervalMS {
		_ = db.GetIns().WithContext(ctx).Model(&db.ClientUpdateSession{}).Where("player_id=? AND session_hash=? AND connected=? AND last_seen_at<=?", row.Id, clientUpdateSessionHash(row.Session), true, now-constUtil.ClientUpdateSeenIntervalMS).Update("last_seen_at", now).Error
	}
	return nil
}

// CheckClientReconnect 只保留发布时在线的同一会话，首次断线最多保留五分钟。
func (s *GameService) CheckClientReconnect(ctx context.Context, row *db.Profile) error {
	if !clientUpdatesEnabled() {
		return nil
	}
	p, err := clientUpdatePolicy(ctx)
	if err != nil {
		return err
	}
	r, err := clientUpdateBound(ctx, row)
	if err != nil {
		return err
	}
	if err = clientUpdateRunError(p, r); err != nil {
		return err
	}
	if r.BuildCode >= p.MinLoginBuild {
		return nil
	}
	if r.PlayerID == 0 || r.LoginAt > p.LoginGateAt || (!r.Connected && (r.DisconnectedAt == 0 || r.DisconnectedAt < p.LoginGateAt || time.Now().UnixMilli()-r.DisconnectedAt > constUtil.ClientUpdateReconnectMS)) {
		return constUtil.ErrClientUpdateRequired
	}
	return nil
}

// ClientUpdateConnected 仅更新当前会话的连接事实。
func (s *GameService) ClientUpdateConnected(ctx context.Context, id int64, session []byte) {
	if !clientUpdatesEnabled() {
		return
	}
	_ = db.GetIns().WithContext(ctx).Model(&db.ClientUpdateSession{}).Where("player_id=? AND session_hash=?", id, clientUpdateSessionHash(session)).Updates(map[string]any{"connected": true, "disconnected_at": 0, "last_seen_at": time.Now().UnixMilli()}).Error
}

// ClientUpdateDisconnected 保留首次断线时刻，旧连接无法覆盖新会话。
func (s *GameService) ClientUpdateDisconnected(ctx context.Context, id int64, session []byte) {
	if !clientUpdatesEnabled() {
		return
	}
	_ = db.GetIns().WithContext(ctx).Model(&db.ClientUpdateSession{}).Where("player_id=? AND session_hash=? AND connected=?", id, clientUpdateSessionHash(session), true).Updates(map[string]any{"connected": false, "disconnected_at": time.Now().UnixMilli()}).Error
}

// ClientCheckVersionReply 保留原资源版本和禁用原下载器的字段。
func (s *GameService) ClientCheckVersionReply(ctx context.Context, appVersion, resVersion string) ([]proto.Message, error) {
	if err := s.CheckClientLogin(ctx, appVersion); err != nil {
		return GameError(constUtil.ErrGameRequest, err.Error()), nil
	}
	return gameReply("SCCheckVersion", map[string]any{"serverAppVersion": appVersion, "serverResVersion": resVersion, "programMethod": 0, "resMethod": 0, "entry": []any{map[string]any{"key": "needActivation", "value": "0"}}})
}

func clientUpdatePolicyDTO(p db.ClientUpdatePolicy) model.ClientUpdatePolicy {
	var out model.ClientUpdatePolicy
	_ = json.Unmarshal(adminJSON(p), &out)
	return out
}
func clientUpdateReleaseDTO(r db.ClientUpdateRelease) model.ClientUpdateRelease {
	var out model.ClientUpdateRelease
	_ = json.Unmarshal(adminJSON(r), &out)
	return out
}
func (s *GameService) clientUpdateStats(ctx context.Context, tx *gorm.DB, p db.ClientUpdatePolicy) (model.ClientUpdateStats, error) {
	out := model.ClientUpdateStats{ObservedAt: time.Now().UnixMilli()}
	owner := s.p10ClientOwner()
	sessions := map[int64]string{}
	ids := []int64{}
	p10ClientSinks.Range(func(key, value any) bool {
		k := key.(p10ClientKey)
		if k.service == owner {
			sessions[k.playerID] = clientUpdateSessionHash(value.(*p10ClientSink).session)
			ids = append(ids, k.playerID)
		}
		return true
	})
	if len(ids) == 0 {
		return out, nil
	}
	// 同一快照分块批量读取会话、账号和当前提醒回执，避免按连接逐条查库。
	for start := 0; start < len(ids); start += 400 {
		end := start + 400
		if end > len(ids) {
			end = len(ids)
		}
		chunk := ids[start:end]
		var bound []db.ClientUpdateSession
		if err := tx.Where("player_id IN ?", chunk).Find(&bound).Error; err != nil {
			return out, err
		}
		byID := map[int64]db.ClientUpdateSession{}
		for _, r := range bound {
			byID[r.PlayerID] = r
		}
		var profiles []db.Profile
		if err := tx.Select("id", "session", "account_id", "account_auth_revision").Where("id IN ?", chunk).Find(&profiles).Error; err != nil {
			return out, err
		}
		accountIDs := []int64{}
		for _, r := range profiles {
			if r.AccountID > 0 {
				accountIDs = append(accountIDs, r.AccountID)
			}
		}
		accounts := map[int64]db.GameAccount{}
		if len(accountIDs) > 0 {
			var rows []db.GameAccount
			if err := db.AccountsTable(tx, "game_accounts").Select("id", "enabled", "auth_revision").Where("id IN ?", accountIDs).Find(&rows).Error; err != nil {
				return out, err
			}
			for _, r := range rows {
				accounts[r.ID] = r
			}
		}
		var receipts []db.ClientUpdateReceipt
		if err := tx.Where("batch_id=? AND player_id IN ?", p.NoticeBatch, chunk).Find(&receipts).Error; err != nil {
			return out, err
		}
		receiptFlags := map[string]bool{}
		for _, r := range receipts {
			receiptFlags[strconv.FormatInt(r.PlayerID, 10)+":"+r.StartupID+":"+r.Kind] = true
		}
		for _, profile := range profiles {
			r, ok := byID[profile.Id]
			if !ok {
				r = db.ClientUpdateSession{PlayerID: profile.Id, SessionHash: sessions[profile.Id]}
			}
			if r.SessionHash != sessions[profile.Id] || clientUpdateSessionHash(profile.Session) != r.SessionHash {
				continue
			}
			if profile.AccountID > 0 {
				a, ok := accounts[profile.AccountID]
				if !ok || !a.Enabled || a.AuthRevision != profile.AccountAuthRevision {
					continue
				}
			}
			out.Online++
			if r.BuildCode == 0 {
				out.Unknown++
			}
			if r.BuildCode < p.LatestBuild {
				out.Older++
			}
			prefix := strconv.FormatInt(r.PlayerID, 10) + ":" + r.StartupID + ":"
			if receiptFlags[prefix+"shown"] {
				out.Shown++
			}
			if receiptFlags[prefix+"acknowledged"] {
				out.Acknowledged++
			}
		}
	}
	out.Pending = out.Online - out.Shown
	return out, nil
}

func (s *GameService) clientUpdateOverview(ctx context.Context, tx *gorm.DB, actor *model.AdminPrincipal) (*model.AdminClientUpdatesResponse, error) {
	var p db.ClientUpdatePolicy
	if err := tx.First(&p, 1).Error; err != nil {
		return nil, err
	}
	var actual db.AdminUser
	if err := tx.Select("id", "role").First(&actual, actor.UserID).Error; err != nil {
		return nil, err
	}
	out := &model.AdminClientUpdatesResponse{Version: clientUpdateVersion(p), Policy: clientUpdatePolicyDTO(p), Items: []model.ClientUpdateRelease{}, CanEdit: actual.Role != constUtil.AdminRoleViewer, CanPublish: actual.Role == constUtil.AdminRoleAdmin}
	var rows []db.ClientUpdateRelease
	if err := tx.Order("id DESC").Limit(50).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.Items = append(out.Items, clientUpdateReleaseDTO(r))
	}
	var err error
	out.Stats, err = s.clientUpdateStats(ctx, tx, p)
	return out, err
}

// AdminClientUpdates 返回当前服务连接快照及最近五十条发布记录。
func (s *GameService) AdminClientUpdates(ctx context.Context, actor *model.AdminPrincipal) (*model.AdminClientUpdatesResponse, error) {
	d := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(d, actor, false); err != nil {
		return nil, err
	}
	if _, err := clientUpdatePolicy(ctx); err != nil {
		return nil, err
	}
	return s.clientUpdateOverview(ctx, d, actor)
}

// AdminClientUpdateDraft 保存草稿并推进全局乐观锁，不改变生效门槛。
func (s *GameService) AdminClientUpdateDraft(ctx context.Context, actor *model.AdminPrincipal, req model.AdminClientUpdateDraftRequest) (*model.AdminClientUpdatesResponse, error) {
	var out *model.AdminClientUpdatesResponse
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var p db.ClientUpdatePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, 1).Error; err != nil {
			return err
		}
		if clientUpdateVersion(p) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		var r db.ClientUpdateRelease
		if req.ID > 0 {
			if err := tx.First(&r, req.ID).Error; err != nil {
				return err
			}
			if r.State != "draft" {
				return constUtil.ErrAdminConfigConflict
			}
		}
		r.ID = req.ID
		r.BuildCode = req.BuildCode
		r.VersionName = req.VersionName
		r.Title = req.Title
		r.Content = req.Content
		r.Mode = req.Mode
		r.MinLoginBuild = req.MinLoginBuild
		r.MinRunBuild = req.MinRunBuild
		r.MaintenanceStartAt = req.MaintenanceStartAt
		r.MaintenanceEndAt = req.MaintenanceEndAt
		r.State = "draft"
		r.UpdatedAt = time.Now().UnixMilli()
		if err := tx.Save(&r).Error; err != nil {
			return err
		}
		p.Revision++
		if err := tx.Save(&p).Error; err != nil {
			return err
		}
		if err := adminAudit(tx, actor, "client_update.draft", strconv.FormatInt(r.ID, 10), req.Reason); err != nil {
			return err
		}
		var err error
		out, err = s.clientUpdateOverview(ctx, tx, actor)
		return err
	})
	return out, err
}

// AdminClientUpdateAction 将策略、审计与幂等响应放入同一事务后再发送维护提示。
func (s *GameService) AdminClientUpdateAction(ctx context.Context, actor *model.AdminPrincipal, req model.AdminClientUpdateActionRequest) (*model.AdminClientUpdatesResponse, error) {
	if _, err := clientUpdatePolicy(ctx); err != nil {
		return nil, err
	}
	var out *model.AdminClientUpdatesResponse
	notify := false
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := adminOperationUser(tx, actor, true); err != nil {
			return err
		}
		var user db.AdminUser
		if err := tx.First(&user, actor.UserID).Error; err != nil {
			return err
		}
		if user.Role != constUtil.AdminRoleAdmin {
			return constUtil.ErrAdminPermission
		}
		var p db.ClientUpdatePolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, 1).Error; err != nil {
			return err
		}
		if selected := AdminSelectedServerID(ctx); selected > 0 && selected != s.OriginalServerID() {
			return constUtil.ErrAdminPermission
		}
		hash := adminVersion(req)
		var prior db.ClientUpdateAction
		err := tx.First(&prior, "request_id=?", req.RequestID).Error
		if err == nil {
			if prior.PayloadHash != hash || prior.ActorID != actor.UserID {
				return constUtil.ErrAdminConfigConflict
			}
			return json.Unmarshal([]byte(prior.ResponseJSON), &out)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if clientUpdateVersion(p) != req.Version {
			return constUtil.ErrAdminConfigConflict
		}
		if req.Action == "server_stop" || req.Action == "server_start" {
			stop := req.Action == "server_stop"
			if stop != p.Maintenance || (!stop && p.MaintenanceStartAt != 0) {
				p.Maintenance = stop
				p.MaintenanceStartAt, p.MaintenanceEndAt = 0, 0
				if stop {
					p.MaintenanceStartAt = time.Now().UnixMilli()
					p.MaintenanceEpoch++
					notify = true
				}
				p.Revision++
				if err = tx.Save(&p).Error; err != nil {
					return err
				}
			}
			if err = adminAudit(tx, actor, "server."+map[bool]string{true: "stop", false: "start"}[stop], "runtime", req.Reason); err != nil {
				return err
			}
			out, err = s.clientUpdateOverview(ctx, tx, actor)
			if err != nil {
				return err
			}
			return tx.Create(&db.ClientUpdateAction{RequestID: req.RequestID, PayloadHash: hash, ActorID: actor.UserID, ResponseJSON: string(adminJSON(out)), CreatedAt: time.Now().UnixMilli()}).Error
		}
		var r db.ClientUpdateRelease
		if err = tx.First(&r, req.ID).Error; err != nil {
			return err
		}
		now := time.Now().UnixMilli()
		switch req.Action {
		case "publish":
			if (r.Mode == constUtil.ClientUpdateModeNotice && (r.MinLoginBuild != p.MinLoginBuild || r.MinRunBuild != p.MinRunBuild)) || (r.Mode == constUtil.ClientUpdateModeNextLogin && (r.MinRunBuild != p.MinRunBuild || r.MinLoginBuild < 1)) {
				return constUtil.ErrAdminInput
			}
			if r.State != "draft" || r.BuildCode < p.LatestBuild || r.MinLoginBuild < p.MinLoginBuild || r.MinRunBuild < p.MinRunBuild {
				return constUtil.ErrAdminConfigConflict
			}
			if p.Maintenance || p.MaintenanceStartAt > 0 {
				return constUtil.ErrAdminConfigConflict
			}
			r.State = "published"
			r.PublishedAt = now
			r.PublishedBy = actor.Username
			p.ReleaseID = r.ID
			p.LatestBuild = r.BuildCode
			p.VersionName = r.VersionName
			p.Title = r.Title
			p.Content = r.Content
			p.Mode = r.Mode
			p.NoticeEnabled = true
			p.NoticeBatch++
			if r.Mode == constUtil.ClientUpdateModeMaintenance {
				p.MaintenanceStartAt = r.MaintenanceStartAt
				p.MaintenanceEndAt = r.MaintenanceEndAt
				if p.MaintenanceStartAt <= now {
					p.Maintenance = true
					p.MaintenanceEpoch++
					notify = true
				}
			} else {
				if r.MinLoginBuild > p.MinLoginBuild {
					p.LoginGateAt = now
				}
				p.MinLoginBuild = r.MinLoginBuild
				p.MinRunBuild = r.MinRunBuild
			}
		case "remind":
			if p.ReleaseID != r.ID || (r.State != "published" && r.State != "withdrawn") {
				return constUtil.ErrAdminConfigConflict
			}
			r.State = "published"
			p.NoticeEnabled = true
			p.NoticeBatch++
		case "withdraw":
			if p.ReleaseID != r.ID || r.State != "published" {
				return constUtil.ErrAdminConfigConflict
			}
			p.NoticeEnabled = false
			r.State = "withdrawn"
		case "start_maintenance":
			if p.ReleaseID != r.ID || p.Mode != constUtil.ClientUpdateModeMaintenance || p.Maintenance {
				return constUtil.ErrAdminConfigConflict
			}
			p.Maintenance = true
			p.MaintenanceStartAt = now
			p.MaintenanceEpoch++
			notify = true
		case "end_maintenance":
			if p.ReleaseID != r.ID || (!p.Maintenance && p.MaintenanceStartAt == 0) {
				return constUtil.ErrAdminConfigConflict
			}
			p.Maintenance = false
			p.MaintenanceStartAt = 0
			p.MaintenanceEndAt = 0
			if r.MinLoginBuild > p.MinLoginBuild {
				p.LoginGateAt = now
			}
			p.MinLoginBuild = r.MinLoginBuild
			p.MinRunBuild = r.MinRunBuild
		default:
			return constUtil.ErrAdminInput
		}
		p.Revision++
		r.UpdatedAt = now
		if err = tx.Save(&r).Error; err != nil {
			return err
		}
		if err = tx.Save(&p).Error; err != nil {
			return err
		}
		if err = adminAudit(tx, actor, "client_update."+req.Action, strconv.FormatInt(r.ID, 10), req.Reason); err != nil {
			return err
		}
		out, err = s.clientUpdateOverview(ctx, tx, actor)
		if err != nil {
			return err
		}
		return tx.Create(&db.ClientUpdateAction{RequestID: req.RequestID, PayloadHash: hash, ActorID: actor.UserID, ResponseJSON: string(adminJSON(out)), CreatedAt: now}).Error
	})
	if err == nil && notify {
		s.clientUpdateMaintenancePush()
	}
	return out, err
}
func (s *GameService) clientUpdateMaintenancePush() {
	owner := s.p10ClientOwner()
	messages := GameError(constUtil.ErrGameRequest, constUtil.ErrClientUpdateMaintenance.Error())
	p10ClientSinks.Range(func(key, value any) bool {
		if key.(p10ClientKey).service == owner {
			sink := value.(*p10ClientSink)
			for _, msg := range messages {
				_ = sink.send(proto.Clone(msg))
			}
			if sink.close != nil {
				sink.close()
			}
		}
		return true
	})
}

// AdminClientUpdateHistory 分页返回操作对象的发布记录。
func (s *GameService) AdminClientUpdateHistory(ctx context.Context, actor *model.AdminPrincipal, page, size int64) (*model.AdminClientUpdateHistoryResponse, error) {
	d := db.GetIns().WithContext(ctx)
	if err := adminOperationUser(d, actor, false); err != nil {
		return nil, err
	}
	out := &model.AdminClientUpdateHistoryResponse{Items: []model.ClientUpdateRelease{}}
	if err := d.Model(&db.ClientUpdateRelease{}).Count(&out.Total).Error; err != nil {
		return nil, err
	}
	var rows []db.ClientUpdateRelease
	if err := d.Order("id DESC").Offset(int((page - 1) * size)).Limit(int(size)).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.Items = append(out.Items, clientUpdateReleaseDTO(r))
	}
	return out, nil
}

func clientUpdateAuthenticate(ctx context.Context, build int64, startup string, id int64, sessionHex string) (*db.Profile, error) {
	if id == 0 && sessionHex == "" {
		return nil, nil
	}
	session, err := hex.DecodeString(sessionHex)
	if err != nil || len(session) != 16 {
		return nil, constUtil.ErrClientUpdateIdentity
	}
	row, err := AuthorizeAccount(ctx, id, session)
	if err != nil {
		return nil, err
	}
	bound, err := clientUpdateBound(ctx, row)
	if err != nil {
		return nil, err
	}
	if bound.PlayerID == 0 || bound.BuildCode != build || bound.StartupID != startup {
		return nil, constUtil.ErrClientUpdateIdentity
	}
	return row, nil
}

// ClientUpdateStatus 返回已绑定会话资格；被拦截时仍返回可供强制提示的策略。
func (s *GameService) ClientUpdateStatus(ctx context.Context, req model.ClientUpdateStatusRequest) (*model.ClientUpdateStatusResponse, error) {
	p, err := clientUpdatePolicy(ctx)
	if err != nil {
		return nil, err
	}
	out := &model.ClientUpdateStatusResponse{}
	_ = json.Unmarshal(adminJSON(clientUpdatePolicyDTO(p)), out)
	out.BatchID = p.NoticeBatch
	out.PollSeconds = 60
	// 达到当前发布版本时无需普通提醒，运行与登录资格继续独立校验。
	out.NoticeEnabled = out.NoticeEnabled && req.BuildCode < p.LatestBuild
	row, authErr := clientUpdateAuthenticate(ctx, req.BuildCode, req.StartupID, req.PlayerID, req.SessionHex)
	if req.PlayerID > 0 || req.SessionHex != "" {
		if authErr != nil {
			// 踢线、改密及绑定失效沿原登录流程处理；状态接口不授予业务访问。
			// 当前身份不可用时按新登录资格检查，只有更新/维护门槛展示强制更新。
			err = authErr
			if !errors.Is(authErr, constUtil.ErrClientUpdateRequired) && !errors.Is(authErr, constUtil.ErrClientUpdateMaintenance) {
				err = clientUpdateLoginError(p, req.BuildCode)
			}
			if err != nil {
				out.Blocked = true
				out.Reason = err.Error()
			}
			return out, nil
		}
		r, e := clientUpdateBound(ctx, row)
		if e != nil {
			return nil, e
		}
		err = clientUpdateRunError(p, r)
	} else {
		err = clientUpdateLoginError(p, req.BuildCode)
	}
	if err != nil {
		out.Blocked = true
		out.Reason = err.Error()
	}
	return out, nil
}

// ClientUpdateAck 只将已认证并匹配绑定的回执计入在线送达，匿名回执不冒充玩家。
func (s *GameService) ClientUpdateAck(ctx context.Context, req model.ClientUpdateAckRequest) (*model.ClientUpdateAckResponse, error) {
	p, err := clientUpdatePolicy(ctx)
	if err != nil {
		return nil, err
	}
	if req.BatchID != p.NoticeBatch || !p.NoticeEnabled {
		return nil, constUtil.ErrAdminConfigConflict
	}
	row, err := clientUpdateAuthenticate(ctx, req.BuildCode, req.StartupID, req.PlayerID, req.SessionHex)
	if err != nil {
		return nil, err
	}
	id := int64(0)
	if row != nil {
		id = row.Id
	}
	receipt := db.ClientUpdateReceipt{BatchID: req.BatchID, StartupID: req.StartupID, Kind: req.Kind, PlayerID: id, CreatedAt: time.Now().UnixMilli()}
	err = db.GetIns().WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "batch_id"}, {Name: "startup_id"}, {Name: "kind"}}, DoUpdates: clause.Assignments(map[string]any{"player_id": gorm.Expr("CASE WHEN player_id = 0 THEN ? ELSE player_id END", id)})}).Create(&receipt).Error
	return &model.ClientUpdateAckResponse{OK: err == nil}, err
}
