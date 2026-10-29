package service

import (
	"bytes"
	"errors"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// Refresh only already matched participants under the caller's profile and
// arena policy locks. Registration and virtual-slot migration remain at matching.
func (s *GameService) p11RefreshArenaParticipant(tx *gorm.DB, profile *db.Profile, member *db.P11ArenaMember, now time.Time) error {
	if profile.Id != member.PlayerID || !member.Active || member.Rank < 1 {
		return errors.New(constUtil.P11ArenaFightRefreshRequired)
	}
	info, battle, err := s.p11ArenaProjection(profile, tx)
	if err != nil {
		return err
	}
	rawInfo, err := proto.Marshal(info)
	if err != nil {
		return err
	}
	rawBattle, err := proto.Marshal(battle)
	if err != nil {
		return err
	}
	hash := p6Digest(append(append([]byte(nil), rawInfo...), rawBattle...))
	if bytes.Equal(member.Info, rawInfo) && bytes.Equal(member.Battle, rawBattle) && member.SnapshotHash == hash {
		return nil
	}
	member.Info, member.Battle, member.SnapshotHash, member.UpdatedAt = rawInfo, rawBattle, hash, now.UnixMilli()
	return tx.Model(member).Updates(map[string]any{"info": rawInfo, "battle": rawBattle, "snapshot_hash": hash, "updated_at": member.UpdatedAt}).Error
}
