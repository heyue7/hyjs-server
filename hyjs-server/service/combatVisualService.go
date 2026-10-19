package service

import (
	"errors"

	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// battleWeaponID 使用当前上阵槽实际拥有的主手装备，供原客户端加载武器模型。
// 角色属性仍由 equipmentFormationProperties 计算，外观字段不参与属性乘算。
func (s *GameService) battleWeaponID(game *model.GameState, slot *pb.FormationItem) (string, error) {
	weapon := ""
	for _, thing := range slot.GetThings() {
		if thing.GetPos() != 1 {
			continue
		}
		equip, err := s.equipmentRecord(game, thing.GetGuid())
		if err != nil {
			return "", err
		}
		if weapon != "" || thing.GetId() != equip.GetEquipId() || tableNumber(s.table("Equip", "id", equip.GetEquipId()), "type") != 1 {
			return "", errors.New("战斗武器与实际主手装备不匹配")
		}
		weapon = equip.GetEquipId()
	}
	return weapon, nil
}
