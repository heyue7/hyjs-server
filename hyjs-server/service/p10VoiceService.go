package service

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *GameService) InitializeP10Voice(ctx context.Context) error {
	tx := db.GetIns().WithContext(ctx)
	if err := tx.AutoMigrate(&db.P10VoicePolicy{}, &db.P10VoiceAsset{}, &db.P10VoiceMessage{}); err != nil {
		return err
	}
	return tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&db.P10VoicePolicy{ID: 1, Source: constUtil.P10VoiceSource}).Error
}
func p10VoiceAssetHash(row *db.P10VoiceAsset) string {
	copy := *row
	copy.MetaHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}
func p10VoicePolicy(tx *gorm.DB) (*db.P10VoicePolicy, error) {
	var row db.P10VoicePolicy
	if err := tx.First(&row, 1).Error; err != nil {
		return nil, err
	}
	if !row.Enabled {
		return nil, errors.New(constUtil.P10VoiceClosed)
	}
	if row.RetentionMillis < 60000 || row.RetentionMillis > 604800000 || row.MinDurationMillis < 20 || row.MinDurationMillis > 1000 || strings.TrimSpace(row.Source) == "" {
		return nil, errors.New("本地真实语音保留/最短时长策略缺失")
	}
	return &row, nil
}
func p10VoiceAuth(tx *gorm.DB, id int64, session []byte) (*db.Profile, *pb.Player, error) {
	var row db.Profile
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&row, id).Error; err != nil {
		return nil, nil, err
	}
	if len(session) != 16 || subtle.ConstantTimeCompare(row.Session, session) != 1 {
		return nil, nil, errors.New("当前本地会话无效")
	}
	player, _, err := decodeProfile(&row)
	return &row, player, err
}
func (s *GameService) p10VoiceCanSend(player *pb.Player) error {
	level, err := s.p10Setting("ChatLevel")
	if err != nil {
		return err
	}
	vip, err := s.p10Setting("ChatVipLevel")
	if err != nil {
		return err
	}
	if player.GetLevel() < level && player.GetVip() < vip {
		return errors.New("原聊天需20级或VIP2")
	}
	return nil
}
func (s *GameService) p10VoiceScope(tx *gorm.DB, id int64, kind int32, live bool) (string, error) {
	if live && kind != 3 && !s.p10ChannelOnline(tx, id) {
		return "", errors.New("语音频道需要当前认证TCP")
	}
	switch kind {
	case 3:
		var policy db.P10SocialPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return "", err
		}
		if !policy.Enabled {
			return "", errors.New(constUtil.P10SocialClosed)
		}
		return "person", nil
	case 1:
		var policy db.P10ChannelPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return "", err
		}
		if !policy.WorldEnabled {
			return "", errors.New(constUtil.P10ChannelClosed)
		}
		return "world/1", nil
	case 2:
		enabled, err := P13GuildChannelEnabled(tx)
		if err != nil {
			return "", err
		}
		if !enabled {
			return "", errors.New("当前公会频道已关闭")
		}
		guild, _, err := P13GuildScope(tx, id)
		if err != nil || guild == "" {
			return "", errors.New("当前角色不属于实际公会")
		}
		return "guild/" + guild, nil
	case 5:
		var policy db.P10ChannelPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return "", err
		}
		if !policy.FieldEnabled {
			return "", errors.New(constUtil.P10ChannelClosed)
		}
		var presence db.P10ScenePresence
		if err := tx.First(&presence, "player_id=?", id).Error; err != nil {
			return "", err
		}
		var profile db.Profile
		if err := tx.First(&profile, id).Error; err != nil {
			return "", err
		}
		_, game, err := decodeProfile(&profile)
		if err != nil {
			return "", err
		}
		if presence.Scene != "field_001" || p10SceneScope(game) != "field_001" {
			return "", errors.New("当前角色不属于原野外语音频道")
		}
		if live {
			if _, ok := s.p10SceneLive(tx, presence); !ok {
				return "", errors.New("当前野外会话已失效")
			}
		}
		return fmt.Sprintf("field/%s/%d", presence.Scene, presence.Line), nil
	case 6:
		var policy db.P13UnionFightChatPolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return "", err
		}
		if !policy.TextEnabled {
			return "", errors.New("当前公会战频道已关闭")
		}
		actor, match, _, err := s.p13UFChatIdentity(tx, id, p13UFNow(tx.Statement.Context))
		if err != nil {
			return "", err
		}
		if p13UFNow(tx.Statement.Context).UnixMilli() >= match.EndAt {
			return "", errors.New("本场语音已结束")
		}
		var profile db.Profile
		if err = tx.First(&profile, id).Error; err != nil {
			return "", err
		}
		_, game, err := decodeProfile(&profile)
		if err != nil || game.PositionLoading {
			return "", errors.New("原当前场景加载未完成")
		}
		return "war/" + match.ID + "/" + actor.GuildID, nil
	}
	return "", errors.New("原语音频道尚未支持")
}

// P10VoiceConfig 对当前会话返回真实开启状态，不申请任何原GCloud外部messageKey。
func (s *GameService) P10VoiceConfig(ctx context.Context, id int64, session []byte, kind int32) (*model.P10VoiceConfigResponse, error) {
	out := &model.P10VoiceConfigResponse{MaxSeconds: 30, RecordingGapSeconds: 1, MaxBytes: constUtil.P10VoiceMaxBytes}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		_, player, err := p10VoiceAuth(tx, id, session)
		if err != nil {
			return err
		}
		var policy db.P10VoicePolicy
		if err = tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Enabled {
			return nil
		}
		cfg, err := p10VoicePolicy(tx)
		if err != nil {
			return err
		}
		max, err := s.p10Setting("audioMaxTime")
		if err != nil || max != 30 {
			return errors.New("原语音30秒上限来源损坏")
		}
		gap, err := s.p10Setting("audioNapTime")
		if err != nil || gap != 1 {
			return errors.New("原语音1秒录音操作间隔来源损坏")
		}
		out.Enabled = true
		out.Adapter = 1
		out.CanRecord = s.p10VoiceCanSend(player) == nil
		out.MinDurationMillis = cfg.MinDurationMillis
		if kind != 0 {
			out.Scope, err = s.p10VoiceScope(tx, id, kind, true)
			return err
		}
		return nil
	})
	return out, err
}

// P10VoiceKVInfo 原登录先从audiosystem设置界面并初始化语音；只公开本地实际策略，自动播放保持具名默认关闭。
func (s *GameService) P10VoiceKVInfo(ctx context.Context, id int64, session []byte) ([]*pb.KeyValueEntry, error) {
	cfg, err := s.P10VoiceConfig(ctx, id, session, 0)
	if err != nil {
		return nil, err
	}
	value := "off"
	if cfg.Enabled {
		value = "on"
	}
	return []*pb.KeyValueEntry{{Key: proto.String("audiosystem"), Value: proto.String(value)}, {Key: proto.String("audioautoget"), Value: proto.String("off")}}, nil
}

// 仅接受明确本地PCM16单声道16k规范WAV，原SDK私有云格式没有被冒充为本地音频。
func p10VoiceWAV(raw []byte) (int64, error) {
	if len(raw) < 44 || int64(len(raw)) > constUtil.P10VoiceMaxBytes || !bytes.Equal(raw[:4], []byte("RIFF")) || !bytes.Equal(raw[8:12], []byte("WAVE")) || int64(binary.LittleEndian.Uint32(raw[4:8]))+8 != int64(len(raw)) {
		return 0, errors.New("真实PCM WAV长度/RIFF标识不符")
	}
	if !bytes.Equal(raw[12:16], []byte("fmt ")) || binary.LittleEndian.Uint32(raw[16:20]) != 16 || binary.LittleEndian.Uint16(raw[20:22]) != 1 || binary.LittleEndian.Uint16(raw[22:24]) != 1 || binary.LittleEndian.Uint32(raw[24:28]) != 16000 || binary.LittleEndian.Uint32(raw[28:32]) != 32000 || binary.LittleEndian.Uint16(raw[32:34]) != 2 || binary.LittleEndian.Uint16(raw[34:36]) != 16 || !bytes.Equal(raw[36:40], []byte("data")) || int(binary.LittleEndian.Uint32(raw[40:44])) != len(raw)-44 || (len(raw)-44)%2 != 0 {
		return 0, errors.New("本地音频须实际PCM16/mono16k规范WAV")
	}
	duration := int64(len(raw)-44) * 1000 / 32000
	if duration <= 0 || duration > 30000 {
		return 0, errors.New("真实录音为空或超过原30秒")
	}
	return duration, nil
}
func p10VoiceReadAsset(row *db.P10VoiceAsset, now time.Time) error {
	if row.MetaHash != p10VoiceAssetHash(row) || row.FileName != constUtil.P10VoicePrefix+row.ID+".wav" || len(row.ID) != 32 || strings.TrimSpace(row.Source) == "" {
		return errors.New("实际音频对象SHA/命名损坏")
	}
	if row.State == constUtil.P10VoiceDeleted || now.UnixMilli() >= row.ExpiresAt {
		return errors.New("实际音频已删除或到期")
	}
	if row.State != constUtil.P10VoiceUploaded && row.State != constUtil.P10VoicePublished {
		return errors.New("实际音频尚未持久完成")
	}
	duration, err := p10VoiceWAV(row.Bytes)
	if err != nil || row.SHA != p6Digest(row.Bytes) || row.ByteLength != int64(len(row.Bytes)) || row.DurationMillis != duration {
		return errors.New("实际音频字节/SHA/时长损坏")
	}
	return nil
}
func p10VoiceUploadView(row *db.P10VoiceAsset) *model.P10VoiceUploadResponse {
	return &model.P10VoiceUploadResponse{FileName: row.FileName, AudioTime: row.DurationMillis / 1000, DurationMillis: row.DurationMillis, FileSize: row.ByteLength, SHA256: row.SHA, ExpiresAt: row.ExpiresAt}
}

// UploadP10Voice 在验证真实录音和当前会话/频道后原子存储；失败不返回fileName或成功占位。
func (s *GameService) UploadP10Voice(ctx context.Context, id int64, session []byte, kind int32, requestID, expectedScope string, raw []byte) (*model.P10VoiceUploadResponse, error) {
	duration, err := p10VoiceWAV(raw)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	var out *model.P10VoiceUploadResponse
	err = db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		_, player, err := p10VoiceAuth(tx, id, session)
		if err != nil {
			return err
		}
		policy, err := p10VoicePolicy(tx)
		if err != nil {
			return err
		}
		if duration < policy.MinDurationMillis {
			return errors.New("真实录音未达到具名本地最短时长")
		}
		if err = s.p10VoiceCanSend(player); err != nil {
			return err
		}
		scope, err := s.p10VoiceScope(tx, id, kind, true)
		if err != nil {
			return err
		}
		if scope != expectedScope {
			return errors.New("录音开始后频道/公会/场次已改变")
		}
		requestHash := p6Digest([]byte(strconv.Itoa(int(kind)) + "/" + scope + "/" + p6Digest(raw)))
		var prior db.P10VoiceAsset
		err = tx.First(&prior, "owner_id=? AND request_id=?", id, requestID).Error
		if err == nil {
			if prior.RequestID != requestID || prior.RequestHash != requestHash || prior.OwnerSessionHash != p6Digest(session) || prior.Scope != scope || prior.ChatType != int64(kind) {
				return errors.New("实际语音上传nonce/会话冲突")
			}
			if err = p10VoiceReadAsset(&prior, now); err != nil {
				return err
			}
			out = p10VoiceUploadView(&prior)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var pending, total int64
		if err = tx.Model(&db.P10VoiceAsset{}).Where("owner_id=? AND state=? AND expires_at>?", id, constUtil.P10VoiceUploaded, now.UnixMilli()).Count(&pending).Error; err != nil {
			return err
		}
		if err = tx.Model(&db.P10VoiceAsset{}).Where("owner_id=? AND state<>? AND expires_at>?", id, constUtil.P10VoiceDeleted, now.UnixMilli()).Count(&total).Error; err != nil {
			return err
		}
		if pending >= 3 || total >= 50 {
			return errors.New("真实语音未发送/保留对象达到具名本地3/50上限")
		}
		token := make([]byte, 16)
		if _, err = rand.Read(token); err != nil {
			return err
		}
		opaque := hex.EncodeToString(token)
		asset := db.P10VoiceAsset{ID: opaque, FileName: constUtil.P10VoicePrefix + opaque + ".wav", OwnerID: id, RequestID: requestID, RequestHash: requestHash, OwnerSessionHash: p6Digest(session), ChatType: int64(kind), Scope: scope, State: constUtil.P10VoiceUploaded, Bytes: append([]byte(nil), raw...), SHA: p6Digest(raw), ByteLength: int64(len(raw)), DurationMillis: duration, CreatedAt: now.UnixMilli(), ExpiresAt: now.UnixMilli() + policy.RetentionMillis, Source: constUtil.P10VoiceSource + "；发布：" + policy.Source}
		asset.MetaHash = p10VoiceAssetHash(&asset)
		if err = tx.Create(&asset).Error; err != nil {
			return err
		}
		out = p10VoiceUploadView(&asset)
		return nil
	})
	return out, err
}
func (s *GameService) p10VoiceAccess(tx *gorm.DB, id int64, session []byte, asset *db.P10VoiceAsset, now time.Time) error {
	return s.p10VoiceReadAccess(tx, id, session, asset, now, true)
}

func (s *GameService) p10VoiceReadAccess(tx *gorm.DB, id int64, session []byte, asset *db.P10VoiceAsset, now time.Time, requireCurrentTCP bool) error {
	if _, _, err := p10VoiceAuth(tx, id, session); err != nil {
		return err
	}
	if _, err := p10VoicePolicy(tx); err != nil {
		return err
	}
	if err := p10VoiceReadAsset(asset, now); err != nil {
		return err
	}
	if asset.State == constUtil.P10VoiceUploaded {
		if id != asset.OwnerID || asset.OwnerSessionHash != p6Digest(session) {
			return errors.New("未发送真实语音仅原上传会话可读")
		}
	}
	if asset.State == constUtil.P10VoicePublished {
		var message db.P10VoiceMessage
		if err := tx.First(&message, "asset_id=?", asset.ID).Error; err != nil {
			return err
		}
		if _, err := p10VoiceMessageFrame(&message, asset); err != nil {
			return err
		}
	}
	if asset.ChatType == 3 && asset.State == constUtil.P10VoicePublished {
		blocked, err := p10Blocked(tx, asset.OwnerID, asset.TargetID)
		if err != nil {
			return err
		}
		if blocked {
			return errors.New("当前私聊双方黑名单禁止旧语音读取")
		}
	}
	if asset.ChatType == 3 && id != asset.OwnerID && id != asset.TargetID {
		return errors.New("实际私聊语音接收人不匹配")
	}
	viewerLive := id != asset.OwnerID || asset.ChatType != 3
	// 原登录先读HTTP缓存再连接TCP；世界历史仅提供元数据，播放字节仍走当前TCP校验。
	if !requireCurrentTCP && asset.ChatType == int64(constUtil.P10WorldChatType) {
		viewerLive = false
	}
	viewer, err := s.p10VoiceScope(tx, id, int32(asset.ChatType), viewerLive)
	if err != nil || viewer != asset.Scope {
		return errors.New("当前语音频道权限已撤销")
	}
	owner, err := s.p10VoiceScope(tx, asset.OwnerID, int32(asset.ChatType), false)
	if err != nil || owner != asset.Scope {
		return errors.New("原语音发送来源已失去当前频道资格")
	}
	if id != asset.OwnerID {
		blocked, err := p10Blocked(tx, id, asset.OwnerID)
		if err != nil {
			return err
		}
		if blocked {
			return errors.New("当前黑名单禁止语音读取")
		}
	}
	return nil
}

// DownloadP10Voice 每次实际播放都重新执行权限和完整性检查，客户端缓存不得绕过本入口。
func (s *GameService) DownloadP10Voice(ctx context.Context, id int64, session []byte, fileName string) ([]byte, error) {
	// 事务外只取不可变对象所有人以确定锁顺序，真实字节/会员快照在全部Profile锁取得后读取。
	var owner db.P10VoiceAsset
	if err := db.GetIns().WithContext(ctx).Select("owner_id").First(&owner, "file_name=?", fileName).Error; err != nil {
		return nil, err
	}
	var data []byte
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if _, err := p10LockProfiles(tx, id, owner.OwnerID); err != nil {
			return err
		}
		var asset db.P10VoiceAsset
		if err := tx.First(&asset, "file_name=?", fileName).Error; err != nil {
			return err
		}
		if asset.OwnerID != owner.OwnerID {
			return errors.New("实际语音所有权映射已改变")
		}
		if err := s.p10VoiceAccess(tx, id, session, &asset, time.Now()); err != nil {
			return err
		}
		data = append([]byte(nil), asset.Bytes...)
		return nil
	})
	return data, err
}

// DeleteP10Voice 当前会话只能清除自己真实存在的对象，退会/过期也允许本人清理。
func (s *GameService) DeleteP10Voice(ctx context.Context, id int64, session []byte, fileName string) (*model.P10VoiceDeleteResponse, error) {
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if _, _, err := p10VoiceAuth(tx, id, session); err != nil {
			return err
		}
		var asset db.P10VoiceAsset
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&asset, "file_name=?", fileName).Error; err != nil {
			return err
		}
		if asset.MetaHash != p10VoiceAssetHash(&asset) || asset.OwnerID != id {
			return errors.New("真实语音删除所有权不符")
		}
		asset.Bytes = []byte{}
		asset.State = constUtil.P10VoiceDeleted
		asset.MetaHash = p10VoiceAssetHash(&asset)
		return tx.Save(&asset).Error
	})
	if err != nil {
		return nil, err
	}
	return &model.P10VoiceDeleteResponse{Deleted: true}, nil
}

// TickP10Voice 只分批扫描到期对象ID，逐行锁定并清字节，避免把所有LONGBLOB装入回收器内存。
func (s *GameService) TickP10Voice(ctx context.Context, now time.Time) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var candidates []struct{ ID string }
		if err := db.GetIns().WithContext(ctx).Model(&db.P10VoiceAsset{}).Select("id").Where("state<>? AND expires_at<=?", constUtil.P10VoiceDeleted, now.UnixMilli()).Order("id").Limit(constUtil.P10VoiceSweepBatchSize).Find(&candidates).Error; err != nil {
			return err
		}
		if len(candidates) == 0 {
			return nil
		}
		for _, row := range candidates {
			if err := ctx.Err(); err != nil {
				return err
			}
			err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
				if err := ServerGameClockGuardTransaction(tx); err != nil {
					return err
				}

				var current db.P10VoiceAsset
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&current, "id=?", row.ID).Error; err != nil {
					return err
				}
				// 并发回收/本人删除和TTL修订在当前行锁下复核，保持已删除对象和未来对象不变。
				if current.ExpiresAt > now.UnixMilli() || current.State == constUtil.P10VoiceDeleted {
					return nil
				}
				current.Bytes = []byte{}
				current.State = constUtil.P10VoiceDeleted
				current.MetaHash = p10VoiceAssetHash(&current)
				return tx.Save(&current).Error
			})
			if err != nil {
				return err
			}
		}
	}
}

// ServeP10Voice 启动即清到期字节，随后按具名本地一分钟周期清理；服务取消停止回收。
func (s *GameService) ServeP10Voice(ctx context.Context) error {
	if err := s.TickP10Voice(ctx, time.Now()); err != nil {
		return err
	}
	ticker := time.NewTicker(constUtil.P10VoiceSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case at := <-ticker.C:
			if err := s.TickP10Voice(ctx, at); err != nil {
				return err
			}
		}
	}
}

func p10VoiceMessageFrame(row *db.P10VoiceMessage, asset *db.P10VoiceAsset) (*pb.SCChatTcp, error) {
	frame := &pb.SCChatTcp{}
	if row.FrameHash != p6Digest(row.Frame) || row.AssetID != asset.ID || row.OwnerID != asset.OwnerID || row.ChatType != asset.ChatType || row.Scope != asset.Scope || row.TargetID != asset.TargetID || proto.Unmarshal(row.Frame, frame) != nil || frame.GetChatType() != int32(asset.ChatType) || frame.GetIsAudio() != 2 || frame.GetDownLoadUrl() != asset.FileName || frame.GetAudioTime() != int32(asset.DurationMillis/1000) || frame.GetPlay().GetPlayerId() != asset.OwnerID {
		return nil, errors.New("真实原语音消息帧/对象映射损坏")
	}
	return frame, nil
}

// 同一实际上传对象只能发送一次，原TCP没有时间戳也能幂等读取原帧；重试仍先检查当前撤权。
func (s *GameService) p10VoicePublish(ctx context.Context, id int64, session []byte, body *pb.CSChatTcp) ([]proto.Message, error) {
	p10SceneMutation.Lock()
	defer p10SceneMutation.Unlock()
	var frame *pb.SCChatTcp
	var asset db.P10VoiceAsset
	fresh := false
	now := time.Now()
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		if body.GetChatType() == 3 {
			if body.GetPlayerId() <= 0 || body.GetPlayerId() == id {
				return errors.New("原私聊语音目标不符")
			}
			if _, err := p10LockProfiles(tx, id, body.GetPlayerId()); err != nil {
				return err
			}
		}
		_, player, err := p10VoiceAuth(tx, id, session)
		if err != nil {
			return err
		}
		if _, err = p10VoicePolicy(tx); err != nil {
			return err
		}
		if err = s.p10VoiceCanSend(player); err != nil {
			return err
		}
		scope, err := s.p10VoiceScope(tx, id, body.GetChatType(), true)
		if err != nil {
			return err
		}
		if err = tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&asset, "file_name=?", body.GetDownLoadUrl()).Error; err != nil {
			return err
		}
		if err = p10VoiceReadAsset(&asset, now); err != nil {
			return err
		}
		if asset.OwnerID != id || asset.ChatType != int64(body.GetChatType()) || asset.Scope != scope || body.GetIsAudio() != 2 || body.GetFileName() != "" && body.GetFileName() != asset.FileName || body.GetAudioTime() != int32(asset.DurationMillis/1000) || body.GetFileSize() != asset.ByteLength {
			return errors.New("原语音引用必须对应本人真实当前上传/时长/字节")
		}
		target := int64(0)
		if body.GetChatType() == 3 {
			target = body.GetPlayerId()
			blocked, err := p10Blocked(tx, id, target)
			if err != nil {
				return err
			}
			if blocked {
				return errors.New("当前黑名单禁止语音发送")
			}
		}
		raw, err := proto.MarshalOptions{Deterministic: true}.Marshal(body)
		if err != nil {
			return err
		}
		requestHash := p6Digest(raw)
		var prior db.P10VoiceMessage
		err = tx.First(&prior, "asset_id=?", asset.ID).Error
		if err == nil {
			if prior.RequestHash != requestHash || asset.State != constUtil.P10VoicePublished || asset.TargetID != target {
				return errors.New("真实语音对象重发目标/内容冲突")
			}
			frame, err = p10VoiceMessageFrame(&prior, &asset)
			return err
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if asset.State != constUtil.P10VoiceUploaded || asset.OwnerSessionHash != p6Digest(session) {
			return errors.New("旧上传会话或非真实待发送对象不能新发")
		}
		var last db.P10SocialChatState
		err = tx.First(&last, "player_id=?", id).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		interval, err := s.p10Setting("ChatInterval")
		if err != nil || interval < 1 || interval > 86400 {
			return errors.New("原既有聊天间隔来源不符")
		}
		if last.LastSendAt > 0 && now.UnixMilli()-last.LastSendAt < int64(interval)*1000 {
			return errors.New("本地既有聊天发送间隔未到")
		}
		info, err := s.p10Player(tx, id)
		if err != nil {
			return err
		}
		frame = &pb.SCChatTcp{ChatType: proto.Int32(body.GetChatType()), Content: proto.String(body.GetContent()), Time: proto.Int64(now.UnixMilli()), Play: info, IsAudio: proto.Int32(2), DownLoadUrl: proto.String(asset.FileName), AudioTime: proto.Int32(int32(asset.DurationMillis / 1000))}
		wire, err := proto.MarshalOptions{Deterministic: true}.Marshal(frame)
		if err != nil {
			return err
		}
		asset.TargetID = target
		asset.State = constUtil.P10VoicePublished
		asset.MetaHash = p10VoiceAssetHash(&asset)
		message := db.P10VoiceMessage{AssetID: asset.ID, OwnerID: id, TargetID: target, ChatType: asset.ChatType, Scope: asset.Scope, Frame: wire, FrameHash: p6Digest(wire), RequestHash: requestHash, CreatedAt: now.UnixMilli()}
		if err = tx.Save(&asset).Error; err != nil {
			return err
		}
		if err = tx.Create(&message).Error; err != nil {
			return err
		}
		last.PlayerID = id
		last.LastSendAt = now.UnixMilli()
		if err = tx.Save(&last).Error; err != nil {
			return err
		}
		fresh = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	if fresh {
		s.p10VoicePush(ctx, &asset, frame)
	}
	return []proto.Message{frame}, nil
}
func (s *GameService) p10VoicePush(ctx context.Context, asset *db.P10VoiceAsset, frame *pb.SCChatTcp) {
	recipients := []int64{}
	if asset.ChatType == 3 {
		recipients = append(recipients, asset.TargetID)
	} else {
		p10ClientSinks.Range(func(key, _ any) bool {
			who := key.(p10ClientKey)
			if who.service == s.p10ClientOwner() && who.playerID != asset.OwnerID {
				recipients = append(recipients, who.playerID)
			}
			return true
		})
	}
	for _, who := range recipients {
		var profile db.Profile
		if db.GetIns().WithContext(ctx).First(&profile, who).Error != nil {
			continue
		}
		allowed := false
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			_, err := p10LockProfiles(tx, who, asset.OwnerID)
			if err != nil {
				return err
			}
			if err = s.p10VoiceAccess(tx, who, profile.Session, asset, time.Now()); err != nil {
				return err
			}
			allowed = true
			return nil
		})
		if err == nil && allowed {
			s.publishP10Private(ctx, who, frame)
		}
	}
}
func (s *GameService) p10VoiceCheck(ctx context.Context, id int64, session []byte, kind int32) ([]proto.Message, error) {
	var out *pb.SCAudioChatFileName
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		_, player, err := p10VoiceAuth(tx, id, session)
		if err != nil {
			return err
		}
		if _, err = p10VoicePolicy(tx); err != nil {
			return err
		}
		if err = s.p10VoiceCanSend(player); err != nil {
			return err
		}
		scope, err := s.p10VoiceScope(tx, id, kind, true)
		if err != nil {
			return err
		}
		var asset db.P10VoiceAsset
		if err = tx.Where("owner_id=? AND owner_session_hash=? AND chat_type=? AND scope=? AND state=? AND expires_at>?", id, p6Digest(session), kind, scope, constUtil.P10VoiceUploaded, time.Now().UnixMilli()).Order("created_at DESC,id DESC").First(&asset).Error; err != nil {
			return err
		}
		if err = p10VoiceReadAsset(&asset, time.Now()); err != nil {
			return err
		}
		out = &pb.SCAudioChatFileName{ChatType: proto.Int32(kind), FileName: proto.String(asset.FileName), Time: proto.Int64(asset.CreatedAt), PlayerId: proto.Int64(id)}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return []proto.Message{out}, nil
}
func (s *GameService) p10VoiceHistory(ctx context.Context, id int64, session []byte, kind int32) ([]*pb.ChatInfo, error) {
	list := []*pb.ChatInfo{}
	outside := db.GetIns().WithContext(ctx)
	// 候选查询只确定锁集合；不向调用者返回任何未经当前授权的帧或字节。
	scopes := []string{"world/1"}
	if (kind == 0 || kind == 2) && outside.Migrator().HasTable(&db.P13GuildMember{}) {
		if guild, _, err := P13GuildScope(outside, id); err == nil && guild != "" {
			scopes = append(scopes, "guild/"+guild)
		}
	}
	var presence db.P10ScenePresence
	if (kind == 0 || kind == 5) && outside.First(&presence, "player_id=?", id).Error == nil && presence.Scene == "field_001" {
		scopes = append(scopes, fmt.Sprintf("field/%s/%d", presence.Scene, presence.Line))
	}
	if (kind == 0 || kind == 6) && outside.Migrator().HasTable(&db.P13UnionFightFieldPlayer{}) {
		var actor db.P13UnionFightFieldPlayer
		if outside.First(&actor, "player_id=? AND left_at=0", id).Error == nil {
			scopes = append(scopes, "war/"+actor.MatchID+"/"+actor.GuildID)
		}
	}
	query := outside.Model(&db.P10VoiceMessage{}).Where("(chat_type=3 AND (owner_id=? OR target_id=?)) OR (chat_type<>3 AND scope IN ?)", id, id, scopes)
	if kind != 0 {
		query = query.Where("chat_type=?", kind)
	}
	query = query.Where("NOT EXISTS (SELECT 1 FROM p10_social_blacks b WHERE (b.owner_id=? AND b.target_id=p10_voice_messages.owner_id) OR (b.target_id=? AND b.owner_id=p10_voice_messages.owner_id))", id, id)
	var rows []db.P10VoiceMessage
	if err := query.Order("id DESC").Limit(250).Find(&rows).Error; err != nil {
		return nil, err
	}
	err := outside.Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		// 所有相关角色按同一顺序先锁定，随后才形成InnoDB读取快照，防止退会/删除已提交后读旧会员。
		owners := []int64{id}
		for _, row := range rows {
			owners = append(owners, row.OwnerID)
		}
		sort.Slice(owners, func(i, j int) bool { return owners[i] < owners[j] })
		for i, owner := range owners {
			if i > 0 && owners[i-1] == owner {
				continue
			}
			var profile db.Profile
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, owner).Error; err != nil {
				return err
			}
		}
		if _, _, err := p10VoiceAuth(tx, id, session); err != nil {
			return err
		}
		var policy db.P10VoicePolicy
		if err := tx.First(&policy, 1).Error; err != nil {
			return err
		}
		if !policy.Enabled {
			return nil
		}
		if _, err := p10VoicePolicy(tx); err != nil {
			return err
		}
		for _, row := range rows {
			var asset db.P10VoiceAsset
			if err := tx.First(&asset, "id=?", row.AssetID).Error; err != nil {
				return err
			}
			if asset.OwnerID != row.OwnerID {
				return errors.New("实际语音历史所有权映射损坏")
			}
			if err := s.p10VoiceReadAccess(tx, id, session, &asset, time.Now(), false); err != nil {
				continue
			}
			frame, err := p10VoiceMessageFrame(&row, &asset)
			if err != nil {
				return err
			}
			list = append(list, &pb.ChatInfo{ChatType: frame.ChatType, Content: frame.Content, Time: frame.Time, Play: frame.Play, IsAudio: frame.IsAudio, AudioDownloadUrl: frame.DownLoadUrl, AudioTime: frame.AudioTime})
			if len(list) >= 50 {
				break
			}
		}
		return nil
	})
	return list, err
}
func (s *GameService) DispatchP10Voice(ctx context.Context, id int64, session []byte, request proto.Message, stamp int64) ([]proto.Message, bool, error) {
	switch request.(type) {
	case *pb.CSChatCache, *pb.CSUFChatCache, *pb.CSSceneChatCache:
		if err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}
			_, _, err := p10VoiceAuth(tx, id, session)
			return err
		}); err != nil {
			return nil, true, err
		}
	}
	switch body := request.(type) {
	case *pb.CSAudioChatCheck:
		out, err := s.p10VoiceCheck(ctx, id, session, body.GetChatType())
		return out, true, err
	case *pb.CSChatTcp:
		if body.GetIsAudio() == 0 {
			return nil, false, nil
		}
		out, err := s.p10VoicePublish(ctx, id, session, body)
		return out, true, err
	case *pb.CSChatCache:
		var base []proto.Message
		var handled bool
		var err error
		if body.GetChatType() == 6 {
			base, err = s.p13UFChatHistory(ctx, id, p13UFNow(ctx))
			handled = true
		} else {
			base, handled, err = s.DispatchP10Channel(ctx, id, request, stamp)
			if !handled {
				base, handled, err = s.DispatchP10Social(ctx, id, request, stamp)
			}
		}
		if !handled || err != nil {
			return base, handled, err
		}
		voices, err := s.p10VoiceHistory(ctx, id, session, body.GetChatType())
		if err != nil {
			return nil, true, err
		}
		for _, message := range base {
			switch cache := message.(type) {
			case *pb.SCChatCache:
				cache.ChatList = append(cache.ChatList, voices...)
				sort.SliceStable(cache.ChatList, func(i, j int) bool { return cache.ChatList[i].GetTime() < cache.ChatList[j].GetTime() })
				if len(cache.ChatList) > 50 {
					cache.ChatList = cache.ChatList[len(cache.ChatList)-50:]
				}
			case *pb.SCUFChatCache:
				cache.ChatList = append(cache.ChatList, voices...)
				sort.SliceStable(cache.ChatList, func(i, j int) bool { return cache.ChatList[i].GetTime() < cache.ChatList[j].GetTime() })
				if len(cache.ChatList) > 50 {
					cache.ChatList = cache.ChatList[len(cache.ChatList)-50:]
				}
			}
		}
		return base, true, nil
	case *pb.CSSceneChatCache:
		base, handled, err := s.DispatchP10Channel(ctx, id, request, stamp)
		if !handled || err != nil {
			return base, handled, err
		}
		voices, err := s.p10VoiceHistory(ctx, id, session, 5)
		if err != nil {
			return nil, true, err
		}
		cache := base[0].(*pb.SCSceneChatCache)
		cache.ChatList = append(cache.ChatList, voices...)
		sort.SliceStable(cache.ChatList, func(i, j int) bool { return cache.ChatList[i].GetTime() < cache.ChatList[j].GetTime() })
		if len(cache.ChatList) > 50 {
			cache.ChatList = cache.ChatList[len(cache.ChatList)-50:]
		}
		return base, true, nil
	case *pb.CSUFChatCache:
		base, err := s.p13UFChatHistory(ctx, id, p13UFNow(ctx))
		if err != nil {
			return nil, true, err
		}
		voices, err := s.p10VoiceHistory(ctx, id, session, 6)
		if err != nil {
			return nil, true, err
		}
		cache := base[0].(*pb.SCUFChatCache)
		cache.ChatList = append(cache.ChatList, voices...)
		sort.SliceStable(cache.ChatList, func(i, j int) bool { return cache.ChatList[i].GetTime() < cache.ChatList[j].GetTime() })
		if len(cache.ChatList) > 50 {
			cache.ChatList = cache.ChatList[len(cache.ChatList)-50:]
		}
		return base, true, nil
	}
	return nil, false, nil
}
