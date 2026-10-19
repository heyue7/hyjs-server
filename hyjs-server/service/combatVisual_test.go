package service

import (
	"testing"

	"google.golang.org/protobuf/proto"
	pb "hyjs-server/model/protocol"
)

func TestBattleWeaponUsesActualOwnedMainhandAndColdWire(t *testing.T) {
	s, player, game := worldSeedFixture(t)
	party, err := s.battlePlayer(player, game)
	if err != nil {
		t.Fatal(err)
	}
	if party.GetFormation()[0].GetWeaponId() != "equip_001" || party.GetFormation()[1].GetWeaponId() != "" {
		t.Fatalf("actual owned mainhand missing or leaked to another actor: %v", party)
	}
	// 用实际战斗包序列化与冷解码确认原 weaponId 字段传输，保存不依赖 UI 缓存。
	wire, err := proto.Marshal(party)
	if err != nil {
		t.Fatal(err)
	}
	cold := &pb.BattlePlayer{}
	if err = proto.Unmarshal(wire, cold); err != nil || !proto.Equal(cold, party) {
		t.Fatalf("battle weapon cold wire differs: %v", err)
	}
	if cold.GetFormation()[0].GetWeaponId() != "equip_001" || cold.GetFormation()[0].Prop.GetAtt() != 171 {
		t.Fatal("weapon transport changed original actual equipment properties")
	}
	game.Formation[1].Things = append(game.Formation[1].Things, game.Formation[0].Things[1])
	game.Formation[0].Things = game.Formation[0].Things[:1]
	after, err := s.battlePlayer(player, game)
	if err != nil || after.GetFormation()[0].GetWeaponId() != "" || after.GetFormation()[1].GetWeaponId() != "equip_001" {
		t.Fatalf("changed ownership slot kept stale weapon: %v", err)
	}
}

func TestBattleWeaponRejectsUnownedWrongTypeAndDuplicateMainhand(t *testing.T) {
	for _, kind := range []string{"unowned", "wrong-id", "wrong-type", "duplicate"} {
		t.Run(kind, func(t *testing.T) {
			s, _, game := worldSeedFixture(t)
			slot := game.Formation[0]
			switch kind {
			case "unowned":
				game.Equipment = nil
			case "wrong-id":
				slot.Things[1].Id = proto.String("equip_002")
			case "wrong-type":
				s.table("Equip", "id", "equip_001")["type"] = "2"
			case "duplicate":
				slot.Things = append(slot.Things, proto.Clone(slot.Things[1]).(*pb.FormationThing))
			}
			if _, err := s.battleWeaponID(game, slot); err == nil {
				t.Fatal("invalid equipped weapon admitted")
			}
		})
	}
}
