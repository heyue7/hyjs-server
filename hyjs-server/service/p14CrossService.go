package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/httpClient"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"math"
	"net/http"
	"strconv"
	"time"
)

// InitializeP14Cross 初始化真实联合身份与投递回执，赛季日历、对等服和报名必须明确配置。
func (s *GameService) InitializeP14Cross(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P14SeasonPolicy{}, &db.P14TimeSlot{}, &db.P14TrustedPeer{}, &db.P14Member{}, &db.P14Receipt{}, &db.P14Delivery{}, &db.P14ImportReceipt{}, &db.P14AuditionRoster{}, &db.P14AuditionBoundary{}, &db.P14AuditionMatchPolicy{}, &db.P14AuditionRound{}, &db.P14AuditionMatch{}, &db.P14AuditionQualification{}, &db.P14PromotionFormation{}); err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14AuditionMatchPolicy{ID: 1, Source: constUtil.P14AuditionMatchSource}).Error; err != nil {
		return err
	}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P14SeasonPolicy{ID: 1, Source: constUtil.P14Source}).Error; err != nil {
		return err
	}
	if err := s.InitializeP14Promotion(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14PromotionFight(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14PromotionRank(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14FinalQualification(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14FinalFormation(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14FinalDraw(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14FinalGame(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14Guess(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14PairGuess(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14FinalSeries(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14PerGame(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14FinalSettlement(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14FinalResult(ctx); err != nil {
		return err
	}
	if err := s.InitializeP14FinalContinuation(ctx); err != nil {
		return err
	}
	return s.InitializeP14PairGuessAwards(ctx)
}
func p14SnapshotDigest(formation, battle []byte) string {
	raw, _ := json.Marshal(struct{ Formation, Battle []byte }{formation, battle})
	return p6Digest(raw)
}

// P14SignPayload 仅由原服持有的互信密钥签名精确载荷，游戏客户端没有跨服上传入口。
func P14SignPayload(key, raw []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}
func (s *GameService) p14ConfigurationHash() (string, error) {
	raw, err := json.Marshal(s.Tables)
	if err != nil {
		return "", err
	}
	return p6Digest(raw), nil
}
func p14CurrentStage(policy *db.P14SeasonPolicy, now time.Time) int32 {
	if policy.Enabled && policy.SeasonIndex > 0 && now.UnixMilli() >= policy.RegisterStart && now.UnixMilli() < policy.RegisterEnd {
		return constUtil.P14RegisterStage
	}
	return 0
}

// p14Register 由已认证原角色和可信启动服号生成快照；本地登记/投递outbox/原帧回执同事务。
func (s *GameService) p14Register(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	raw, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	op := string(request.ProtoReflect().Descriptor().Name())
	var response proto.Message
	switch request.(type) {
	case *pb.CSKFEnterGame:
		response = &pb.SCKFEnterGame{}
	case *pb.CSKFEnterFrightGame:
		response = &pb.SCKFEnterFrightGame{}
	default:
		return nil, errors.New(constUtil.P14ResultsUnverified)
	}
	var player *pb.Player
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		var row db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
			return err
		}
		p, state, err := decodeProfileForCalendarUpdate(tx, &row)
		if err != nil {
			return err
		}
		if err = s.RefreshP5GuildPrayerPower(ctx, tx, p, state, p13UFNow(ctx)); err != nil {
			return err
		}
		player = proto.Clone(p).(*pb.Player)
		var receipt db.P14Receipt
		err = tx.First(&receipt, "player_id=? AND operation=? AND stamp=?", id, op, stamp).Error
		if err == nil {
			if !bytes.Equal(raw, receipt.Request) {
				return errors.New("跨服原请求时间戳冲突")
			}
			return proto.Unmarshal(receipt.Response, response)
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		// Original promotion stage6 has its own upload button. Preserve the sealed audition
		// roster and write a distinct, qualified promotion snapshot and outbox.
		if _, upload := request.(*pb.CSKFEnterFrightGame); upload {
			stage, _, err := s.p14SeasonStage(tx, &policy, now)
			if err != nil {
				return err
			}
			if stage == constUtil.P14Final16BeforeStage || stage == constUtil.P14Final16Stage {
				power, err := s.P14FinalFormationUploadCurrent(ctx, tx, &policy, p, state, now)
				if err != nil {
					return err
				}
				response.(*pb.SCKFEnterFrightGame).LastFightPower = proto.Int32(power)
				encoded, err := proto.Marshal(response)
				if err != nil {
					return err
				}
				return tx.Create(&db.P14Receipt{PlayerID: id, Operation: op, Stamp: stamp, Request: raw, Response: encoded}).Error
			}
			if stage == constUtil.P14PromotionStage {
				formation, err := s.p14PromotionFormationUpload(ctx, tx, &policy, p, state, now)
				if err != nil {
					return err
				}
				response.(*pb.SCKFEnterFrightGame).LastFightPower = proto.Int32(formation.FightPower)
				encoded, err := proto.Marshal(response)
				if err != nil {
					return err
				}
				return tx.Create(&db.P14Receipt{PlayerID: id, Operation: op, Stamp: stamp, Request: raw, Response: encoded}).Error
			}
		}
		var frozen int64
		if err = tx.Model(&db.P14AuditionBoundary{}).Where("season_index=? AND server_id=?", policy.SeasonIndex, s.OriginalServerID()).Count(&frozen).Error; err != nil {
			return err
		}
		if frozen > 0 {
			return errors.New("原本服海选名单已截止封存，排队上传不能改变名单")
		}
		stage, _, err := p14PublishedStage(tx, &policy, now)
		if err != nil {
			return err
		}
		if stage != constUtil.P14RegisterStage {
			return errors.New(constUtil.P14Closed)
		}
		level, err := s.p10Setting("ServerBattleLevelLimit")
		if err != nil {
			return err
		}
		if p.GetLevel() < level {
			return errors.New("未达到原70级跨服报名资格")
		}
		var member db.P14Member
		err = tx.First(&member, "season_index=? AND server_id=? AND player_id=?", policy.SeasonIndex, s.OriginalServerID(), id).Error
		exists := err == nil
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if _, upload := request.(*pb.CSKFEnterFrightGame); upload && !exists {
			return errors.New("当前赛季尚未报名")
		}
		// 新stamp重复报名保持原首份阵容；显式原上传请求才建立下一版本。
		_, signup := request.(*pb.CSKFEnterGame)
		if !signup || !exists {
			if member.Revision == math.MaxInt64 {
				return errors.New("跨服阵容版本溢出")
			}
			battle, err := s.battlePlayer(p, state)
			if err != nil {
				return err
			}
			formation, err := s.p10FormationProfile(tx, id)
			if err != nil {
				return err
			}
			encodedBattle, err := proto.Marshal(battle)
			if err != nil {
				return err
			}
			encodedFormation, err := proto.Marshal(formation)
			if err != nil {
				return err
			}
			configHash, err := s.p14ConfigurationHash()
			if err != nil {
				return err
			}
			member = db.P14Member{SeasonIndex: policy.SeasonIndex, ServerID: s.OriginalServerID(), PlayerID: id, Revision: member.Revision + 1, CapturedAt: now.UnixMilli(), FightPower: formation.GetFightPower(), Snapshot: encodedFormation, SnapshotHash: p14SnapshotDigest(encodedFormation, encodedBattle), Battle: encodedBattle, ConfigurationHash: configHash, EngineVersion: constUtil.P6EngineVersion, ClientVersion: constUtil.P6OriginalClientHash}
			if err = tx.Save(&member).Error; err != nil {
				return err
			}
			var peers []db.P14TrustedPeer
			if err = tx.Where("enabled=1 AND server_id<>?", s.OriginalServerID()).Order("server_id").Find(&peers).Error; err != nil {
				return err
			}
			if len(peers) == 0 {
				return errors.New("跨服可信对等服尚未配置")
			}
			for _, peer := range peers {
				if len(peer.SigningKey) < 32 || peer.Endpoint == "" {
					return errors.New("跨服可信目的服配置缺失")
				}
				envelope := model.P14SnapshotImportRequest{ProtocolVersion: constUtil.P14ProtocolVersion, SourceServerID: member.ServerID, DestinationServerID: peer.ServerID, SeasonIndex: member.SeasonIndex, PlayerID: member.PlayerID, Revision: member.Revision, CapturedAt: member.CapturedAt, FightPower: member.FightPower, Formation: member.Snapshot, Battle: member.Battle, SnapshotHash: member.SnapshotHash, ConfigurationHash: member.ConfigurationHash, ClientVersion: member.ClientVersion, EngineVersion: member.EngineVersion}
				envelope.DeliveryID = p6Digest([]byte(fmt.Sprintf("p14/%d/%d/%d/%d/%d", member.SeasonIndex, member.ServerID, member.PlayerID, member.Revision, peer.ServerID)))
				payload, err := json.Marshal(envelope)
				if err != nil {
					return err
				}
				if err = tx.Create(&db.P14Delivery{ID: envelope.DeliveryID, DestinationServerID: peer.ServerID, Payload: payload, CreatedAt: now.UnixMilli()}).Error; err != nil {
					return err
				}
			}
		}
		if reply, ok := response.(*pb.SCKFEnterFrightGame); ok {
			reply.LastFightPower = proto.Int32(member.FightPower)
		}
		encoded, err := proto.Marshal(response)
		if err != nil {
			return err
		}
		return tx.Create(&db.P14Receipt{PlayerID: id, Operation: op, Stamp: stamp, Request: raw, Response: encoded}).Error
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response, &pb.SCPlayer{Player: player}}, nil
}

// ImportP14Snapshot 校验原服互信、精确载荷、版本与联合身份，高水位防止乱序覆盖新阵容。
func (s *GameService) ImportP14Snapshot(ctx context.Context, request model.P14SnapshotImportRequest, raw []byte, signature string, now time.Time) (*model.P14SnapshotImportResponse, error) {
	// Only an explicitly injected internal test context can advance the clock.
	// The ordinary HTTP handler has no request field or header for this value.
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		now = clock()
	}
	var signed model.P14SnapshotImportRequest
	if json.Unmarshal(raw, &signed) != nil {
		return nil, errors.New("跨服签名载荷无法读取")
	}
	encoded, _ := json.Marshal(request)
	signedEncoded, _ := json.Marshal(signed)
	if !bytes.Equal(encoded, signedEncoded) {
		return nil, errors.New("跨服签名载荷与解释字段不一致")
	}
	reply := &model.P14SnapshotImportResponse{DeliveryID: request.DeliveryID}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		var peer db.P14TrustedPeer
		if err := tx.First(&peer, "server_id=? AND enabled=1", request.SourceServerID).Error; err != nil {
			return errors.New("跨服原服不在可信名单")
		}
		actual, err := hex.DecodeString(signature)
		if err != nil || len(peer.SigningKey) < 32 {
			return errors.New("跨服载荷签名无效")
		}
		expected, _ := hex.DecodeString(P14SignPayload(peer.SigningKey, raw))
		if !hmac.Equal(actual, expected) {
			return errors.New("跨服载荷签名无效")
		}
		if (request.ProtocolVersion != constUtil.P14ProtocolVersion && request.ProtocolVersion != constUtil.P14PromotionFormationProtocolVersion && request.ProtocolVersion != constUtil.P14FinalFormationProtocolVersion) || request.DestinationServerID != s.OriginalServerID() || request.SourceServerID == s.OriginalServerID() {
			return errors.New("跨服协议版本或目的服不兼容")
		}
		expectedID := p6Digest([]byte(fmt.Sprintf("p14/%d/%d/%d/%d/%d", request.SeasonIndex, request.SourceServerID, request.PlayerID, request.Revision, request.DestinationServerID)))
		if request.ProtocolVersion == constUtil.P14PromotionFormationProtocolVersion {
			expectedID = p14PromotionFormationID(request)
		}
		if request.ProtocolVersion == constUtil.P14FinalFormationProtocolVersion {
			expectedID = p14FinalFormationID(request)
		} else if request.RoundStage != 0 || request.FinalQualificationHash != "" || request.FinalOfferHash != "" {
			return errors.New(constUtil.P14FinalFormationCorrupt)
		}
		if request.DeliveryID != expectedID {
			return errors.New("跨服投递单号与联合身份不一致")
		}
		var receipt db.P14ImportReceipt
		err = tx.First(&receipt, "id=?", request.DeliveryID).Error
		if err == nil {
			if receipt.SourceServerID != request.SourceServerID || receipt.PayloadHash != p6Digest(raw) {
				return errors.New("跨服投递单号冲突")
			}
			reply.Confirmed = true
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if request.ProtocolVersion == constUtil.P14FinalFormationProtocolVersion {
			if err = s.p14FinalFormationImport(ctx, tx, &policy, request, now); err != nil {
				return err
			}
			if err = tx.Create(&db.P14ImportReceipt{ID: request.DeliveryID, PayloadHash: p6Digest(raw), SourceServerID: request.SourceServerID, ImportedAt: now.UnixMilli()}).Error; err != nil {
				return err
			}
			reply.Confirmed = true
			return nil
		}
		if request.ProtocolVersion == constUtil.P14PromotionFormationProtocolVersion {
			if err = s.p14PromotionFormationImport(ctx, tx, &policy, request, now); err != nil {
				return err
			}
			if err = tx.Create(&db.P14ImportReceipt{ID: request.DeliveryID, PayloadHash: p6Digest(raw), SourceServerID: request.SourceServerID, ImportedAt: now.UnixMilli()}).Error; err != nil {
				return err
			}
			reply.Confirmed = true
			return nil
		}
		if !policy.Enabled || request.SeasonIndex != policy.SeasonIndex || request.CapturedAt < policy.RegisterStart || request.CapturedAt >= policy.RegisterEnd {
			return errors.New("跨服赛季或原采集资格无效")
		}
		capturedStage, _, err := p14PublishedStage(tx, &policy, time.UnixMilli(request.CapturedAt))
		if err != nil {
			return err
		}
		if capturedStage != constUtil.P14RegisterStage {
			return errors.New("跨服阵容采集不在原发布报名阶段")
		}
		configHash, err := s.p14ConfigurationHash()
		if err != nil {
			return err
		}
		if request.ClientVersion != constUtil.P6OriginalClientHash || request.EngineVersion != constUtil.P6EngineVersion || request.ConfigurationHash != configHash {
			return errors.New("跨服阵容配置版本不兼容")
		}
		formation := &pb.FormationSimpleInfo{}
		battle := &pb.BattlePlayer{}
		if p14SnapshotDigest(request.Formation, request.Battle) != request.SnapshotHash || proto.Unmarshal(request.Formation, formation) != nil || proto.Unmarshal(request.Battle, battle) != nil || battle.GetPlayerId() != request.PlayerID || int64(battle.GetServerId()) != request.SourceServerID || formation.GetFightPower() != request.FightPower || len(battle.GetFormation()) == 0 {
			return errors.New("跨服联合身份或阵容内容损坏")
		}
		var member db.P14Member
		err = tx.First(&member, "season_index=? AND server_id=? AND player_id=?", request.SeasonIndex, request.SourceServerID, request.PlayerID).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err == nil && member.Revision == request.Revision && member.SnapshotHash != request.SnapshotHash {
			return errors.New("跨服同版本阵容冲突")
		}
		if errors.Is(err, gorm.ErrRecordNotFound) || request.Revision > member.Revision {
			member = db.P14Member{SeasonIndex: request.SeasonIndex, ServerID: request.SourceServerID, PlayerID: request.PlayerID, Revision: request.Revision, CapturedAt: request.CapturedAt, FightPower: request.FightPower, Snapshot: request.Formation, SnapshotHash: request.SnapshotHash, Battle: request.Battle, ConfigurationHash: request.ConfigurationHash, EngineVersion: request.EngineVersion, ClientVersion: request.ClientVersion}
			if err = tx.Save(&member).Error; err != nil {
				return err
			}
		}
		if err = tx.Create(&db.P14ImportReceipt{ID: request.DeliveryID, PayloadHash: p6Digest(raw), SourceServerID: request.SourceServerID, ImportedAt: now.UnixMilli()}).Error; err != nil {
			return err
		}
		reply.Confirmed = true
		return nil
	})
	return reply, err
}

// DeliverP14Snapshots 外部HTTP发生在本地提交后；原服离线仅保留待确认投递，不伪造失败或败北。
func (s *GameService) DeliverP14Snapshots(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	var entries []db.P14Delivery
	if err := tx.Where("confirmed=0").Order("created_at,id").Limit(100).Find(&entries).Error; err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
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
		// 每次取得独立内部客户端，头、超时和ctx不会污染另一份并发投递。
		cli := httpClient.HttpInner("p14-zone-"+strconv.FormatInt(peer.ServerID, 10)).SetTimeout(3*time.Second).AddHeader("X-HYJS-P14-Signature", P14SignPayload(peer.SigningKey, entry.Payload))
		status, raw, err := cli.PostWithContext(ctx, peer.Endpoint+constUtil.P14DeliveryPath, entry.Payload)
		if err = tx.Model(&db.P14Delivery{}).Where("id=? AND confirmed=0", entry.ID).UpdateColumn("attempt_count", gorm.Expr("attempt_count+1")).Error; err != nil {
			return err
		}
		if status != http.StatusOK {
			continue
		}
		var reply struct {
			Code int64                           `json:"code"`
			Data model.P14SnapshotImportResponse `json:"data"`
		}
		if json.Unmarshal(raw, &reply) != nil || reply.Code != 0 || !reply.Data.Confirmed || reply.Data.DeliveryID != entry.ID {
			continue
		}
		if err = tx.Model(&db.P14Delivery{}).Where("id=? AND confirmed=0", entry.ID).Updates(map[string]any{"confirmed": true, "confirmed_at": time.Now().UnixMilli()}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ServeP14Delivery 使用共享服务ctx恢复未确认投递，关闭进程或暂不可达不会删掉outbox。
func (s *GameService) ServeP14Delivery(ctx context.Context) error {
	ticker := time.NewTicker(constUtil.P14DeliveryInterval)
	defer ticker.Stop()
	for {
		if err := s.TickP14Season(ctx, LocalPlayNow(ctx)); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := s.DeliverP14Snapshots(ctx); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := s.TickP14FinalGame(ctx, LocalPlayNow(ctx)); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := s.TickP14FinalSeries(ctx, LocalPlayNow(ctx)); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := s.TickP14PerGame(ctx, LocalPlayNow(ctx)); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := s.TickP14FinalSettlements(ctx, LocalPlayNow(ctx)); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := s.TickP14FinalResults(ctx, LocalPlayNow(ctx)); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		// New continuation shares this worker; only verified current-slot results advance.
		if err := s.TickP14FinalContinuations(ctx, LocalPlayNow(ctx)); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		if err := s.TickP14PairGuessAwards(ctx, LocalPlayNow(ctx)); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *GameService) p14Query(ctx context.Context, id int64, request proto.Message, now time.Time) ([]proto.Message, error) {
	var response proto.Message
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var policy db.P14SeasonPolicy
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&policy, 1).Error; err != nil {
			return err
		}
		if err := s.p14FreezeAuditionTX(tx, &policy, now); err != nil {
			return err
		}
		var member db.P14Member
		err := tx.First(&member, "season_index=? AND server_id=? AND player_id=?", policy.SeasonIndex, s.OriginalServerID(), id).Error
		registered := err == nil
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		entered := int32(0)
		if registered {
			entered = 1
		}
		stage, endAt, err := s.p14SeasonStage(tx, &policy, now)
		if err != nil {
			return err
		}
		switch b := request.(type) {
		case *pb.CSKFLogin:
			response = &pb.SCKFLogin{IsEnter: proto.Int32(entered), TypeStage: proto.Int32(stage), KfOrder: proto.Int32(policy.SeasonIndex), LastFightPower: proto.Int32(member.FightPower)}
		case *pb.CSKFOpenFrightServerInfo:
			currentEntered := int32(0)
			if stage == constUtil.P14RegisterStage {
				currentEntered = entered
			} else {
				currentEntered, err = s.p14AuditionEntered(tx, &policy, id, stage)
				if err != nil {
					return err
				}
			}
			genuineAdmission := currentEntered
			eliminatedStage := int32(0)
			mateTime := int32(0)
			if stage == constUtil.P14PromotionStage {
				currentEntered, mateTime, err = s.p14PromotionStatusTX(ctx, tx, &policy, id, now)
				if err != nil {
					return err
				}
			}
			if stage == constUtil.P14Final16BeforeStage || stage == constUtil.P14Final16Stage {
				qualified, finalErr := s.p14FinalQualificationTX(ctx, tx, policy.SeasonIndex, now)
				if finalErr != nil {
					return finalErr
				}
				currentEntered = p14FinalQualificationEntered(qualified, s.OriginalServerID(), id)
				genuineAdmission = currentEntered
				if qualified != nil && currentEntered == 0 {
					for _, entry := range qualified.Ranking {
						if entry.ServerID == s.OriginalServerID() && entry.PlayerID == id {
							eliminatedStage = constUtil.P14EliminatedPromotion
						}
					}
				}
			}
			participation, wireErr := p14OriginalParticipation(stage, registered, genuineAdmission == 1, eliminatedStage)
			if wireErr != nil {
				return wireErr
			}
			// 原晋级赛只展示真实资格和点账；胜负、晋级计分及原匹配成功仍须权威来源。
			response = &pb.SCKFOpenFrightServerInfo{TypeStage: proto.Int32(stage), EndTime: proto.Int64(endAt), CurrentTime: proto.Int64(now.UnixMilli()), IsEnterCurStage: proto.Int32(participation), IsNeedPoped: proto.Int32(0), MateTime: proto.Int32(mateTime)}
		case *pb.CSKFTimeInfo:
			response, err = s.p14SeasonCalendarReply(tx, &policy, b)
			if err != nil {
				return err
			}
		case *pb.CSKFPlayerInfo:
			if b.GetType() == 3 && b.GetReplayFlag() == 0 && (b.GetPeakMatchIndex() == 0 || b.GetPeakMatchIndex() == policy.SeasonIndex) {
				formation, err := s.P14FinalFormationQueryCurrent(ctx, tx, &policy, int64(b.GetServerId()), b.GetPlayerId(), now)
				if err != nil {
					return err
				}
				response = &pb.SCKFPlayerInfo{FormationInfo: formation}
				break
			}
			if b.GetType() == 2 && b.GetReplayFlag() == 0 && (b.GetPeakMatchIndex() == 0 || b.GetPeakMatchIndex() == policy.SeasonIndex) {
				formation, err := s.p14PromotionFormationQuery(ctx, tx, &policy, int64(b.GetServerId()), b.GetPlayerId(), now)
				if err != nil {
					return err
				}
				response = &pb.SCKFPlayerInfo{FormationInfo: formation}
				break
			}
			if b.GetReplayFlag() != 0 || b.GetPeakMatchIndex() != 0 && b.GetPeakMatchIndex() != policy.SeasonIndex {
				return errors.New("跨服历史赛季资料尚未开放")
			}
			var target db.P14Member
			if err = tx.First(&target, "season_index=? AND server_id=? AND player_id=?", policy.SeasonIndex, b.GetServerId(), b.GetPlayerId()).Error; err != nil {
				return errors.New("当前赛季没有该联合身份阵容")
			}
			if p14SnapshotDigest(target.Snapshot, target.Battle) != target.SnapshotHash {
				return errors.New("跨服保存阵容损坏")
			}
			formation := &pb.FormationSimpleInfo{}
			if err = proto.Unmarshal(target.Snapshot, formation); err != nil {
				return err
			}
			response = &pb.SCKFPlayerInfo{FormationInfo: formation}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{response}, nil
}

// DispatchP14Cross 接入原认证请求与资料，权威匹配/奖励和竞猜继续保持明确错误边界。
func (s *GameService) DispatchP14Cross(ctx context.Context, id int64, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	now := LocalPlayNow(ctx)
	// 一次请求只读取一次内部时钟，资格、阶段和匹配扣点共用同一事务时刻。
	if clock, ok := ctx.Value(constUtil.P14CrossClockContextKey{}).(func() time.Time); ok {
		now = clock()
	}
	var messages []proto.Message
	var err error
	switch request.(type) {
	case *pb.CSKFEnterGame, *pb.CSKFEnterFrightGame:
		messages, err = s.p14Register(ctx, id, request, stamp, now)
	case *pb.CSKFLogin, *pb.CSKFOpenFrightServerInfo, *pb.CSKFPlayerInfo, *pb.CSKFTimeInfo:
		messages, err = s.p14Query(ctx, id, request, now)
	case *pb.CSKuaFuMatchResult:
		messages, err = s.p14AuditionResult(ctx, id, now)
	case *pb.CSKFReplayInfo:
		if request.(*pb.CSKFReplayInfo).GetType() == 3 {
			messages, err = s.P14FinalReplayInfoAt(ctx, id, request.(*pb.CSKFReplayInfo), now)
		} else {
			messages, err = s.p14AuditionAutoReplayInfo(ctx, id, request.(*pb.CSKFReplayInfo), now)
		}
	case *pb.CSKFReplay:
		messages, err = s.P14FinalReplayAt(ctx, id, request.(*pb.CSKFReplay), now)
	case *pb.CSKFRankInfo:
		if request.(*pb.CSKFRankInfo).GetType() != 2 {
			return nil, true, errors.New(constUtil.P14ResultsUnverified)
		}
		messages, err = s.p14PromotionRankReadAt(ctx, request.(*pb.CSKFRankInfo), now)
	case *pb.CSKuafuMatchPlayer:
		messages, err = s.p14PromotionFightAt(ctx, id, stamp, now)
	case *pb.CSKFMatchFrightList:
		messages, err = s.p14FinalDrawReadAt(ctx, request.(*pb.CSKFMatchFrightList), now)
	case *pb.CSKFGuess:
		if request.(*pb.CSKFGuess).GetGuessType() == constUtil.P14PairGuessType {
			messages, err = s.p14PairGuessAt(ctx, id, stamp, request.(*pb.CSKFGuess), now)
		} else {
			messages, err = s.p14GuessAt(ctx, id, stamp, request.(*pb.CSKFGuess), now)
		}
	case *pb.CSKFGuessList:
		messages, err = s.p14GuessListAt(ctx, id, now)
	case *pb.CSKuafuFightRecord, *pb.CSKFRewardInfo, *pb.CSKFLastSessionResult:
		return nil, true, errors.New(constUtil.P14ResultsUnverified)
	default:
		return nil, false, nil
	}
	return messages, true, err
}

// p14OriginalParticipation preserves the original 0=not eliminated / actual failure-stage wire.
// A missing future qualification is an explicit pending error, never boolean1 or a made-up loss.
func p14OriginalParticipation(stage int32, registered, admitted bool, eliminated int32) (int32, error) {
	if !registered {
		if eliminated != 0 {
			return 0, errors.New(constUtil.P14ParticipationCorrupt)
		}
		return constUtil.P14ParticipationEligible, nil
	}
	if eliminated != 0 {
		switch eliminated {
		case constUtil.P14EliminatedAudition, constUtil.P14EliminatedPromotion, constUtil.P14EliminatedFinal16, constUtil.P14EliminatedFinal8, constUtil.P14EliminatedFinal4, constUtil.P14EliminatedFinalHalf, constUtil.P14EliminatedFinalThird, constUtil.P14EliminatedFinal:
		default:
			return 0, errors.New(constUtil.P14ParticipationCorrupt)
		}
		if admitted || eliminated > stage {
			return 0, errors.New(constUtil.P14ParticipationCorrupt)
		}
		return eliminated, nil
	}
	if admitted || stage <= constUtil.P14RegisterStage {
		return constUtil.P14ParticipationEligible, nil
	}
	return 0, errors.New(constUtil.P14ParticipationPending)
}
