package service

import (
	"errors"
	"hyjs-server/constUtil"
)

// P5FormationHeroAllowed mirrors GetAvailableHerosForFormation 0114-012c:
// only original quality>12 cards enter the main formation selection.
// Owned green cards remain legitimate inventory, collection and material data.
func (s *GameService) P5FormationHeroAllowed(heroID string) error {
	row := s.table("Hero", "id", heroID)
	if row == nil || row["heroClass"] == "99" || tableNumber(row, "quality") <= constUtil.P5FormationExcludedQuality {
		return errors.New(constUtil.P5FormationIneligibleHero)
	}
	return nil
}
