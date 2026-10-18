package service

import (
	"context"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// P9PetRecruitInfo projects only a pending original first reward into the native
// unused reward preview string. Ordinary pool, free-count and pity remain actual state.
func (s *GameService) P9PetRecruitInfo(ctx context.Context, id int64) ([]proto.Message, error) {
	out := &pb.SCPetRecruitGet{}
	err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := ServerGameClockGuardTransaction(tx); err != nil {
			return err
		}

		var profile db.Profile
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&profile, id).Error; err != nil {
			return err
		}
		p, _, err := decodeProfile(&profile)
		if err != nil {
			return err
		}
		state, err := p9State(tx, id)
		if err != nil {
			return err
		}
		reward := ""
		if !state.FirstGranted && p.GetLevel() >= int32(tableNumber(s.table("GameConfigure", "StaticName", "PetUnlockLevel"), "NameValue")) {
			spec := s.table("GameConfigure", "StaticName", "PetFirstRecruitGet")["NameValue"]
			pair := strings.Split(spec, ",")
			if len(pair) != 2 || pair[1] != strconv.Itoa(1) || s.table("Pet", "id", pair[0]) == nil {
				return p5Error()
			}
			reward = spec
		}
		if state.FirstGranted && s.p9Open(p) == nil {
			_, _, err := s.p9RecruitProgress(tx, id, state, time.Now().UnixMilli())
			if err != nil && err.Error() != constUtil.P9RecruitDisabled {
				return err
			}
			if err != nil {
				state.FreeRemaining = 0
				state.NextFree = 0
				state.MustTimes = 0
			}
		}
		out.Recruit = &pb.PetRecruit{NextFree: proto.Int64(state.NextFree), Num: proto.Int32(state.FreeRemaining), MustTimes: proto.Int32(state.MustTimes), Reward: proto.String(reward)}
		return nil
	})
	return []proto.Message{out}, err
}
