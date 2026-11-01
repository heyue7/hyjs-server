package service

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

func TestP10SceneGuildAndTitleSelfPeersAndRefresh(t *testing.T) {
	f, ids := p13GuildSetup(t)
	now := time.Now()
	if err := f.s.LoadP5Tables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	p13GuildOpen(t, false)
	if err := f.s.InitializeP10Scene(f.ctx); err != nil {
		t.Fatal(err)
	}
	if err := db.GetIns().Model(&db.P10ScenePolicy{}).Where("id=1").Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	p13GuildCall(t, f, ids[0], &pb.CSUnionCreate{Name: proto.String("同屏公会")}, 18001, now)
	guild := p13GuildID(t, f, ids[0], now)
	p13Join(t, f, ids[0], ids[1], guild, 18002, now)
	if err := updateProfile(f.ctx, ids[0], func(_ *pb.Player, game *model.GameState) error {
		owned, err := json.Marshal(p5TitleState{Source: "isolated title fixture"})
		if err != nil {
			return err
		}
		p5SetKV(game, "p5.title:title_001", string(owned))
		p5SetKV(game, "p5.current-title", "title_001")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	login, err := f.s.fieldScene(f.ctx, ids[0], true)
	if err != nil || len(login) == 0 {
		t.Fatalf("self login: %v", err)
	}
	self := login[0].(*pb.SCLoginScene).GetSelf().GetInfo()
	if self.GetUnionName() != "同屏公会" || self.GetTitle() != "title_001" {
		t.Fatalf("self head identity: %v", self)
	}
	ra, rb := &p10SceneRecorder{}, &p10SceneRecorder{}
	p10SceneConnect(t, f, ids[0], ra)
	p10SceneConnect(t, f, ids[1], rb)
	var peer *pb.MovePlayerInfo
	for _, message := range rb.drain() {
		if appear, ok := message.(*pb.SCPlayerAppear); ok {
			for _, player := range appear.GetPlayer() {
				if player.GetInfo().GetPlayerId() == ids[0] {
					peer = player.GetInfo()
				}
			}
		}
	}
	if peer == nil || peer.GetUnionName() != "同屏公会" || peer.GetTitle() != "title_001" {
		t.Fatalf("peer head identity: %v", peer)
	}
	ra.drain()
	quit := &pb.CSQuitUnion{}
	p13GuildCall(t, f, ids[1], quit, 18003, now)
	ctx := context.WithValue(f.ctx, constUtil.GameRequestContextKey, quit)
	f.s.AfterP10SceneRequest(ctx, ids[1], quit)
	var refreshed *pb.MovePlayerInfo
	for _, message := range ra.drain() {
		if update, ok := message.(*pb.SCMovePlayerInfo); ok {
			for _, player := range update.GetPlayerInfos() {
				if player.GetInfo().GetPlayerId() == ids[1] {
					refreshed = player.GetInfo()
				}
			}
		}
	}
	if refreshed == nil || refreshed.UnionName == nil || refreshed.GetUnionName() != "" {
		t.Fatalf("quit did not clear peer guild name: %v", refreshed)
	}
	if err := updateProfile(f.ctx, ids[0], func(_ *pb.Player, game *model.GameState) error {
		p5SetKV(game, "p5.current-title", "")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.s.SyncP10Scene(f.ctx, ids[0], false); err != nil {
		t.Fatal(err)
	}
	var titleUpdate *pb.MovePlayerInfo
	for _, message := range rb.drain() {
		if update, ok := message.(*pb.SCMovePlayerInfo); ok && len(update.GetPlayerInfos()) > 0 {
			titleUpdate = update.GetPlayerInfos()[0].GetInfo()
		}
	}
	if titleUpdate == nil || titleUpdate.Title == nil || titleUpdate.GetTitle() != "" || titleUpdate.GetUnionName() != "同屏公会" {
		t.Fatalf("title removal lost guild or failed to propagate: %v", titleUpdate)
	}
}
