package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func p14PromotionFormationHash(row *db.P14PromotionFormation) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

func p14PromotionFormationID(request model.P14SnapshotImportRequest) string {
	return p6Digest([]byte(fmt.Sprintf("p14/promotion/%d/%d/%d/%d/%d", request.SeasonIndex, request.SourceServerID, request.PlayerID, request.Revision, request.DestinationServerID)))
}

// p14PromotionFormationEligibility checks the actual local or signed foreign
// qualification at capture time; the original audition payload is never changed.
func (s *GameService) p14PromotionFormationEligibility(ctx context.Context, tx *gorm.DB, policy *db.P14SeasonPolicy, server, player int64, at time.Time) (string, int64, int64, error) {
	return s.p14PromotionFormationEligibilityAtStage(ctx, tx, policy, server, player, at, constUtil.P14PromotionStage)
}

// The explicit legacy stage is used only to read an already frozen historical row.
func (s *GameService) p14PromotionFormationEligibilityAtStage(ctx context.Context, tx *gorm.DB, policy *db.P14SeasonPolicy, server, player int64, at time.Time, expectedStage int32) (string, int64, int64, error) {
	var gate db.P14PromotionPolicy
	if err := tx.First(&gate, 1).Error; err != nil {
		return "", 0, 0, err
	}
	if !policy.Enabled || !gate.Enabled || gate.Source != constUtil.P14PromotionSource {
		return "", 0, 0, errors.New("晋级赛独立规则尚未开放")
	}
	if _, err := p14PromotionDecode(gate.RulesJSON, gate.RulesHash); err != nil {
		return "", 0, 0, err
	}
	stage, _, err := s.p14SeasonStage(tx, policy, at)
	if err != nil || stage != expectedStage {
		return "", 0, 0, errors.New("当前不在原晋级赛阵容上传窗口")
	}
	slots, err := s.p14SeasonCalendar(tx, policy.SeasonIndex)
	if err != nil {
		return "", 0, 0, err
	}
	var start, end int64
	for _, slot := range slots {
		if slot.Stage == expectedStage {
			start, end = slot.StartAt, slot.EndAt
		}
	}
	if start <= 0 || end <= start || end-start > constUtil.P14PromotionMaxStageMillis {
		return "", 0, 0, errors.New("原晋级赛完整时刻尚未发布")
	}
	var qualificationHash string
	var playerIDs []int64
	if server == s.OriginalServerID() {
		qualified, err := s.p14AuditionQualificationRead(ctx, tx, policy.SeasonIndex, server)
		if err != nil {
			return "", 0, 0, err
		}
		if qualified.QualifiedAt > at.UnixMilli() || json.Unmarshal(qualified.Players, &playerIDs) != nil {
			return "", 0, 0, errors.New("原晋级资格尚未成立")
		}
		qualificationHash = qualified.SourceHash
	} else {
		var saved db.P14ForeignQualification
		if err = tx.First(&saved, "season_index=? AND server_id=?", policy.SeasonIndex, server).Error; err != nil {
			return "", 0, 0, err
		}
		var request model.P14QualificationImportRequest
		if p6Digest(saved.Payload) != saved.PayloadHash || json.Unmarshal(saved.Payload, &request) != nil || request.DestinationServerID != s.OriginalServerID() || request.SourceServerID != server || request.SeasonIndex != policy.SeasonIndex || request.DeliveryID != saved.DeliveryID {
			return "", 0, 0, errors.New("晋级阵容缺真实外服资格来源")
		}
		_, boundary, qualified, err := p14QualificationProof(request, at)
		if err != nil || boundary.SourceHash != saved.BoundaryHash || qualified.SourceHash != saved.QualificationHash || json.Unmarshal(qualified.Players, &playerIDs) != nil {
			return "", 0, 0, errors.New("晋级阵容外服资格证书无效")
		}
		qualificationHash = qualified.SourceHash
	}
	for _, id := range playerIDs {
		if id == player {
			return qualificationHash, start, end, nil
		}
	}
	return "", 0, 0, errors.New("该联合角色没有真实晋级资格")
}

func p14PromotionFormationRead(row *db.P14PromotionFormation) (*pb.FormationSimpleInfo, error) {
	formation := &pb.FormationSimpleInfo{}
	battle := &pb.BattlePlayer{}
	if row.Source != constUtil.P14PromotionFormationSource || row.SourceHash != p14PromotionFormationHash(row) || row.SnapshotHash != p14SnapshotDigest(row.Snapshot, row.Battle) || row.CapturedAt < row.StageStart || row.CapturedAt >= row.StageEnd || row.Revision <= 0 || proto.Unmarshal(row.Snapshot, formation) != nil || proto.Unmarshal(row.Battle, battle) != nil || battle.GetPlayerId() != row.PlayerID || int64(battle.GetServerId()) != row.ServerID || formation.GetFightPower() != row.FightPower || len(battle.Formation) == 0 {
		return nil, errors.New("晋级赛实际上传阵容与冻结来源不符")
	}
	return formation, nil
}

// p14PromotionFormationUpload saves an independent actual profile capture and
// outbox in the same locked transaction. Qualification and audition remain read-only.
func (s *GameService) p14PromotionFormationUpload(ctx context.Context, tx *gorm.DB, policy *db.P14SeasonPolicy, player *pb.Player, game *model.GameState, now time.Time) (*db.P14PromotionFormation, error) {
	server, id := s.OriginalServerID(), player.GetPlayerId()
	qualified, start, end, err := s.p14PromotionFormationEligibility(ctx, tx, policy, server, id, now)
	if err != nil {
		return nil, err
	}
	var row db.P14PromotionFormation
	err = tx.First(&row, "season_index=? AND server_id=? AND player_id=?", policy.SeasonIndex, server, id).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err == nil {
		if _, err = p14PromotionFormationRead(&row); err != nil || row.QualificationHash != qualified || row.StageStart != start || row.StageEnd != end || row.CapturedAt > now.UnixMilli() {
			return nil, errors.New("晋级阵容既有版本与真实阶段资格不符")
		}
	}
	if row.Revision == math.MaxInt64 {
		return nil, errors.New("晋级阵容版本溢出")
	}
	battle, err := s.battlePlayer(player, game)
	if err != nil {
		return nil, err
	}
	formation, err := s.p10FormationProfile(tx, id)
	if err != nil {
		return nil, err
	}
	encodedBattle, err := proto.Marshal(battle)
	if err != nil {
		return nil, err
	}
	encodedFormation, err := proto.Marshal(formation)
	if err != nil {
		return nil, err
	}
	configHash, err := s.p14ConfigurationHash()
	if err != nil {
		return nil, err
	}
	row = db.P14PromotionFormation{SeasonIndex: policy.SeasonIndex, ServerID: server, PlayerID: id, Revision: row.Revision + 1, CapturedAt: now.UnixMilli(), FightPower: formation.GetFightPower(), Snapshot: encodedFormation, SnapshotHash: p14SnapshotDigest(encodedFormation, encodedBattle), Battle: encodedBattle, ClientVersion: constUtil.P6OriginalClientHash, EngineVersion: constUtil.P6EngineVersion, ConfigurationHash: configHash, QualificationHash: qualified, StageStart: start, StageEnd: end, Source: constUtil.P14PromotionFormationSource}
	row.SourceHash = p14PromotionFormationHash(&row)
	if _, err = p14PromotionFormationRead(&row); err != nil {
		return nil, err
	}
	if err = tx.Save(&row).Error; err != nil {
		return nil, err
	}
	var peers []db.P14TrustedPeer
	if err = tx.Where("enabled=1 AND server_id<>?", server).Find(&peers).Error; err != nil {
		return nil, err
	}
	if len(peers) == 0 {
		return nil, errors.New("晋级阵容可信目的服尚未发布")
	}
	for _, peer := range peers {
		if len(peer.SigningKey) < 32 || peer.Endpoint == "" {
			return nil, errors.New("晋级阵容可信目的地址缺失")
		}
		request := model.P14SnapshotImportRequest{ProtocolVersion: constUtil.P14PromotionFormationProtocolVersion, SourceServerID: server, DestinationServerID: peer.ServerID, SeasonIndex: row.SeasonIndex, PlayerID: id, Revision: row.Revision, CapturedAt: row.CapturedAt, FightPower: row.FightPower, Formation: row.Snapshot, Battle: row.Battle, SnapshotHash: row.SnapshotHash, ConfigurationHash: row.ConfigurationHash, ClientVersion: row.ClientVersion, EngineVersion: row.EngineVersion}
		request.DeliveryID = p14PromotionFormationID(request)
		raw, err := json.Marshal(request)
		if err != nil {
			return nil, err
		}
		if err = tx.Create(&db.P14Delivery{ID: request.DeliveryID, DestinationServerID: peer.ServerID, Payload: raw, CreatedAt: now.UnixMilli()}).Error; err != nil {
			return nil, err
		}
	}
	return &row, nil
}

// p14PromotionFormationImport is called only after exact HMAC and deployment
// checks. Delayed delivery is validated at the real signed capture time.
func (s *GameService) p14PromotionFormationImport(ctx context.Context, tx *gorm.DB, policy *db.P14SeasonPolicy, request model.P14SnapshotImportRequest, now time.Time) error {
	if request.SeasonIndex != policy.SeasonIndex || request.CapturedAt > now.UnixMilli() || request.Revision <= 0 {
		return errors.New("晋级阵容赛季或实际采集时刻无效")
	}
	qualification, start, end, err := s.p14PromotionFormationEligibility(ctx, tx, policy, request.SourceServerID, request.PlayerID, time.UnixMilli(request.CapturedAt))
	if err != nil {
		return err
	}
	configHash, err := s.p14ConfigurationHash()
	if err != nil {
		return err
	}
	if request.ClientVersion != constUtil.P6OriginalClientHash || request.EngineVersion != constUtil.P6EngineVersion || request.ConfigurationHash != configHash {
		return errors.New("新晋级阵容与当前原配置引擎不兼容")
	}
	row := db.P14PromotionFormation{SeasonIndex: request.SeasonIndex, ServerID: request.SourceServerID, PlayerID: request.PlayerID, Revision: request.Revision, CapturedAt: request.CapturedAt, FightPower: request.FightPower, Snapshot: append([]byte(nil), request.Formation...), SnapshotHash: request.SnapshotHash, Battle: append([]byte(nil), request.Battle...), ClientVersion: request.ClientVersion, EngineVersion: request.EngineVersion, ConfigurationHash: request.ConfigurationHash, QualificationHash: qualification, StageStart: start, StageEnd: end, Source: constUtil.P14PromotionFormationSource}
	row.SourceHash = p14PromotionFormationHash(&row)
	if _, err = p14PromotionFormationRead(&row); err != nil {
		return err
	}
	var previous db.P14PromotionFormation
	err = tx.First(&previous, "season_index=? AND server_id=? AND player_id=?", row.SeasonIndex, row.ServerID, row.PlayerID).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if err == nil {
		if _, err = p14PromotionFormationRead(&previous); err != nil {
			return err
		}
		if previous.Revision == row.Revision && previous.SourceHash != row.SourceHash || previous.QualificationHash != row.QualificationHash || previous.StageStart != row.StageStart || previous.StageEnd != row.StageEnd || row.Revision > previous.Revision && row.CapturedAt < previous.CapturedAt {
			return errors.New("晋级阵容同版本内容或真实资格冲突")
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || row.Revision > previous.Revision {
		if err = tx.Save(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *GameService) p14PromotionFormationQuery(ctx context.Context, tx *gorm.DB, policy *db.P14SeasonPolicy, server, player int64, now time.Time) (*pb.FormationSimpleInfo, error) {
	var row db.P14PromotionFormation
	readErr := tx.First(&row, "season_index=? AND server_id=? AND player_id=?", policy.SeasonIndex, server, player).Error
	if readErr != nil && !errors.Is(readErr, gorm.ErrRecordNotFound) {
		return nil, readErr
	}
	at, expectedStage := now, constUtil.P14PromotionStage
	if readErr == nil {
		// Read the original frozen window without rewriting its source or SHA.
		if row.CapturedAt > now.UnixMilli() {
			return nil, errors.New("晋级阵容冻结时刻尚未到达")
		}
		at = time.UnixMilli(row.CapturedAt)
		capturedStage, _, stageErr := s.p14SeasonStage(tx, policy, at)
		if stageErr != nil || capturedStage != constUtil.P14PromotionStage && capturedStage != constUtil.P14LegacyPromotionStage {
			return nil, errors.New("晋级历史冻结窗口无效")
		}
		expectedStage = capturedStage
	}
	qualified, start, end, err := s.p14PromotionFormationEligibilityAtStage(ctx, tx, policy, server, player, at, expectedStage)
	if err != nil {
		return nil, err
	}
	err = readErr
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// A player who has not uploaded uses the genuine frozen qualified roster.
		var roster []db.P14AuditionRoster
		if server == s.OriginalServerID() {
			_, roster, err = p14AuditionRead(tx, policy.SeasonIndex, server)
		} else {
			roster, err = s.ReadP14ForeignQualifiedEntrants(ctx, tx, policy.SeasonIndex, now)
		}
		if err != nil {
			return nil, err
		}
		for _, item := range roster {
			if item.PlayerID == player && item.ServerID == server {
				frame := &pb.FormationSimpleInfo{}
				if err = proto.Unmarshal(item.Snapshot, frame); err != nil {
					return nil, err
				}
				return frame, nil
			}
		}
		return nil, errors.New("晋级角色真实截止阵容缺失")
	}
	if err != nil {
		return nil, err
	}
	if row.QualificationHash != qualified || row.StageStart != start || row.StageEnd != end || row.CapturedAt > now.UnixMilli() {
		return nil, errors.New("晋级阵容不属于当前实际资格")
	}
	return p14PromotionFormationRead(&row)
}
