package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p11ArenaGuideSourceLabel(ctx context.Context) string {
	if _, active := ctx.Value(constUtil.P11ArenaGuideContextKey{}).(int64); active {
		return constUtil.P11ArenaGuideSource
	}
	return constUtil.P11ArenaModernSource
}

// p11ArenaGuideQueryProfile checks only the current player projection inside
// the existing query transaction; normal queries do no extra JSON decoding.
func p11ArenaGuideQueryProfile(ctx context.Context, row *db.Profile) error {
	if _, active := ctx.Value(constUtil.P11ArenaGuideContextKey{}).(int64); !active {
		return nil
	}
	player := &pb.Player{}
	if row == nil || row.Player == nil || json.Unmarshal([]byte(*row.Player), player) != nil {
		return errors.New(constUtil.P11ArenaGuideDenied)
	}
	return p11ArenaGuidePrepare(ctx, player, nil, nil, false)
}

// p11ArenaGuidePrepare runs inside the existing query/settlement transaction.
// Original Guide SetData forces attacker-first; modern SetData reads Inst. Only
// an actual 1/0 pair has the same interpretation in both original consumers.
func p11ArenaGuidePrepare(ctx context.Context, player *pb.Player, attacker, defender *pb.BattlePlayer, complete bool) error {
	id, active := ctx.Value(constUtil.P11ArenaGuideContextKey{}).(int64)
	if !active {
		return nil
	}
	if id <= 0 || player == nil || player.GetPlayerId() != id || player.GetLevel() < constUtil.P11ArenaGuideLevel || player.GetNoviceGuideSchedule() < 0 || player.GetNoviceGuideSchedule() >= constUtil.P11ArenaGuideCompleted {
		return errors.New(constUtil.P11ArenaGuideDenied)
	}
	if attacker != nil || defender != nil {
		if attacker == nil || defender == nil || attacker.GetPlayerId() != id || attacker.GetInst() == nil || defender.GetInst() == nil || attacker.GetInst().GetFirstOrder() != 1 || defender.GetInst().GetFirstOrder() != 0 || len(attacker.GetFormation()) == 0 || len(defender.GetFormation()) == 0 {
			return errors.New(constUtil.P11ArenaGuideShape)
		}
	}
	if complete {
		// The original guide waits for returning to ArenaPage after the battle.
		// Accepted real defeat also completes the tutorial; no win is invented.
		player.NoviceGuideSchedule = proto.Int32(constUtil.P11ArenaGuideCompleted)
	}
	return nil
}

// p11ArenaGuideAt preserves the original Guide request/response types. Its
// existing Arena transaction owns actual matching, computation, rank, costs,
// rewards, current SCPlayer and historical receipt verification.
func (s *GameService) p11ArenaGuideAt(ctx context.Context, id int64, request proto.Message, stamp int64, now time.Time) ([]proto.Message, error) {
	ctx = context.WithValue(ctx, constUtil.P11ArenaGuideContextKey{}, id)
	var (
		results []proto.Message
		err     error
		output  proto.Message
	)
	switch b := request.(type) {
	case *pb.CSPvpFindMatchGuide:
		results, err = s.p11ArenaQuery(ctx, id, &pb.CSPvpFindMatch{}, stamp, now)
		if err == nil {
			listing, ok := results[0].(*pb.SCPvpFindMatch)
			if !ok || len(listing.GetRankList()) < 2 {
				return nil, errors.New(constUtil.P11ArenaGuidePeers)
			}
			output = &pb.SCPvpFindMatchGuide{}
		}
	case *pb.CSPvpFightResultGuide:
		results, err = s.p11ArenaScopedFightAt(ctx, id, &pb.CSPvpFightResult{PlayerId: b.PlayerId, ServerId: b.ServerId}, stamp, now)
		output = &pb.SCPvpFightResultGuide{}
	default:
		return nil, errors.New(constUtil.P11ArenaGuideDenied)
	}
	if err != nil {
		return nil, err
	}
	// Original generated descriptors have identical field tags/types for these
	// two Guide pairs. Marshal/unmarshal retains nested Inst, GUIDs, every skill,
	// real result and previews while emitting the original Guide message ID.
	raw, err := proto.Marshal(results[0])
	if err != nil {
		return nil, err
	}
	if err = proto.Unmarshal(raw, output); err != nil {
		return nil, err
	}
	results[0] = output
	return results, nil
}
