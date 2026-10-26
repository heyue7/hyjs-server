package service

import (
	"context"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestTrustedDeploymentServerIdentity 请求区服不能改变部署身份或借用其它服会话。
func TestTrustedDeploymentServerIdentity(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "trusted-server.db")); err != nil {
		t.Fatal(err)
	}
	pool, _ := db.GetIns().DB()
	defer pool.Close()
	if err := InitializeLoginSecurity(ctx); err != nil {
		t.Fatal(err)
	}
	s := &GameService{Host: "127.0.0.1", ServerID: 2}
	wrong := &model.GamePacket{ZoneId: 1, Body: &pb.CSUserLogin{PlatformUid: proto.String("trusted-id-fixture"), AuthCode: proto.String("isolated-fixture-password")}}
	if _, err := s.dispatch(ctx, wrong, "127.0.0.1"); err == nil {
		t.Fatal("foreign deployment login accepted")
	}
	var count int64
	if err := db.GetIns().Model(&db.Profile{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatal("foreign login wrote local account", err, count)
	}
	wrong.ZoneId = 2
	result, err := s.dispatch(ctx, wrong, "127.0.0.1")
	if err != nil || len(result) != 1 {
		t.Fatal("configured deployment login failed", err)
	}
	login := result[0].(*pb.SCUserLogin)
	packet := &model.GamePacket{PlayerId: login.GetPlayerId(), Session: login.GetSessionKey(), ZoneId: 1, Body: &pb.CSSocketLogin{SessionKey: login.GetSessionKey()}}
	if _, err := s.dispatch(ctx, packet, "127.0.0.1"); err == nil {
		t.Fatal("valid local session with foreign zone accepted")
	}
	packet.ZoneId = 2
	if _, err := s.dispatch(ctx, packet, "127.0.0.1"); err != nil {
		t.Fatal(err)
	}
	result, err = s.dispatch(ctx, &model.GamePacket{Body: &pb.CSGetServerList{PlatformUid: proto.String("trusted-id-fixture")}}, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	list := result[0].(*pb.SCGetServerList)
	if len(list.GetServerList()) != 1 || list.GetServerList()[0].GetServerId() != 2 || list.GetServerList()[0].GetGlobalServerKey() != "local-2" {
		t.Fatal("server directory lost trusted deployment identity")
	}
	var row db.Profile
	if err := db.GetIns().First(&row, login.GetPlayerId()).Error; err != nil && err != gorm.ErrRecordNotFound {
		t.Fatal(err)
	}
	if s.OriginalServerID() != 2 || (&GameService{}).OriginalServerID() != 1 {
		t.Fatal("deployment identity changed through request fields")
	}
}
