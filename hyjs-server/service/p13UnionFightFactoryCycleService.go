package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

func p13UFFactoryCycleOfferHash(row *db.P13UnionFightFactoryCycleOffer) string {
	copy := *row
	copy.SourceHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

func p13UFFactoryCycleStateHash(row *db.P13UnionFightFactoryCycleState) string {
	copy := *row
	copy.StateHash = ""
	raw, _ := json.Marshal(copy)
	return p6Digest(raw)
}

func (s *GameService) p13UFFactoryCycleBuild(tx *gorm.DB, match *db.P13UnionFightMatch, original *GameService, now time.Time) error {
	var sources []db.P13UnionFightFactoryCycleSource
	if err := tx.Where("day=? AND enabled=1", match.Day).Find(&sources).Error; err != nil {
		return err
	}
	for _, source := range sources {
		definition := original.table("UnionFightFactory", "UFFId", strconv.FormatInt(source.FactoryID, 10))
		if definition == nil || definition["CraftTime"] != "120" || source.MaxGenerations < 2 || source.MaxGenerations > 128 || source.MaxAlive < 1 || source.MaxAlive > 32 || source.MaxAlive > source.MaxGenerations || source.PublishedAt <= 0 || source.PublishedAt > now.UnixMilli() || strings.TrimSpace(source.Source) == "" {
			return errors.New("原120秒连续产车与提前具名产能来源缺失")
		}
		animation, err := strconv.ParseFloat(definition["factoryBuildAnimationTime"], 64)
		if err != nil || animation < 0 || animation >= 120 || math.IsNaN(animation) || math.IsInf(animation, 0) {
			return errors.New("原厂生产动画时间缺失")
		}
		guid := p13UFFieldGUID(match.ID, "factory", strconv.FormatInt(source.FactoryID, 10))
		var factory db.P13UnionFightFactoryState
		if err = tx.First(&factory, "match_id=? AND guid=?", match.ID, guid).Error; err != nil {
			return err
		}
		if _, err = p13UFFactoryFrame(&factory); err != nil {
			return err
		}
		if factory.ChariotHP <= 0 {
			return errors.New("连续产车需要原实际预发布初始HP来源")
		}
		quote := db.P13UnionFightFactoryCycleOffer{MatchID: match.ID, FactoryGUID: guid, CraftMillis: 120000, AnimationMillis: int64(math.Round(animation * 1000)), MaxGenerations: source.MaxGenerations, MaxAlive: source.MaxAlive, Source: constUtil.P13UnionFightFactoryCycleSource + "；发布：" + source.Source}
		quote.SourceHash = p13UFFactoryCycleOfferHash(&quote)
		if err = tx.Create(&quote).Error; err != nil {
			return err
		}
	}
	return nil
}

// TickP13UnionFightFactoryCycle 实际首车完成后按原120秒窗口推进，具名本地产能阻塞窗口不追补。
func (s *GameService) TickP13UnionFightFactoryCycle(ctx context.Context, now time.Time) error {
	var offers []db.P13UnionFightFactoryCycleOffer
	if err := db.GetIns().WithContext(ctx).Where("match_id IN (?)", db.GetIns().WithContext(ctx).Model(&db.P13UnionFightMatch{}).Select("id").Where("start_at<=? AND end_at>?", now.UnixMilli(), now.UnixMilli())).Order("match_id,factory_guid").Find(&offers).Error; err != nil {
		return err
	}
	for _, candidate := range offers {
		var frames []proto.Message
		err := db.GetIns().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := ServerGameClockGuardTransaction(tx); err != nil {
				return err
			}

			var guild db.P13GuildPolicy
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&guild, 1).Error; err != nil {
				return err
			}
			var policy db.P13UnionFightPolicy
			if err := tx.First(&policy, 1).Error; err != nil {
				return err
			}
			var field db.P13UnionFightFieldPolicy
			if err := tx.First(&field, 1).Error; err != nil {
				return err
			}
			if !guild.ManagementEnabled || !policy.EnrollmentEnabled || !policy.MatchEnabled || !field.Enabled {
				return nil
			}
			var match db.P13UnionFightMatch
			if err := tx.First(&match, "id=?", candidate.MatchID).Error; err != nil {
				return err
			}
			if now.UnixMilli() < match.StartAt || now.UnixMilli() >= match.EndAt {
				return nil
			}
			var count int64
			if err := tx.Model(&db.P13Guild{}).Where("id IN ?", []string{match.LeftGuildID, match.RightGuildID}).Count(&count).Error; err != nil {
				return err
			}
			if count != 2 {
				return nil
			}
			var quote db.P13UnionFightFactoryCycleOffer
			if err := tx.First(&quote, "match_id=? AND factory_guid=?", candidate.MatchID, candidate.FactoryGUID).Error; err != nil {
				return err
			}
			if quote.SourceHash != p13UFFactoryCycleOfferHash(&quote) || quote.CraftMillis != 120000 || quote.AnimationMillis < 0 || quote.AnimationMillis >= quote.CraftMillis || quote.MaxGenerations < 2 || quote.MaxGenerations > 128 || quote.MaxAlive < 1 || quote.MaxAlive > 32 || quote.MaxAlive > quote.MaxGenerations {
				return errors.New("连续车厂冻结来源SHA/产能不符")
			}
			factory, frame, definition, err := s.p13UFFactoryRead(tx, &match, quote.FactoryGUID)
			if err != nil {
				return err
			}
			if factory.CompletedAt <= 0 || frame.GetIsOpen() != 1 || frame.GetState() == 2 || frame.GetState() == 4 {
				return nil
			}
			var state db.P13UnionFightFactoryCycleState
			err = tx.First(&state, "match_id=? AND factory_guid=?", quote.MatchID, quote.FactoryGUID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				state = db.P13UnionFightFactoryCycleState{MatchID: quote.MatchID, FactoryGUID: quote.FactoryGUID, Generation: 2, NextBornAt: factory.CompletedAt + quote.CraftMillis, OfferHash: quote.SourceHash}
				state.StateHash = p13UFFactoryCycleStateHash(&state)
			} else if err != nil {
				return err
			}
			if state.StateHash != p13UFFactoryCycleStateHash(&state) || state.OfferHash != quote.SourceHash || state.Generation < 2 || state.Generation > quote.MaxGenerations+1 || state.NextBornAt < factory.CompletedAt+quote.CraftMillis {
				return errors.New("连续厂周期时序/SHA损坏")
			}
			if state.ClosedAt > 0 {
				return nil
			}
			if state.Generation > quote.MaxGenerations || state.NextBornAt >= match.EndAt {
				state.ClosedAt = now.UnixMilli()
			} else {
				rows, err := s.p13UFCarScopeRows(tx, match.ID, nil)
				if err != nil {
					return err
				}
				alive := int64(0)
				for _, carRow := range rows {
					if carRow.GUID != factory.GUID {
						continue
					}
					car, err := p13UFCarItemRead(&carRow)
					if err != nil {
						return err
					}
					if car.GetHp() <= 0 {
						continue
					}
					var attacking db.P13UnionFightCarAttackState
					err = tx.First(&attacking, "match_id=? AND factory_guid=?", match.ID, p13UFCarStateGUID(&carRow)).Error
					if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
						return err
					}
					if err == nil && attacking.Completed {
						if _, err = p13UFCarAttackFrames(tx, &carRow); err != nil {
							return err
						}
						continue
					}
					alive++
				}
				if alive >= quote.MaxAlive {
					for state.NextBornAt <= now.UnixMilli() {
						state.NextBornAt += quote.CraftMillis
						state.AnimationPlayed = false
					}
				} else if now.UnixMilli() >= state.NextBornAt {
					carID, err := strconv.ParseInt(definition["CariotId"], 10, 32)
					if err != nil {
						return err
					}
					shadow := *factory
					shadow.CarGeneration = state.Generation
					shadow.StartedAt = state.NextBornAt - quote.CraftMillis
					shadow.DueAt = state.NextBornAt
					shadow.AnimationAt = state.NextBornAt - quote.AnimationMillis
					shadow.CompletedAt = state.NextBornAt
					car := &pb.InstUFCariot{Guid: proto.Int64(p13UFCarInstanceGUID(&shadow)), CariotId: proto.Int32(int32(carID)), Camp: proto.Int32(int32(factory.Camp)), FactoryGuid: proto.Int64(factory.GUID), Hp: proto.Float64(factory.ChariotHP), MaxHp: proto.Float64(factory.ChariotHP), IsSlowDown: proto.Int32(0)}
					shadow.CarFrame, err = proto.Marshal(car)
					if err != nil {
						return err
					}
					shadow.CarHash = p6Digest(shadow.CarFrame)
					shadow.StateHash = p13UFFactoryStateDigest(&shadow)
					raw, err := json.Marshal(shadow)
					if err != nil {
						return err
					}
					record := db.P13UnionFightCarGeneration{MatchID: match.ID, CarGUID: car.GetGuid(), FactoryGUID: factory.GUID, Generation: state.Generation, BornAt: state.NextBornAt, FactorySnapshot: raw, OfferHash: quote.SourceHash, CarFrame: shadow.CarFrame, CarHash: shadow.CarHash}
					record.SourceHash = p13UFCarGenerationHash(&record)
					record.StateHash = p13UFCarGenerationStateHash(&record)
					if err = tx.Create(&record).Error; err != nil {
						return err
					}
					point, err := fieldPoint(definition["CariotBornPos"])
					if err != nil {
						return err
					}
					frames = append(frames, &pb.SCUFCariotFactoryBuildComplete{FactoryGuid: proto.Int64(factory.GUID), Cariot: car, Pos: point})
					frame.CariotGuid = car.Guid
					state.Generation++
					state.NextBornAt += quote.CraftMillis
					state.AnimationPlayed = false
				} else if !state.AnimationPlayed && now.UnixMilli() >= state.NextBornAt-quote.AnimationMillis {
					state.AnimationPlayed = true
					frames = append(frames, &pb.SCUFFactoryBuildPlay{Guid: proto.Int64(factory.GUID)})
				}
			}
			// 原state3回到working0；新代实体独立，不替换首车记录。
			frame.State = proto.Int32(0)
			if state.AnimationPlayed && state.ClosedAt == 0 {
				frame.State = proto.Int32(1)
			}
			if err = p13UFFactorySave(tx, factory, frame); err != nil {
				return err
			}
			state.StateHash = p13UFFactoryCycleStateHash(&state)
			return tx.Save(&state).Error
		})
		if err != nil {
			return err
		}
		for _, frame := range frames {
			s.p13UFFieldBroadcast(ctx, candidate.MatchID, 0, frame)
		}
	}
	return nil
}
