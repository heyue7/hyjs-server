package service

import (
	"errors"
	"hyjs-server/constUtil"
)

// P5FormationHeroAllowed includes owned green12 and blue13 combat cards.
// Experience materials remain excluded independently by heroClass99.
func (s *GameService) P5FormationHeroAllowed(heroID string) error {
	row := s.table("Hero", "id", heroID)
	if row == nil || row["heroClass"] == "99" || tableNumber(row, "quality") <= constUtil.P5FormationExcludedQuality {
		return errors.New(constUtil.P5FormationIneligibleHero)
	}
	return nil
}
