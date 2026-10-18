package service

import (
	"context"
	"path/filepath"
	"testing"

	"google.golang.org/protobuf/proto"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// TestEquipmentForgeFailureAndCumulativeProperties 验证原表累计属性和失败扣费保级。
func TestEquipmentForgeFailureAndCumulativeProperties(t *testing.T) {
	ctx := context.Background()
	if err := db.InitGame(ctx, filepath.Join(t.TempDir(), "equipment.db")); err != nil {
		t.Fatal(err)
	}
	connection, _ := db.GetIns().DB()
	defer connection.Close()
	s := &GameService{}
	if err := s.LoadTables("../../analysis/tables"); err != nil {
		t.Fatal(err)
	}
	if err := s.InitEquipmentMutations(ctx); err != nil {
		t.Fatal(err)
	}
	profile, err := LoginAccount(ctx, "equipment-forge-test", "isolated-test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = CreatePlayer(ctx, profile.Id, "isolated", "hero_043"); err != nil {
		t.Fatal(err)
	}
	if err = updateProfile(ctx, profile.Id, func(p *pb.Player, g *model.GameState) error {
		p.Level = proto.Int32(60)
		p.Coin = proto.Int64(10000)
		g.Equipment = []*pb.Equipment{{Guid: proto.String("test-equip"), EquipId: proto.String("equip_037"), Level: proto.Int32(1)}}
		g.Items = []*pb.SimpleItem{{Id: proto.String("frag_equip_037"), Num: proto.Int32(10)}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// 隔离测试将首档置零，确定性覆盖失败响应；正式 APK 表和运营配置保持不变。
	for _, row := range s.Tables["EquipForgeOdds"] {
		if row["quality"] == "23" && row["forgeLevel"] == "1" {
			row["odds"] = "0"
		}
	}
	request := &pb.CSEquipForge{Guid: proto.String("test-equip"), Cost: []*pb.BaseResource{{Id: proto.String("frag_equip_037"), Num: proto.Int32(3)}, {Id: proto.String("coin"), Num: proto.Int32(1000)}}}
	responses, err := s.equipmentForge(ctx, profile.Id, request, 100)
	if err != nil {
		t.Fatal(err)
	}
	response := responses[0].(*pb.SCEquipForge)
	if response.GetResult() != 0 || response.GetEquip().GetForgeLevel() != 0 {
		t.Fatalf("failure changed level: %v", response)
	}
	retry, err := s.equipmentForge(ctx, profile.Id, request, 100)
	if err != nil || !proto.Equal(response, retry[0]) {
		t.Fatalf("failure replay err=%v", err)
	}
	if err = db.GetIns().First(profile, profile.Id).Error; err != nil {
		t.Fatal(err)
	}
	player, game, err := decodeProfile(profile)
	if err != nil || player.GetCoin() != 9000 || game.Items[0].GetNum() != 7 {
		t.Fatalf("failure cost incorrect player=%v game=%v err=%v", player, game, err)
	}
	values := map[string]float64{}
	if err = s.equipmentForgeProperties(&pb.Equipment{EquipId: proto.String("equip_037"), ForgeLevel: proto.Int32(2)}, values); err != nil {
		t.Fatal(err)
	}
	if values["att"] != 65 {
		t.Fatalf("expected cumulative 25+40 attack, got %v", values)
	}
	if err = s.equipmentForgeProperties(&pb.Equipment{EquipId: proto.String("equip_001"), ForgeLevel: proto.Int32(1)}, map[string]float64{}); err == nil {
		t.Fatal("unsupported forged state accepted")
	}
}
