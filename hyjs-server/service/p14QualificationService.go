package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/httpClient"
	"hyjs-server/model"
)

// InitializeP14Qualification 初始化独立资格同步门禁和持久outbox，不把普通报名当作晋级。
func (s *GameService) InitializeP14Qualification(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14QualificationFederationPolicy{}, &db.P14QualificationDelivery{}, &db.P14ForeignQualification{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14QualificationFederationPolicy{ID: 1, Source: constUtil.P14QualificationSource}).Error
}
func p14QualificationID(request model.P14QualificationImportRequest, hash string) string {
	return p6Digest([]byte(fmt.Sprintf("p14-qualified/%d/%d/%d/%s", request.SeasonIndex, request.SourceServerID, request.DestinationServerID, hash)))
}

// p14QualificationProof 重建完整零场auto来源；真实打过的轮次须权威战斗证明，本版本明确拒绝。
func p14QualificationProof(request model.P14QualificationImportRequest, now time.Time) ([]db.P14AuditionRoster, *db.P14AuditionBoundary, *db.P14AuditionQualification, error) {
	var boundary db.P14AuditionBoundary
	var qualified db.P14AuditionQualification
	var roster []db.P14AuditionRoster
	if request.ProtocolVersion != constUtil.P14QualificationProtocol || request.Source != constUtil.P14QualificationSource || request.SourceServerID <= 0 || request.DestinationServerID <= 0 || request.SourceServerID == request.DestinationServerID || request.SeasonIndex <= 0 || json.Unmarshal(request.Boundary, &boundary) != nil || json.Unmarshal(request.Roster, &roster) != nil || json.Unmarshal(request.Qualification, &qualified) != nil {
		return nil, nil, nil, errors.New("跨服资格证书来源或完整结构不符")
	}
	if len(roster) < 1 || len(roster) > constUtil.P14AuditionPromotionLimit || boundary.Count != int64(len(roster)) || boundary.SeasonIndex != request.SeasonIndex || boundary.ServerID != request.SourceServerID || boundary.RegisterStart <= 0 || boundary.RegisterEnd <= boundary.RegisterStart || boundary.FrozenAt < boundary.RegisterEnd || boundary.FrozenAt > now.UnixMilli() || p6Digest(boundary.Calendar) != boundary.CalendarHash || p14AuditionBoundaryHash(&boundary) != boundary.SourceHash || p14AuditionRosterHash(roster) != boundary.RosterHash {
		return nil, nil, nil, errors.New("跨服资格截止名册数量/时刻/SHA不符")
	}
	var slots []db.P14TimeSlot
	if json.Unmarshal(boundary.Calendar, &slots) != nil {
		return nil, nil, nil, errors.New("跨服资格日历不可读")
	}
	var end int64
	registered, audition := false, false
	for _, slot := range slots {
		if slot.SeasonIndex != request.SeasonIndex || slot.Stage < 1 || slot.Stage > 19 || slot.StartAt <= 0 || slot.EndAt <= slot.StartAt || slot.StartAt < end || slot.Source == "" || slot.Description == "" {
			return nil, nil, nil, errors.New("跨服资格原日历损坏")
		}
		end = slot.EndAt
		if slot.Stage == constUtil.P14RegisterStage {
			registered = slot.StartAt == boundary.RegisterStart && slot.EndAt == boundary.RegisterEnd
		}
		if slot.Stage == constUtil.P14AuditionStage {
			audition = true
		}
	}
	if !registered || !audition {
		return nil, nil, nil, errors.New("跨服资格缺真实报名/海选日历")
	}
	ids := make([]int64, 0, len(roster))
	var previous int64
	for _, row := range roster {
		if err := p14AuditionValidate(&row); err != nil {
			return nil, nil, nil, err
		}
		if row.SeasonIndex != request.SeasonIndex || row.ServerID != request.SourceServerID || row.PlayerID <= previous || row.CapturedAt < boundary.RegisterStart || row.CapturedAt >= boundary.RegisterEnd {
			return nil, nil, nil, errors.New("跨服资格联合身份或截止顺序不符")
		}
		previous = row.PlayerID
		ids = append(ids, row.PlayerID)
	}
	players, err := json.Marshal(ids)
	if err != nil {
		return nil, nil, nil, err
	}
	if err := p14AuditionAutoRecoveryVerify(&boundary, &qualified); err != nil {
		return nil, nil, nil, err
	}
	if qualified.SeasonIndex != request.SeasonIndex || qualified.ServerID != request.SourceServerID || qualified.RoundCount != 0 || qualified.QualifiedAt < boundary.FrozenAt || qualified.QualifiedAt > now.UnixMilli() || qualified.BoundaryHash != boundary.SourceHash || !bytes.Equal(qualified.Players, players) || qualified.ResultsHash != p6Digest([]byte(boundary.SourceHash+"/"+constUtil.P14AuditionAutoSource)) || qualified.SourceHash != p14AuditionQualificationHash(&qualified) || request.DeliveryID != p14QualificationID(request, qualified.SourceHash) {
		return nil, nil, nil, errors.New("跨服零场晋级证书与真实截止来源不一致")
	}
	return roster, &boundary, &qualified, nil
}

// QueueP14Qualifications 只从已验证的实际本服资格建立不可变证书，不读取别服报名推断晋级。
func (s *GameService) QueueP14Qualifications(ctx context.Context, now time.Time) error {
	return db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var gate db.P14QualificationFederationPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		if !gate.Enabled {
			return nil
		}
		if gate.Source != constUtil.P14QualificationSource {
			return errors.New(constUtil.P14QualificationClosed)
		}
		var policy db.P14SeasonPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Enabled || policy.SeasonIndex <= 0 {
			return nil
		}
		qualified, err := s.p14AuditionQualificationRead(ctx, tx, policy.SeasonIndex, s.OriginalServerID())
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if qualified.RoundCount != 0 {
			return errors.New("真实比赛晋级证书仍需完整权威证明")
		}
		boundary, roster, err := p14AuditionRead(tx, policy.SeasonIndex, s.OriginalServerID())
		if err != nil {
			return err
		}
		boundaryRaw, err := json.Marshal(boundary)
		if err != nil {
			return err
		}
		rosterRaw, err := json.Marshal(roster)
		if err != nil {
			return err
		}
		qualifiedRaw, err := json.Marshal(qualified)
		if err != nil {
			return err
		}
		var peers []db.P14TrustedPeer
		if err := tx.Where("enabled=1 AND server_id<>?", s.OriginalServerID()).Order("server_id").Find(&peers).Error; err != nil {
			return err
		}
		for _, peer := range peers {
			if len(peer.SigningKey) < 32 || peer.Endpoint == "" {
				return errors.New("跨服资格可信目的服配置缺失")
			}
			request := model.P14QualificationImportRequest{ProtocolVersion: constUtil.P14QualificationProtocol, SourceServerID: s.OriginalServerID(), DestinationServerID: peer.ServerID, SeasonIndex: policy.SeasonIndex, Source: constUtil.P14QualificationSource, Boundary: boundaryRaw, Roster: rosterRaw, Qualification: qualifiedRaw}
			request.DeliveryID = p14QualificationID(request, qualified.SourceHash)
			if _, _, _, err := p14QualificationProof(request, now); err != nil {
				return err
			}
			payload, err := json.Marshal(request)
			if err != nil {
				return err
			}
			var previous db.P14QualificationDelivery
			err = tx.First(&previous, "id=?", request.DeliveryID).Error
			if err == nil {
				if previous.PayloadHash != p6Digest(payload) || !bytes.Equal(previous.Payload, payload) {
					return errors.New("跨服资格投递精确来源冲突")
				}
				continue
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := tx.Create(&db.P14QualificationDelivery{ID: request.DeliveryID, DestinationServerID: peer.ServerID, SeasonIndex: policy.SeasonIndex, Payload: payload, PayloadHash: p6Digest(payload), CreatedAt: now.UnixMilli()}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ImportP14Qualification 签名校验、当前赛季与不可覆写原服证书在同一行锁事务内提交。
func (s *GameService) ImportP14Qualification(ctx context.Context, request model.P14QualificationImportRequest, raw []byte, signature string, now time.Time) (*model.P14QualificationImportResponse, error) {
	var parsed model.P14QualificationImportRequest
	if json.Unmarshal(raw, &parsed) != nil {
		return nil, errors.New("跨服资格签名载荷不可读")
	}
	canonical, _ := json.Marshal(request)
	signed, _ := json.Marshal(parsed)
	if !bytes.Equal(canonical, signed) {
		return nil, errors.New("跨服资格签名载荷与解释不符")
	}
	reply := &model.P14QualificationImportResponse{DeliveryID: request.DeliveryID}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var gate db.P14QualificationFederationPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&gate, 1).Error; err != nil {
			return err
		}
		if !gate.Enabled || gate.Source != constUtil.P14QualificationSource {
			return errors.New(constUtil.P14QualificationClosed)
		}
		if request.DestinationServerID != s.OriginalServerID() {
			return errors.New("跨服资格目的部署不符")
		}
		var peer db.P14TrustedPeer
		if err := tx.First(&peer, "server_id=? AND enabled=1", request.SourceServerID).Error; err != nil {
			return err
		}
		actual, err := hex.DecodeString(signature)
		expected, _ := hex.DecodeString(P14SignPayload(peer.SigningKey, raw))
		if err != nil || len(peer.SigningKey) < 32 || !hmac.Equal(actual, expected) {
			return errors.New("跨服资格原服签名不符")
		}
		roster, boundary, qualified, err := p14QualificationProof(request, now)
		if err != nil {
			return err
		}
		var previous db.P14ForeignQualification
		err = tx.First(&previous, "season_index=? AND server_id=?", request.SeasonIndex, request.SourceServerID).Error
		if err == nil {
			if previous.DeliveryID != request.DeliveryID || previous.PayloadHash != p6Digest(raw) || !bytes.Equal(previous.Payload, raw) || previous.BoundaryHash != boundary.SourceHash || previous.QualificationHash != qualified.SourceHash {
				return errors.New("跨服原服截止证书不能被覆写")
			}
			reply.Confirmed = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var policy db.P14SeasonPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Enabled || policy.SeasonIndex != request.SeasonIndex {
			return errors.New("跨服资格未属于当前发布赛季")
		}
		configHash, err := s.p14ConfigurationHash()
		if err != nil {
			return err
		}
		for _, row := range roster {
			if row.ClientVersion != constUtil.P6OriginalClientHash || row.EngineVersion != constUtil.P6EngineVersion || row.ConfigurationHash != configHash {
				return errors.New("跨服新资格阵容版本与当前P6不兼容")
			}
		}
		if err := tx.Create(&db.P14ForeignQualification{SeasonIndex: request.SeasonIndex, ServerID: request.SourceServerID, DeliveryID: request.DeliveryID, Payload: append([]byte(nil), raw...), PayloadHash: p6Digest(raw), BoundaryHash: boundary.SourceHash, QualificationHash: qualified.SourceHash, ImportedAt: now.UnixMilli()}).Error; err != nil {
			return err
		}
		reply.Confirmed = true
		return nil
	})
	return reply, err
}

// ReadP14ForeignQualifiedEntrants 每次验证持久精确SHA与完整证书；联合身份保留原服，不合并同localPlayerID。
func (s *GameService) ReadP14ForeignQualifiedEntrants(ctx context.Context, tx *gorm.DB, season int32, now time.Time) ([]db.P14AuditionRoster, error) {
	var certificates []db.P14ForeignQualification
	if err := tx.WithContext(ctx).Where("season_index=?", season).Order("server_id").Find(&certificates).Error; err != nil {
		return nil, err
	}
	var result []db.P14AuditionRoster
	for _, saved := range certificates {
		var request model.P14QualificationImportRequest
		if saved.PayloadHash != p6Digest(saved.Payload) || json.Unmarshal(saved.Payload, &request) != nil || request.DeliveryID != saved.DeliveryID || request.DestinationServerID != s.OriginalServerID() || request.SourceServerID != saved.ServerID || request.SeasonIndex != saved.SeasonIndex {
			return nil, errors.New("跨服持久资格证书精确SHA损坏")
		}
		roster, boundary, qualified, err := p14QualificationProof(request, now)
		if err != nil {
			return nil, err
		}
		if boundary.SourceHash != saved.BoundaryHash || qualified.SourceHash != saved.QualificationHash {
			return nil, errors.New("跨服持久资格来源SHA损坏")
		}
		result = append(result, roster...)
	}
	return result, nil
}

// DeliverP14Qualifications 仅向当前可信配置的目的地址投递冻结字节；离线保留outbox，确认精确ID后才置完成。
func (s *GameService) DeliverP14Qualifications(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	var gate db.P14QualificationFederationPolicy
	if err := tx.First(&gate, 1).Error; err != nil {
		return err
	}
	if !gate.Enabled {
		return nil
	}
	if gate.Source != constUtil.P14QualificationSource {
		return errors.New(constUtil.P14QualificationClosed)
	}
	var entries []db.P14QualificationDelivery
	if err := tx.Where("confirmed=0").Order("created_at,id").Limit(constUtil.P14QualificationBatchSize).Find(&entries).Error; err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if p6Digest(entry.Payload) != entry.PayloadHash {
			return errors.New("跨服资格outbox精确SHA损坏")
		}
		var peer db.P14TrustedPeer
		if err := tx.First(&peer, "server_id=? AND enabled=1", entry.DestinationServerID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			}
			return err
		}
		if len(peer.SigningKey) < 32 || peer.Endpoint == "" {
			continue
		}
		cli := httpClient.HttpInner("p14-qualified-"+strconv.FormatInt(peer.ServerID, 10)).SetTimeout(3*time.Second).AddHeader("X-HYJS-P14-Signature", P14SignPayload(peer.SigningKey, entry.Payload))
		status, raw, callErr := cli.PostWithContext(ctx, peer.Endpoint+constUtil.P14QualificationPath, entry.Payload)
		if err := tx.Model(&db.P14QualificationDelivery{}).Where("id=? AND confirmed=0", entry.ID).UpdateColumn("attempt_count", gorm.Expr("attempt_count+1")).Error; err != nil {
			return err
		}
		if callErr != nil || status != http.StatusOK {
			continue
		}
		var reply struct {
			Code int64                                `json:"code"`
			Data model.P14QualificationImportResponse `json:"data"`
		}
		if json.Unmarshal(raw, &reply) != nil || reply.Code != 0 || !reply.Data.Confirmed || reply.Data.DeliveryID != entry.ID {
			continue
		}
		if err := tx.Model(&db.P14QualificationDelivery{}).Where("id=? AND confirmed=0", entry.ID).Updates(map[string]any{"confirmed": true, "confirmed_at": time.Now().UnixMilli()}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ServeP14Qualification 复用原P14五秒ctx节拍，仅同步真实已产生资格；不推进比赛、计分或奖励。
func (s *GameService) ServeP14Qualification(ctx context.Context) error {
	ticker := time.NewTicker(constUtil.P14DeliveryInterval)
	defer ticker.Stop()
	for {
		if err := s.QueueP14Qualifications(ctx, LocalPlayNow(ctx)); err != nil {
			return err
		}
		if err := s.DeliverP14Qualifications(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
