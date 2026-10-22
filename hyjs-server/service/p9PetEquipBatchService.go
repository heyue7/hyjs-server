package service

import (
	"errors"
	"hyjs-server/constUtil"
	pb "hyjs-server/model/protocol"
)

// p9EquipFiveSteps 按总最高级、当前精灵和进阶上限截取剩余强化阶数。
func (s *GameService) p9EquipFiveSteps(pet *pb.Pet, equip *pb.PetEquip) (int32, error) {
	var maximum int32
	for _, row := range s.Tables["PetEquipStrength"] {
		if row["equipId"] == equip.GetEquipId() && int32(tableNumber(row, "level")) > maximum {
			maximum = int32(tableNumber(row, "level"))
		}
	}
	if maximum < 1 {
		return 0, errors.New(constUtil.P9PetEquipFiveNativeLimit)
	}
	cap, err := s.p9EquipStrengthCap(pet, equip)
	if err != nil {
		return 0, err
	}
	steps := cap - equip.GetStrengthLevel()
	if steps <= 0 {
		return 0, errors.New(constUtil.P9PetEquipFiveLimit)
	}
	if steps > 5 {
		steps = 5
	}
	return steps, nil
}
