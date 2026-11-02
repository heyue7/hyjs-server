package service

import (
	"fmt"
	"maps"
	"strconv"
	"testing"

	pb "hyjs-server/model/protocol"

	"google.golang.org/protobuf/proto"
)

// Original IL: FSkill.Do 020e–0399 compares the cached FPosition and rule;
// UpdateSkillBegin 001d–0044 assigns the saved position at the callback.
func p6CastPositionRound2Skill(actor *combatRoundActor, rule int32, ultimate bool) *combatRoundSkill {
	skill := *actor.skills[0]
	skill.row = maps.Clone(skill.row)
	skill.row["SkillCastPointSelectRule"] = strconv.FormatInt(int64(rule), 10)
	skill.row["IsUltimate"] = "0"
	if ultimate {
		skill.row["IsUltimate"] = "1"
	}
	skill.row["Duration"] = "10"
	skill.casts = []map[string]string{maps.Clone(skill.casts[0])}
	skill.casts[0]["ExportType"] = "1"
	skill.casts[0]["SelectStrategy"] = "3"
	skill.releases = nil
	return &skill
}

func p6CastPositionRound2Actor(template *combatRoundActor, guid string, camp, pose int32) *combatRoundActor {
	actor := *template
	actor.hero = proto.Clone(template.hero).(*pb.BattleHero)
	actor.hero.Guid = proto.String(guid)
	actor.uid = fmt.Sprintf("%d_%s", camp, guid)
	actor.camp = camp
	actor.runtimePose = pose
	actor.current, actor.waiting = nil, nil
	actor.skills, actor.buffs, actor.talents = nil, nil, nil
	actor.state = 0
	actor.resetCastPosition()
	actor.waitUntil = 0
	return &actor
}

func p6CastPositionRound2Move(e *combatRoundEngine, actor *combatRoundActor) float64 {
	if actor.heroClass == 1 {
		return e.number("monsterMoveSkillTime")
	}
	return e.number("moveToSkillTime")
}

func p6CastPositionRound2Clean(t *testing.T, actor *combatRoundActor) {
	t.Helper()
	if actor.movedTarget != nil || actor.castPositionOwner != nil || actor.movedRule != -1 {
		t.Fatalf("position/cache not reset: target=%p owner=%p rule=%d", actor.movedTarget, actor.castPositionOwner, actor.movedRule)
	}
}

func TestP6CastPositionRound2PositionAndRuleValues(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	actor, template := e.players[0].actors[0], e.players[1].actors[0]
	beforeAttacker, beforeDefender := proto.Clone(e.input.Attacker), proto.Clone(e.input.Defender)
	wantMove := p6CastPositionRound2Move(e, actor)
	if wantMove <= 0 {
		t.Fatal("fixture has no normal movement duration", wantMove)
	}
	for _, tc := range []struct {
		name       string
		camp, pose int32
		rule       int32
		move       bool
	}{
		{"different owners same camp row column", 1, 1, 1, false},
		{"different column", 1, 2, 1, true},
		{"different row", 1, 6, 1, true},
		{"different camp", 0, 1, 1, true},
		{"different rule", 1, 1, 2, true},
		{"rule four retains earlier cache", 1, 6, 4, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cached := p6CastPositionRound2Actor(template, "cached", 1, 1)
			owner := p6CastPositionRound2Actor(template, "position-owner", tc.camp, tc.pose)
			target := p6CastPositionRound2Actor(template, "different-target", 1, 15)
			target.castPositionOwner = owner
			e.players[1].actors = []*combatRoundActor{target}
			actor.current, actor.waiting = nil, nil
			actor.state = 0
			actor.movedTarget, actor.castPositionOwner, actor.movedRule = cached, cached, 1
			skill := p6CastPositionRound2Skill(actor, tc.rule, false)
			e.beginSkill(skill)
			if skill.target != target {
				t.Fatal("skill did not select expected target")
			}
			if tc.move {
				if skill.move != wantMove || actor.movedTarget != owner || actor.movedRule != tc.rule {
					t.Fatalf("position/rule change not moved and cached: move=%v rule=%d", skill.move, actor.movedRule)
				}
			} else if skill.move != 0 || actor.movedTarget != cached || actor.movedRule != 1 {
				t.Fatalf("same position or rule four replaced cache: move=%v rule=%d", skill.move, actor.movedRule)
			}
			if actor.castPositionOwner != cached {
				t.Fatal("position changed before first SkillBegin callback")
			}
		})
	}
	if !proto.Equal(beforeAttacker, e.input.Attacker) || !proto.Equal(beforeDefender, e.input.Defender) {
		t.Fatal("source battle heroes changed")
	}
}

func TestP6CastPositionRound2MovingTargetAndCallback(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	e.round = 1
	e.supplied = &pb.BattleOperate{}
	actor, target := e.players[0].actors[0], e.players[1].actors[0]
	beforeAttacker, beforeDefender := proto.Clone(e.input.Attacker), proto.Clone(e.input.Defender)
	anchor := p6CastPositionRound2Actor(actor, "moved-target-owner", 0, 6)
	target.castPositionOwner = anchor
	first := p6CastPositionRound2Skill(actor, 1, false)
	e.beginSkill(first)
	if first.target != target || first.move != p6CastPositionRound2Move(e, actor) || first.move <= 0 || actor.movedTarget != anchor || actor.castPositionOwner != nil {
		t.Fatal("first cast did not cache target's current position before callback")
	}
	if err := e.step(); err != nil {
		t.Fatal(err)
	}
	if !first.beginNotified || first.stage != 0 || actor.castPositionOwner != anchor {
		t.Fatal("first callback did not assign cached position before movement finished")
	}
	// Resetting the target changes its own reference, leaving the cached home
	// position object in place for the attacker.
	target.resetCastPosition()
	if actor.movedTarget != anchor || actor.castPositionOwner != anchor {
		t.Fatal("target reset changed the attacker's cached position")
	}
	other := p6CastPositionRound2Actor(target, "other-uid", 1, 2)
	other.castPositionOwner = anchor
	e.players[1].actors = []*combatRoundActor{other}
	actor.current = nil
	second := p6CastPositionRound2Skill(actor, 1, false)
	e.beginSkill(second)
	if second.target != other || second.move != 0 || actor.movedTarget != anchor {
		t.Fatal("different UID at the same current position caused another movement")
	}
	// Formation changes the referenced MainPosition in place; another owner
	// with the same value must still compare equal.
	anchor.runtimePose = 7
	equalOwner := p6CastPositionRound2Actor(actor, "equal-position-owner", 0, 7)
	other.castPositionOwner = equalOwner
	actor.current = nil
	third := p6CastPositionRound2Skill(actor, 1, false)
	e.beginSkill(third)
	if third.move != 0 || actor.movedTarget != anchor {
		t.Fatal("equal position after home mutation replaced cached owner")
	}
	other.castPositionOwner = p6CastPositionRound2Actor(actor, "new-position-owner", 0, 8)
	actor.current = nil
	fourth := p6CastPositionRound2Skill(actor, 1, false)
	e.beginSkill(fourth)
	if fourth.move != p6CastPositionRound2Move(e, actor) || actor.movedTarget != other.castPositionOwner || actor.castPositionOwner != anchor {
		t.Fatal("target's new current position was not cached for a new movement")
	}
	if err := e.step(); err != nil {
		t.Fatal(err)
	}
	if actor.castPositionOwner != other.castPositionOwner {
		t.Fatal("subsequent callback did not apply the newly cached position")
	}
	if !proto.Equal(beforeAttacker, e.input.Attacker) || !proto.Equal(beforeDefender, e.input.Defender) {
		t.Fatal("source battle heroes changed")
	}
}

func TestP6CastPositionRound2Resets(t *testing.T) {
	e := p6PropertyTransferEngine(t)
	e.round = 1
	e.supplied = &pb.BattleOperate{}
	actor, target := e.players[0].actors[0], e.players[1].actors[0]
	setMoved := func() {
		actor.movedTarget, actor.castPositionOwner, actor.movedRule = target, target, 1
		actor.state, actor.current, actor.waiting = 0, nil, nil
	}
	t.Run("ultimate", func(t *testing.T) {
		setMoved()
		skill := p6CastPositionRound2Skill(actor, 1, true)
		e.beginSkill(skill)
		if skill.target != target || skill.move != e.number("ultMoveToSkillTime") {
			t.Fatal("ultimate did not retain its configured move timing")
		}
		p6CastPositionRound2Clean(t, actor)
	})
	t.Run("round", func(t *testing.T) {
		setMoved()
		if err := e.beginRound(); err != nil {
			t.Fatal(err)
		}
		p6CastPositionRound2Clean(t, actor)
	})
	t.Run("wait and expiry", func(t *testing.T) {
		setMoved()
		e.finishOriginalSkillActorState(actor)
		if actor.state != 2 || actor.waitUntil <= e.time || actor.movedTarget != target || actor.castPositionOwner != target {
			t.Fatal("waiting did not retain the current position")
		}
		e.time = actor.waitUntil
		e.commands = nil
		if err := e.step(); err != nil {
			t.Fatal(err)
		}
		p6CastPositionRound2Clean(t, actor)
		if actor.waitUntil != 0 {
			t.Fatal("expired wait deadline retained")
		}
	})
	t.Run("idle", func(t *testing.T) {
		setMoved()
		for _, skill := range actor.skills {
			skill.state = 4
		}
		actor.waitUntil = 10
		e.finishOriginalSkillActorState(actor)
		p6CastPositionRound2Clean(t, actor)
		if actor.state != 0 || actor.waitUntil != 0 {
			t.Fatal("idle did not clear wait state")
		}
	})
}

func TestP6CastPositionRound2ReboundRestoresHome(t *testing.T) {
	e := p6ReboundEngine(t)
	source, target, enemy := e.players[0].actors[0], e.players[0].actors[1], e.players[1].actors[0]
	if err := e.completeActorDeath(target); err != nil {
		t.Fatal(err)
	}
	target.movedTarget, target.castPositionOwner, target.movedRule, target.waitUntil = enemy, enemy, 1, 10
	if err := e.loadBuff(source, target, e.service.table("Buff", "Id", "buff_457"), 2); err != nil {
		t.Fatal(err)
	}
	if target.hp <= 0 || target.state != 0 || target.waitUntil != 0 {
		t.Fatal("fixture did not revive target")
	}
	p6CastPositionRound2Clean(t, target)
}
