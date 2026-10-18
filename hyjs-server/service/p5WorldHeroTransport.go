package service

import (
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// P5WorldHeroID follows UIPlayerData.mainHeroId -> GetBattleCamp(1).hero.heroId.
// HeroWarId continues to identify the original growth instance. Legacy saves
// without a populated main camp retain their owned original avatar fallback.
func P5WorldHeroID(p *pb.Player, g *model.GameState) (string, error) {
	for _, slot := range g.Formation {
		if slot.GetFormatPos() != 1 {
			continue
		}
		if hero := formationHero(slot); hero != nil {
			owned, err := p5Warrior(g, hero.GetGuid())
			if err != nil || owned.GetId() != hero.GetId() {
				return "", p5Error()
			}
			return owned.GetId(), nil
		}
	}
	owned, err := p5Warrior(g, p.GetHeroWarId())
	if err != nil {
		return "", err
	}
	return owned.GetId(), nil
}
