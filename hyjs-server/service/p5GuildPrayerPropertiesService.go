package service

import (
	"context"
	"encoding/hex"
	"errors"
	"math"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// PrepareP5GuildPrayerProperties captures current prayer and guild tech in the
// caller's existing transaction. It never persists the projection or opens a
// second connection while the profile is locked.
func (s *GameService) PrepareP5GuildPrayerProperties(ctx context.Context, tx *gorm.DB, player *pb.Player, game *model.GameState, now time.Time) error {
	if tx == nil || player == nil || game == nil || player.GetPlayerId() <= 0 || now.UnixMilli() <= 0 {
		return p5Error()
	}
	game.GuildPrayerProjection = nil
	prop, source, expires, err := s.P13GuildPrayerProperties(ctx, tx, player.GetPlayerId(), now)
	if err != nil {
		return err
	}
	if prop == nil {
		return errors.New("公会祈祷属性来源缺失")
	}
	projection := &model.P5GuildPrayerProjection{PlayerID: player.GetPlayerId(), Properties: proto.Clone(prop).(*pb.BaseProp), SourceHash: source, CapturedAt: now.UnixMilli(), ExpiresAt: expires}
	if err := p5GuildPrayerProjectionValid(projection, player.GetPlayerId()); err != nil {
		return err
	}
	tech, techSource, err := s.P13GuildTechProperties(ctx, tx, player.GetPlayerId(), now)
	if err != nil {
		return err
	}
	if tech == nil {
		return errors.New("公会科技属性来源缺失")
	}
	projection.TechProperties = proto.Clone(tech).(*pb.BaseProp)
	projection.TechSourceHash = techSource
	if err := p5GuildTechProjectionValid(projection, player.GetPlayerId()); err != nil {
		return err
	}
	game.GuildPrayerProjection = projection
	return nil
}

// RefreshP5GuildPrayerPower refreshes only actual current guild sources, an
// actual pre-mutation tech source or historical prayer. Inactive default-off
// players retain their cached state; membership changes remove active bonuses.
func (s *GameService) RefreshP5GuildPrayerPower(ctx context.Context, tx *gorm.DB, player *pb.Player, game *model.GameState, now time.Time) error {
	// Preserve the actual pre-mutation source only within this transaction.
	previousTech := game != nil && game.GuildPrayerProjection != nil && game.GuildPrayerProjection.TechSourceHash != ""
	if err := s.PrepareP5GuildPrayerProperties(ctx, tx, player, game, now); err != nil {
		return err
	}
	if game.GuildPrayerProjection.SourceHash == "" && game.GuildPrayerProjection.TechSourceHash == "" && !previousTech {
		if !tx.Migrator().HasTable(&db.P13GuildPrayerRoll{}) {
			return nil
		}
		var existed int64
		if err := tx.WithContext(ctx).Model(&db.P13GuildPrayerRoll{}).Where("player_id=?", player.GetPlayerId()).Limit(1).Count(&existed).Error; err != nil {
			return err
		}
		if existed == 0 {
			return nil
		}
	}
	power, err := s.formationFightPower(player, game)
	if err != nil {
		return err
	}
	player.FightPower = proto.Int32(power)
	return nil
}

func p5GuildPrayerProjectionValid(source *model.P5GuildPrayerProjection, id int64) error {
	if source == nil || source.Properties == nil || source.PlayerID != id || source.CapturedAt <= 0 {
		return p5Error()
	}
	if source.SourceHash == "" && source.ExpiresAt == 0 && proto.Equal(source.Properties, &pb.BaseProp{}) {
		return nil
	}
	if source.ExpiresAt <= source.CapturedAt {
		return p5Error()
	}
	if raw, err := hex.DecodeString(source.SourceHash); err != nil || len(raw) != 32 {
		return p5Error()
	}
	valid := true
	source.Properties.ProtoReflect().Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		number := value.Float()
		if math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
			valid = false
			return false
		}
		switch string(field.Name()) {
		case "peratt", "perhp", "perpdef", "permdef":
		default:
			valid = number == 0
		}
		return valid
	})
	if !valid {
		return p5Error()
	}
	return nil
}

// AddP5GuildPrayerPercent matches SetUnionChurchAttribute's percentage-point
// conversion and ResetAttribute's additive percentage stage before gems/pets.
// The caller already captured one transaction time for the entire formation.
func AddP5GuildPrayerPercent(player *pb.Player, game *model.GameState, percent map[string]float64) error {
	if game == nil || player == nil || percent == nil {
		return p5Error()
	}
	if game.GuildPrayerProjection == nil {
		return nil
	}
	if err := p5GuildPrayerProjectionValid(game.GuildPrayerProjection, player.GetPlayerId()); err != nil {
		return err
	}
	if err := p5GuildTechProjectionValid(game.GuildPrayerProjection, player.GetPlayerId()); err != nil {
		return err
	}
	prop := game.GuildPrayerProjection.Properties
	percent["att"] += prop.GetPeratt() * .01
	percent["hp"] += prop.GetPerhp() * .01
	percent["pdef"] += prop.GetPerpdef() * .01
	percent["mdef"] += prop.GetPermdef() * .01
	tech := game.GuildPrayerProjection.TechProperties
	percent["att"] += tech.GetPeratt() * .01
	percent["hp"] += tech.GetPerhp() * .01
	percent["pdef"] += tech.GetPerpdef() * .01
	percent["mdef"] += tech.GetPermdef() * .01
	return nil
}

// p5GuildTechProjectionValid accepts only the original four percent and four
// constant attributes. Empty optional tech remains compatible with old callers.
func p5GuildTechProjectionValid(source *model.P5GuildPrayerProjection, id int64) error {
	if source == nil || source.PlayerID != id || source.CapturedAt <= 0 {
		return p5Error()
	}
	if source.TechSourceHash == "" {
		if source.TechProperties == nil || proto.Equal(source.TechProperties, &pb.BaseProp{}) {
			return nil
		}
		return p5Error()
	}
	if source.TechProperties == nil {
		return p5Error()
	}
	if raw, err := hex.DecodeString(source.TechSourceHash); err != nil || len(raw) != 32 {
		return p5Error()
	}
	valid := true
	source.TechProperties.ProtoReflect().Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		number := value.Float()
		if math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
			valid = false
			return false
		}
		switch string(field.Name()) {
		case "peratt", "perhp", "perpdef", "permdef", "hit", "dodge", "cri", "antiCri":
		default:
			valid = number == 0
		}
		return valid
	})
	if !valid {
		return p5Error()
	}
	return nil
}

// P5GuildTechConstants belongs after base*(1+allPercent), before gems and pets.
// Pure projection never performs SQL or changes a previously frozen BattleHero.
func P5GuildTechConstants(player *pb.Player, game *model.GameState) (map[string]float64, error) {
	values := map[string]float64{}
	if player == nil || game == nil {
		return nil, p5Error()
	}
	if game.GuildPrayerProjection == nil {
		return values, nil
	}
	if err := p5GuildTechProjectionValid(game.GuildPrayerProjection, player.GetPlayerId()); err != nil {
		return nil, err
	}
	prop := game.GuildPrayerProjection.TechProperties
	values["hit"] = prop.GetHit()
	values["dodge"] = prop.GetDodge()
	values["cri"] = prop.GetCri()
	values["antiCri"] = prop.GetAntiCri()
	return values, nil
}

// WithP5GuildMembershipRevocation captures an actual active guild source before
// leave/kick/dissolve and refreshes that same target after the original mutation.
// The second decode preserves the position saved by room/battle revocation.
func (s *GameService) WithP5GuildMembershipRevocation(ctx context.Context, tx *gorm.DB, id int64, now time.Time, mutation func() error) error {
	var before db.Profile
	if err := tx.First(&before, id).Error; err != nil {
		return err
	}
	player, game, err := decodeProfileForCalendarUpdate(tx, &before)
	if err != nil {
		return err
	}
	if err = s.PrepareP5GuildPrayerProperties(ctx, tx, player, game, now); err != nil {
		return err
	}
	previous := game.GuildPrayerProjection.Clone()
	if err = mutation(); err != nil {
		return err
	}
	if previous.SourceHash == "" && previous.TechSourceHash == "" {
		return nil
	}
	var current db.Profile
	if err = tx.First(&current, id).Error; err != nil {
		return err
	}
	player, game, err = decodeProfileForCalendarUpdate(tx, &current)
	if err != nil {
		return err
	}
	game.GuildPrayerProjection = previous
	if err = s.RefreshP5GuildPrayerPower(ctx, tx, player, game, now); err != nil {
		return err
	}
	return saveProfile(tx, &current, player, game)
}
