package service

import (
	"errors"
	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// FreezeP6KFReplayInput preserves the original PID-only wrapper loop. Every
// matching player adds a fighter pair: the first matching pair is GetFighter0,
// while any index1 match sets view=true. An unmatched viewer uses source0/1.
// All later source groups remain in KFReplay and are not certified for settlement.
func (s *GameService) FreezeP6KFReplayInput(original *pb.KFReplayData, viewingPlayerID int64) (*model.FrozenBattleInput, error) {
	return s.freezeP6KFReplayViewer(original, &model.FrozenP6KFViewer{PlayerID: viewingPlayerID})
}

// FreezeP6KFReplayRealmInput explicitly models the local same-PID compatibility
// wrapper. It retains both original IDs and the exact full typed source bytes.
func (s *GameService) FreezeP6KFReplayRealmInput(original *pb.KFReplayData, viewingServerID, viewingPlayerID int64) (*model.FrozenBattleInput, error) {
	return s.freezeP6KFReplayViewer(original, &model.FrozenP6KFViewer{PlayerID: viewingPlayerID, ServerID: viewingServerID, CompatibilityRule: constUtil.P6KFJointViewerCompatibility})
}

func p6KFViewerProjection(raw *pb.KFReplayData, viewer *model.FrozenP6KFViewer) (int, bool, error) {
	reject := func() (int, bool, error) { return 0, false, errors.New(constUtil.P6ReplayConflict) }
	if raw == nil || viewer == nil || len(raw.PlayerList) < 2 {
		return reject()
	}
	for _, player := range raw.PlayerList {
		if player == nil {
			return reject()
		}
	}
	if raw.PlayerList[0].Inst == nil || raw.PlayerList[1].Inst == nil {
		return reject()
	}
	if viewer.CompatibilityRule == "" {
		if viewer.ServerID != 0 {
			return reject()
		}
		index := -1
		view := false
		for i, player := range raw.PlayerList {
			if player.GetPlayerId() != viewer.PlayerID {
				continue
			}
			if index < 0 {
				index = i
			}
			if i == 1 {
				view = true
			}
		}
		if index < 0 {
			index = 0
		}
		return index, view, nil
	}
	if viewer.CompatibilityRule != constUtil.P6KFJointViewerCompatibility || viewer.ServerID <= 0 || viewer.PlayerID <= 0 || len(raw.PlayerList) != 2 || raw.PlayerList[0].GetServerId() <= 0 || raw.PlayerList[1].GetServerId() <= 0 || raw.PlayerList[0].GetServerId() == raw.PlayerList[1].GetServerId() {
		return reject()
	}
	index := -1
	for i, player := range raw.PlayerList {
		if int64(player.GetServerId()) == viewer.ServerID && player.GetPlayerId() == viewer.PlayerID {
			if index >= 0 {
				return reject()
			}
			index = i
		}
	}
	if index < 0 {
		return reject()
	}
	return index, index == 1, nil
}

func (s *GameService) freezeP6KFReplayViewer(original *pb.KFReplayData, viewer *model.FrozenP6KFViewer) (*model.FrozenBattleInput, error) {
	if original == nil || len(original.PlayerList) < 2 {
		return nil, errors.New(constUtil.CombatInvalid)
	}
	raw := proto.Clone(original).(*pb.KFReplayData)
	index, view, err := p6KFViewerProjection(raw, viewer)
	if err != nil {
		return nil, err
	}
	maximum := raw.GetMaxRoundNum()
	if maximum <= 0 {
		maximum = 40
	}
	players := raw.PlayerList
	// Original initiative reads global source0/1 before viewer selection.
	return s.FreezeBattleInput(model.FrozenBattleInput{Attacker: players[index], Defender: players[(index+1)%2], KFReplay: raw, KFReplayViewer: viewer, Seed: raw.GetRandSeed(), MaxRound: maximum, BattleType: 17, IsAttToDef: view, DefenderFirst: players[0].GetInst().GetFirstOrder() == 0 && players[1].GetInst().GetFirstOrder() == 1})
}

func validateP6KFProjection(input *model.FrozenBattleInput) error {
	raw := input.KFReplay
	players := raw.GetPlayerList()
	reject := func() error { return errors.New(constUtil.P6ReplayConflict) }
	if input.BattleType != 17 || len(players) < 2 || input.Defender == nil || input.Monster != nil || input.Seed != raw.GetRandSeed() {
		return reject()
	}
	for _, player := range players {
		if player == nil {
			return reject()
		}
	}
	maximum := raw.GetMaxRoundNum()
	if maximum <= 0 {
		maximum = 40
	}
	if input.MaxRound != maximum {
		return reject()
	}
	index := -1
	view := false
	if input.KFReplayViewer != nil {
		var err error
		index, view, err = p6KFViewerProjection(raw, input.KFReplayViewer)
		if err != nil {
			return err
		}
	} else {
		// Historical diagnostic snapshots lacked viewer context. Keep their original
		// bytes readable under their legacy projection; never label them native-equal.
		for i, player := range players {
			if player.GetPlayerId() == input.Attacker.GetPlayerId() {
				index = i
				break
			}
		}
		view = index == 1
	}
	if index < 0 || input.IsAttToDef != view || !proto.Equal(input.Attacker, players[index]) || !proto.Equal(input.Defender, players[(index+1)%2]) {
		return reject()
	}
	return nil
}

// Original duplicate PID matching constructs more than one fighter pair. Keep
// the frozen source auditable but refuse a whole-result calculation until that
// original multi-pair progression is independently restored. The named local
// compatibility selects one joint identity before constructing a single pair.
func validateP6KFCalculationBoundary(input *model.FrozenBattleInput) error {
	if input.KFReplay == nil {
		return nil
	}
	if input.KFReplayViewer != nil && input.KFReplayViewer.CompatibilityRule == constUtil.P6KFJointViewerCompatibility {
		return nil
	}
	viewer := input.Attacker.GetPlayerId()
	if input.KFReplayViewer != nil {
		viewer = input.KFReplayViewer.PlayerID
	}
	count := 0
	for _, player := range input.KFReplay.PlayerList {
		if player.GetPlayerId() == viewer {
			count++
		}
	}
	if count > 1 {
		return errors.New(constUtil.CombatUnsupported)
	}
	return nil
}
