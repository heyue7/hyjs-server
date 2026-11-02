package service

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"hyjs-server/model"
)

// These frozen actual fights and their native results are pinned by
// analysis/round-two-performance-final/arena-source-native.json.
func TestP6ArenaTakeoverActual(t *testing.T) {
	if os.Getenv("HYJS_P6_ARENA_TAKEOVER_TEST") != "1" {
		t.Skip("actual arena takeover fixture is explicitly disabled")
	}
	root := os.Getenv("HYJS_P6_ARENA_TAKEOVER_ROOT")
	proof := os.Getenv("HYJS_P6_ARENA_TAKEOVER_NATIVE")
	output := os.Getenv("HYJS_P6_ARENA_TAKEOVER_OUTPUT")
	if !filepath.IsAbs(root) || !filepath.IsAbs(proof) || !filepath.IsAbs(output) {
		t.Fatal("absolute private fixture, native evidence and output paths required")
	}
	rawProof, err := os.ReadFile(proof)
	if err != nil || p6Digest(rawProof) != "04edf0ae68872fc08c8c68e3eaed91b20e5963dff68ee2e578677e8766217aec" {
		t.Fatal("native evidence missing or digest changed", err)
	}
	var native struct {
		Cases []struct {
			FightID     string `json:"fightId"`
			InputSHA    string `json:"frozenInputSHA256"`
			TraceSHA    string `json:"traceSHA256"`
			AttackerWon bool   `json:"attackerWon"`
			Round       int32  `json:"round"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(rawProof, &native); err != nil || len(native.Cases) != 4 {
		t.Fatal("native evidence case count", err)
	}
	pins := []struct {
		id, inputSHA string
		won          bool
		round        int32
	}{
		{"4d230968-246e-46f5-8d79-2f3327e08a45", "4f3e1ecf8861d863dd63845fd2d2619c987905a18d73eaed9796bc0c0c324fd9", true, 15},
		{"d48fa9ec-d2bd-44cc-a3b3-8dda681451de", "ca396f819d47757fedd56a99e5412ee5c950e5bf598a10279138cbbe5a570a18", false, 10},
		{"19c10bec-7929-42fc-8ce9-0aece4625f90", "872fb309c8a5632f6dececc1c250e17545ff9de82934e32acde30806ea4a885f", false, 10},
		{"af336865-d08f-4bcd-83ce-b9cc9c9f8ad9", "1a25ea55e7a28e39cfd983ff1ae29cd3b5228cd40725a80461b07db71d222335", false, 10},
	}
	for i, pin := range pins {
		actual := native.Cases[i]
		if actual.FightID != pin.id || actual.InputSHA != pin.inputSHA || actual.AttackerWon != pin.won || actual.Round != pin.round || actual.TraceSHA == "" {
			t.Fatalf("native evidence case %d changed", i+1)
		}
	}
	// Require a new private output root so no previous run can be overwritten.
	if err := os.Mkdir(output, 0700); err != nil {
		t.Fatal("create new private output root", err)
	}
	for i, pin := range pins {
		i, pin := i, pin
		t.Run(fmt.Sprintf("case_%d", i+1), func(t *testing.T) {
			p6ArenaTakeoverActualCase(t, root, output, pin.id, pin.inputSHA, native.Cases[i].TraceSHA, pin.won, pin.round)
		})
	}
}

func p6ArenaTakeoverActualCase(t *testing.T, root, output, id, inputSHA, nativeTraceSHA string, nativeWon bool, nativeRound int32) {
	t.Helper()
	const maxTicks = 20000
	const maxEvents = 5000
	caseDir := filepath.Join(output, id)
	if err := os.Mkdir(caseDir, 0700); err != nil {
		t.Fatal("create private case directory", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, id, "input.json"))
	if err != nil || p6Digest(raw) != inputSHA {
		t.Fatal("actual frozen input missing or digest changed", err)
	}
	var input model.FrozenBattleInput
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal("actual frozen input decode failed")
	}
	before, err := json.Marshal(&input)
	if err != nil {
		t.Fatal("actual frozen input cannot be encoded")
	}
	tickFile, err := os.OpenFile(filepath.Join(caseDir, "ticks.jsonl"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal("create private tick output", err)
	}
	buffer := bufio.NewWriterSize(tickFile, 256*1024)
	encoder := json.NewEncoder(buffer)
	tickCount := 0
	var tickErr error
	s := &GameService{Tables: input.Tables}
	outcome, computeErr := s.computeFrozenBattleObserved(t.Context(), &input, nil, func(e *combatRoundEngine) {
		tickCount++
		if tickCount > maxTicks || tickErr != nil {
			return
		}
		var players []map[string]any
		for _, player := range e.players {
			var actors []map[string]any
			for _, actor := range player.actors {
				var skills []map[string]any
				var waiting []string
				current := ""
				if actor.current != nil {
					current = actor.current.id
				}
				for _, skill := range actor.skills {
					skills = append(skills, map[string]any{"id": skill.id, "state": skill.state, "cooldown": skill.cooldown})
				}
				for _, skill := range actor.waiting {
					waiting = append(waiting, skill.id)
				}
				actors = append(actors, map[string]any{"uid": actor.uid, "hp": actor.hp, "state": actor.state, "skills": skills, "waiting": waiting, "current": current})
			}
			players = append(players, map[string]any{"anger": player.anger, "actors": actors})
		}
		tickErr = encoder.Encode(map[string]any{"round": e.round, "time": e.time, "randomDraws": e.rng.Draws(), "players": players, "casts": e.diagnosticCasts})
	})
	flushErr, closeErr := buffer.Flush(), tickFile.Close()
	if tickErr != nil || flushErr != nil || closeErr != nil {
		t.Fatal("private tick write failed", tickErr, flushErr, closeErr)
	}
	after, err := json.Marshal(&input)
	if err != nil {
		t.Fatal("actual frozen input cannot be encoded after calculation")
	}
	unchanged := bytes.Equal(before, after)
	result := map[string]any{
		"fightId": id, "inputSHA256": inputSHA, "nativeTraceSHA256": nativeTraceSHA,
		"nativeAttackerWon": nativeWon, "nativeRound": nativeRound,
		"inputUnchanged": unchanged, "ticksTotal": tickCount, "ticksLimit": maxTicks,
		"ticksTruncated": tickCount > maxTicks, "computed": computeErr == nil && outcome != nil,
	}
	if computeErr != nil {
		result["computeError"] = computeErr.Error()
	}
	if outcome != nil {
		eventCount := len(outcome.Events)
		limited := *outcome
		if len(limited.Events) > maxEvents {
			limited.Events = limited.Events[:maxEvents]
		}
		if err := p6ArenaTakeoverWritePrivateJSON(filepath.Join(caseDir, "outcome.json"), &limited); err != nil {
			t.Fatal("write private outcome", err)
		}
		result["goWinner"] = outcome.Winner
		result["goRound"] = outcome.FinalRound
		result["goRandomDraws"] = outcome.RandomDraws
		result["goAuthoritative"] = outcome.Authoritative
		result["eventsTotal"] = eventCount
		result["eventsLimit"] = maxEvents
		result["eventsTruncated"] = eventCount > maxEvents
		result["matchesNative"] = (outcome.Winner == 0) == nativeWon && (outcome.Winner == 0 || outcome.Winner == 1) && outcome.FinalRound == nativeRound
	}
	if err := p6ArenaTakeoverWritePrivateJSON(filepath.Join(caseDir, "case-result.json"), result); err != nil {
		t.Fatal("write private case result", err)
	}
	if !unchanged {
		t.Error("actual frozen input changed during calculation")
	}
	if computeErr != nil || outcome == nil {
		t.Fatal("actual arena calculation failed; see private case result")
	}
	if (outcome.Winner != 0 && outcome.Winner != 1) || (outcome.Winner == 0) != nativeWon || outcome.FinalRound != nativeRound {
		t.Errorf("native won=%t round=%d; Go winner=%d round=%d", nativeWon, nativeRound, outcome.Winner, outcome.FinalRound)
	}
}

func p6ArenaTakeoverWritePrivateJSON(path string, value any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	writeErr := enc.Encode(value)
	closeErr := f.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}
