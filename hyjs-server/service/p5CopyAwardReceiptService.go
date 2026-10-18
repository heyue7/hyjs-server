package service

import (
	"errors"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// ValidateCopyAwardReceiptCurrent checks the stored incremental projection in
// the caller's locked profile transaction. It never changes the saved award.
func (s *GameService) ValidateCopyAwardReceiptCurrent(tx *gorm.DB, id int64, g *model.GameState, response proto.Message) error {
	if err := s.validateP5GrowthReceipt(g, response); err != nil {
		return err
	}
	checkPet := func(guid string, snapshot *pb.Pet, exp *int32) error {
		if tx == nil || guid == "" {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
		current, err := p9OwnedPet(tx, id, guid)
		if err != nil {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
		if snapshot != nil {
			if snapshot.GetGuid() != guid || !proto.Equal(current, snapshot) {
				return errors.New(constUtil.P5GrowthReceiptChanged)
			}
		} else if exp == nil || current.GetEat().GetExp() != *exp {
			return errors.New(constUtil.P5GrowthReceiptChanged)
		}
		return nil
	}
	if result, ok := response.(interface{ GetGain() *pb.ResultObject }); ok {
		for _, pet := range result.GetGain().GetPets() {
			if err := checkPet(pet.GetGuid(), pet, nil); err != nil {
				return err
			}
		}
	}
	if result, ok := response.(interface{ GetPetAdds() []*pb.PetAddIntimacy }); ok {
		for _, entry := range result.GetPetAdds() {
			if err := checkPet(entry.GetGuid(), entry.GetPet(), proto.Int32(entry.GetCurExp())); err != nil {
				return err
			}
		}
	}
	return nil
}
