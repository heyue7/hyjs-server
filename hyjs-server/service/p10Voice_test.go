package service

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
	"sync"
	"testing"
	"time"
)

// 隔离测试生成明确的PCM方波音频；其真实字节可播放，不声称来自人工录音。
func p10VoiceTone(millis int) []byte {
	n := millis * 32
	raw := make([]byte, 44+n)
	copy(raw, "RIFF")
	binary.LittleEndian.PutUint32(raw[4:], uint32(len(raw)-8))
	copy(raw[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(raw[16:], 16)
	binary.LittleEndian.PutUint16(raw[20:], 1)
	binary.LittleEndian.PutUint16(raw[22:], 1)
	binary.LittleEndian.PutUint32(raw[24:], 16000)
	binary.LittleEndian.PutUint32(raw[28:], 32000)
	binary.LittleEndian.PutUint16(raw[32:], 2)
	binary.LittleEndian.PutUint16(raw[34:], 16)
	copy(raw[36:], "data")
	binary.LittleEndian.PutUint32(raw[40:], uint32(n))
	for i := 0; i < n/2; i++ {
		value := int16(4000)
		if i%40 >= 20 {
			value = -4000
		}
		binary.LittleEndian.PutUint16(raw[44+i*2:], uint16(value))
	}
	return raw
}
func p10VoiceSession(t *testing.T, id int64) []byte {
	t.Helper()
	var row db.Profile
	if err := db.GetIns().First(&row, id).Error; err != nil {
		t.Fatal(err)
	}
	return append([]byte(nil), row.Session...)
}
func p10VoiceSetup(t *testing.T) (*p1Fixture, int64, int64) {
	t.Helper()
	f, b, c := p10ChannelSetup(t)
	if err := f.s.InitializeP10Voice(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10VoiceMessage{}, &db.P10VoiceAsset{}, &db.P10VoicePolicy{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.s.InitializeP10Voice(f.ctx); err != nil {
		t.Fatal(err)
	}
	return f, b, c
}
func p10VoiceEnable(t *testing.T) {
	t.Helper()
	if err := db.GetIns().Model(&db.P10VoicePolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "retention_millis": 60000, "min_duration_millis": 100, "source": "isolated-real-pcm-v1"}).Error; err != nil {
		t.Fatal(err)
	}
}
func p10VoiceBody(upload *model.P10VoiceUploadResponse, kind int32, target int64) *pb.CSChatTcp {
	return &pb.CSChatTcp{ChatType: proto.Int32(kind), PlayerId: proto.Int64(target), IsAudio: proto.Int32(2), DownLoadUrl: proto.String(upload.FileName), AudioTime: proto.Int32(int32(upload.AudioTime)), FileSize: proto.Int64(upload.FileSize), Content: proto.String("")}
}
func TestP10VoiceRealPrivateBytesSessionBlacklistDeleteAndCold(t *testing.T) {
	f, b, c := p10VoiceSetup(t)
	session := p10VoiceSession(t, f.id)
	cfg, err := f.s.P10VoiceConfig(f.ctx, f.id, session, 3)
	if err != nil || cfg.Enabled || cfg.Adapter != 0 {
		t.Fatal("default local adapter", cfg, err)
	}
	if _, err = f.s.UploadP10Voice(f.ctx, f.id, session, 3, "off", "person", p10VoiceTone(2000)); err == nil {
		t.Fatal("closed upload")
	}
	keys, err := f.s.P10VoiceKVInfo(f.ctx, f.id, session)
	if err != nil || keys[0].GetKey() != "audiosystem" || keys[0].GetValue() != "off" || keys[1].GetValue() != "off" {
		t.Fatal("original closed login audio feature", keys, err)
	}
	p10VoiceEnable(t)
	keys, err = f.s.P10VoiceKVInfo(f.ctx, f.id, session)
	if err != nil || keys[0].GetValue() != "on" || keys[1].GetValue() != "off" {
		t.Fatal("actual local original login audio feature", keys, err)
	}
	cfg, err = f.s.P10VoiceConfig(f.ctx, f.id, session, 3)
	if err != nil || !cfg.Enabled || !cfg.CanRecord || cfg.MaxSeconds != 30 || cfg.RecordingGapSeconds != 1 || cfg.Scope != "person" {
		t.Fatal(cfg, err)
	}
	if _, err = f.s.p10VoiceCheck(f.ctx, f.id, session, 3); err == nil {
		t.Fatal("unmaterialized filename success")
	}
	raw := p10VoiceTone(2100)
	upload, err := f.s.UploadP10Voice(f.ctx, f.id, session, 3, "actual-private", "person", raw)
	if err != nil || upload.AudioTime != 2 || upload.DurationMillis != 2100 || upload.SHA256 != p6Digest(raw) {
		t.Fatal(upload, err)
	}
	checked, err := f.s.p10VoiceCheck(f.ctx, f.id, session, 3)
	if err != nil || checked[0].(*pb.SCAudioChatFileName).GetFileName() != upload.FileName {
		t.Fatal(checked, err)
	}
	if _, err = f.s.DownloadP10Voice(f.ctx, b, p10VoiceSession(t, b), upload.FileName); err == nil {
		t.Fatal("unpublished recipient fetched")
	}
	body := p10VoiceBody(upload, 3, b)
	response, err := f.s.p10VoicePublish(f.ctx, f.id, session, body)
	if err != nil || response[0].(*pb.SCChatTcp).GetIsAudio() != 2 || response[0].(*pb.SCChatTcp).GetContent() != "" {
		t.Fatal(response, err)
	}
	fetched, err := f.s.DownloadP10Voice(f.ctx, b, p10VoiceSession(t, b), upload.FileName)
	if err != nil || !bytes.Equal(raw, fetched) {
		t.Fatal("real recipient bytes", err)
	}
	if _, err = f.s.DownloadP10Voice(f.ctx, c, p10VoiceSession(t, c), upload.FileName); err == nil {
		t.Fatal("foreign private read")
	}
	if _, err = f.s.DeleteP10Voice(f.ctx, b, p10VoiceSession(t, b), upload.FileName); err == nil {
		t.Fatal("foreign delete")
	}
	body.Content = proto.String("modified pointer")
	history, err := f.s.p10VoiceHistory(f.ctx, b, p10VoiceSession(t, b), 3)
	if err != nil || len(history) != 1 || history[0].GetContent() != "" {
		t.Fatal("frame snapshot", history, err)
	}
	f.restart(t)
	if err = f.s.InitializeP10Voice(f.ctx); err != nil {
		t.Fatal(err)
	}
	fetched, err = f.s.DownloadP10Voice(f.ctx, b, p10VoiceSession(t, b), upload.FileName)
	if err != nil || !bytes.Equal(raw, fetched) {
		t.Fatal("cold actual bytes", err)
	}
	if err = db.GetIns().Create(&db.P10SocialBlack{OwnerID: b, TargetID: f.id, CreatedAt: time.Now().UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DownloadP10Voice(f.ctx, b, p10VoiceSession(t, b), upload.FileName); err == nil {
		t.Fatal("cached opaque ID bypassed new blacklist")
	}
	history, err = f.s.p10VoiceHistory(f.ctx, b, p10VoiceSession(t, b), 3)
	if err != nil || len(history) != 0 {
		t.Fatal("black history", err)
	}
	if err = db.GetIns().Where("owner_id=? AND target_id=?", b, f.id).Delete(&db.P10SocialBlack{}).Error; err != nil {
		t.Fatal(err)
	}
	var account db.Profile
	if err = db.GetIns().First(&account, b).Error; err != nil {
		t.Fatal(err)
	}
	old := append([]byte(nil), account.Session...)
	if _, err = LoginAccount(f.ctx, account.Uid, "local-test-password"); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DownloadP10Voice(f.ctx, b, old, upload.FileName); err == nil {
		t.Fatal("old logged-out session downloaded")
	}
	if _, err = f.s.DownloadP10Voice(f.ctx, b, p10VoiceSession(t, b), upload.FileName); err != nil {
		t.Fatal("current recipient session", err)
	}
	if _, err = f.s.DeleteP10Voice(f.ctx, f.id, session, upload.FileName); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DeleteP10Voice(f.ctx, f.id, session, upload.FileName); err != nil {
		t.Fatal("own delete idempotence", err)
	}
	var asset db.P10VoiceAsset
	if err = db.GetIns().First(&asset, "file_name=?", upload.FileName).Error; err != nil {
		t.Fatal(err)
	}
	if len(asset.Bytes) != 0 || asset.State != constUtil.P10VoiceDeleted || asset.MetaHash != p10VoiceAssetHash(&asset) {
		t.Fatal("delete retained bytes")
	}
	if _, err = f.s.DownloadP10Voice(f.ctx, b, p10VoiceSession(t, b), upload.FileName); err == nil {
		t.Fatal("deleted cached filename fetched")
	}
	if _, err = f.s.DeleteP10Voice(f.ctx, f.id, session, "hyjsvoice_missing.wav"); err == nil {
		t.Fatal("missing deletion success")
	}
}
func TestP10VoiceRealUploadNoncePublishConcurrentAndIntegrity(t *testing.T) {
	f, b, _ := p10VoiceSetup(t)
	p10VoiceEnable(t)
	session := p10VoiceSession(t, f.id)
	raw := p10VoiceTone(2000)
	var group sync.WaitGroup
	uploads := make(chan *model.P10VoiceUploadResponse, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			u, err := f.s.UploadP10Voice(f.ctx, f.id, session, 3, "same-upload", "person", raw)
			uploads <- u
			errs <- err
		}()
	}
	group.Wait()
	close(uploads)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var upload *model.P10VoiceUploadResponse
	for u := range uploads {
		if upload != nil && upload.FileName != u.FileName {
			t.Fatal("duplicate nonce materialized second file")
		}
		upload = u
	}
	if p1Count(t, &db.P10VoiceAsset{}, "owner_id=?", f.id) != 1 {
		t.Fatal("duplicate blob")
	}
	if _, err := f.s.UploadP10Voice(f.ctx, f.id, session, 3, "same-upload", "person", p10VoiceTone(2300)); err == nil {
		t.Fatal("changed bytes nonce accepted")
	}
	rec := &p10SceneRecorder{}
	p10SceneConnect(t, f, b, rec)
	rec.drain()
	body := p10VoiceBody(upload, 3, b)
	errs = make(chan error, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() { defer group.Done(); _, err := f.s.p10VoicePublish(f.ctx, f.id, session, body); errs <- err }()
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if p1Count(t, &db.P10VoiceMessage{}, "owner_id=?", f.id) != 1 || len(p10ChannelChats(rec.drain())) != 1 {
		t.Fatal("duplicate voice receipt/push")
	}
	changed := proto.Clone(body).(*pb.CSChatTcp)
	changed.PlayerId = proto.Int64(f.id + 1000000)
	if _, err := f.s.p10VoicePublish(f.ctx, f.id, session, changed); err == nil {
		t.Fatal("changed target replay accepted")
	}
	var asset db.P10VoiceAsset
	if err := db.GetIns().First(&asset, "file_name=?", upload.FileName).Error; err != nil {
		t.Fatal(err)
	}
	asset.Bytes[44] ^= 1
	if err := db.GetIns().Model(&asset).Update("bytes", asset.Bytes).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.DownloadP10Voice(f.ctx, b, p10VoiceSession(t, b), upload.FileName); err == nil {
		t.Fatal("corrupt PCM served")
	}
}
func TestP10VoiceOriginalLimitsCurrentScopeSessionAndTTL(t *testing.T) {
	f, b, _ := p10VoiceSetup(t)
	p10VoiceEnable(t)
	p10ChannelEnable(t)
	session := p10VoiceSession(t, f.id)
	for i, raw := range [][]byte{nil, []byte("fake voice"), p10VoiceTone(30001), p10VoiceTone(50)} {
		if _, err := f.s.UploadP10Voice(f.ctx, f.id, session, 3, fmt.Sprint(i), "person", raw); err == nil {
			t.Fatal("invalid/too-short/over-original-limit stored", i)
		}
	}
	if _, err := f.s.UploadP10Voice(f.ctx, f.id, session, 1, "offline", "world/1", p10VoiceTone(1000)); err == nil {
		t.Fatal("offline world upload")
	}
	ra, rb := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, f.id, ra)
	p10SceneConnect(t, f, b, rb)
	cfg, err := f.s.P10VoiceConfig(f.ctx, f.id, session, 5)
	if err != nil {
		t.Fatal(err)
	}
	u, err := f.s.UploadP10Voice(f.ctx, f.id, session, 5, "field", cfg.Scope, p10VoiceTone(2000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p10VoicePublish(f.ctx, f.id, session, p10VoiceBody(u, 5, 0)); err != nil {
		t.Fatal(err)
	}
	fetched, err := f.s.DownloadP10Voice(f.ctx, b, p10VoiceSession(t, b), u.FileName)
	if err != nil || len(fetched) != 64044 {
		t.Fatal("same original line", err)
	}
	scene, handled, err := f.s.DispatchP10Voice(f.ctx, b, p10VoiceSession(t, b), &pb.CSSceneChatCache{}, 0)
	if !handled || err != nil || len(scene[0].(*pb.SCSceneChatCache).GetChatList()) != 1 {
		t.Fatal("original field cache audio", scene, err)
	}
	if _, _, err = f.s.DispatchP10Scene(f.ctx, b, &pb.CSTransferLine{Line: proto.Int32(2)}, 99); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DownloadP10Voice(f.ctx, b, p10VoiceSession(t, b), u.FileName); err == nil {
		t.Fatal("cache bypass after original line change")
	}
	if _, _, err = f.s.DispatchP10Scene(f.ctx, f.id, &pb.CSTransferLine{Line: proto.Int32(2)}, 100); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.UploadP10Voice(f.ctx, f.id, session, 5, "changed-field", cfg.Scope, p10VoiceTone(2000)); err == nil {
		t.Fatal("record-start scope reused")
	}
	for i := 0; i < 3; i++ {
		if _, err = f.s.UploadP10Voice(f.ctx, f.id, session, 3, fmt.Sprintf("pending-%d", i), "person", p10VoiceTone(1000)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = f.s.UploadP10Voice(f.ctx, f.id, session, 3, "fourth", "person", p10VoiceTone(1000)); err == nil {
		t.Fatal("pending cap bypass")
	}
	if err = f.s.TickP10Voice(f.ctx, time.Now().Add(time.Minute*2)); err != nil {
		t.Fatal(err)
	}
	if p1Count(t, &db.P10VoiceAsset{}, "state<>?", constUtil.P10VoiceDeleted) != 0 {
		t.Fatal("expired audio blobs not purged")
	}
}

func TestP10VoiceGuildWithdrawalCurrentSourceAndFrozenBytes(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := time.Now()
	p13GuildOpen(t, false)
	if err := f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP10Channel(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.s.InitializeP10Voice(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10VoiceMessage{}, &db.P10VoiceAsset{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	p10VoiceEnable(t)
	if err := db.GetIns().Model(&db.P13GuildPolicy{}).Where("id=1").Update("chat_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		session := p10VoiceSession(t, id)
		t.Cleanup(f.s.RegisterP10Client(f.ctx, id, session, func(proto.Message) error { return nil }))
	}
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("音频来源会")}, 1, now)
	guild := p13GuildID(t, f, ids[0], now)
	p13Join(t, f, ids[0], ids[1], guild, 2, now)
	p13Join(t, f, ids[0], ids[2], guild, 3, now)
	session := p10VoiceSession(t, ids[1])
	cfg, err := f.s.P10VoiceConfig(f.ctx, ids[1], session, 2)
	if err != nil {
		t.Fatal(err)
	}
	upload, err := f.s.UploadP10Voice(f.ctx, ids[1], session, 2, "guildsource", cfg.Scope, p10VoiceTone(2000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p10VoicePublish(f.ctx, ids[1], session, p10VoiceBody(upload, 2, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DownloadP10Voice(f.ctx, ids[2], p10VoiceSession(t, ids[2]), upload.FileName); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DownloadP10Voice(f.ctx, ids[4], p10VoiceSession(t, ids[4]), upload.FileName); err == nil {
		t.Fatal("nonmember downloaded guild voice")
	}
	p13GuildCall(t, f, ids[1], &pb.CSQuitUnion{}, 4, now)
	if _, err = f.s.DownloadP10Voice(f.ctx, ids[2], p10VoiceSession(t, ids[2]), upload.FileName); err == nil {
		t.Fatal("cached filename bypassed sender source withdrawal")
	}
	if _, err = f.s.DeleteP10Voice(f.ctx, ids[1], session, upload.FileName); err != nil {
		t.Fatal("withdrawn owner cleanup", err)
	}
}
func TestP10VoiceActualWarRosterAndEndedSource(t *testing.T) {
	f, ids, _, clock, match := p13UFFieldSetup(t)
	if err := f.s.InitializeP10Voice(f.ctx); err != nil {
		t.Fatal(err)
	}
	for _, table := range []any{&db.P10VoiceMessage{}, &db.P10VoiceAsset{}} {
		if err := db.GetIns().Where("1=1").Delete(table).Error; err != nil {
			t.Fatal(err)
		}
	}
	p10VoiceEnable(t)
	if err := db.GetIns().Model(&db.P13UnionFightFieldPolicy{}).Where("id=1").Updates(map[string]any{"enabled": true, "preparation_millis": 300000, "left_revive_point_id": 1, "right_revive_point_id": 3, "initial_guard_hp": 1000, "initial_gate_hp": 10000, "move_tolerance": 3, "source": "isolated-original-voice-roster-no-authoritative-hp"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P13UnionFightChatPolicy{}).Where("id=1").Update("text_enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	now := time.UnixMilli(clock.Load())
	var enemy db.P13GuildMember
	other := match.RightGuildID
	if other == p13GuildID(t, f, ids[0], now) {
		other = match.LeftGuildID
	}
	if err := db.GetIns().First(&enemy, "guild_id=?", other).Error; err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{ids[0], ids[10], enemy.PlayerID} {
		if _, err := f.s.p13UFFieldEnter(f.ctx, id, now); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(f.s.RegisterP10Client(f.ctx, id, p10VoiceSession(t, id), func(proto.Message) error { return nil }))
		if _, err := f.s.SyncP13UnionFightField(f.ctx, id, false); err != nil {
			t.Fatal(err)
		}
		if _, err := f.s.p13UFFieldFrames(f.ctx, id, &pb.CSCurrentPosition{}, now); err != nil {
			t.Fatal(err)
		}
	}
	session := p10VoiceSession(t, ids[0])
	cfg, err := f.s.P10VoiceConfig(f.ctx, ids[0], session, 6)
	if err != nil {
		t.Fatal(err)
	}
	u, err := f.s.UploadP10Voice(f.ctx, ids[0], session, 6, "actual-war", cfg.Scope, p10VoiceTone(2000))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.p10VoicePublish(f.ctx, ids[0], session, p10VoiceBody(u, 6, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DownloadP10Voice(f.ctx, ids[10], p10VoiceSession(t, ids[10]), u.FileName); err != nil {
		t.Fatal(err)
	}
	if _, err = f.s.DownloadP10Voice(f.ctx, enemy.PlayerID, p10VoiceSession(t, enemy.PlayerID), u.FileName); err == nil {
		t.Fatal("actual opposing roster voice leak")
	}
	clock.Store(match.EndAt)
	if _, err = f.s.DownloadP10Voice(f.ctx, ids[10], p10VoiceSession(t, ids[10]), u.FileName); err == nil {
		t.Fatal("ended war source still authorized cached file")
	}
}
