package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	pb "hyjs-server/model/protocol"
)

// 快照只由服务端创建，与同一事务内的楼层奖励和扫荡记录一起提交。
type p7ReRushPowerSnapshot struct {
	PlayerID int64              `json:"playerId"`
	RunID    int64              `json:"runId"`
	Floor    int32              `json:"floor"`
	Power    int32              `json:"power"`
	Stage    db.P7RushStageRule `json:"stage"`
	Enemies  []byte             `json:"enemies"`
	Party    []byte             `json:"party"`
}

type p7ReRushSweepKey struct {
	run   int64
	floor int32
}

func p7ReRushPowerSource(runID int64) string {
	return constUtil.P7ReRushPowerSource + strconv.FormatInt(runID, 10)
}

func p7ReRushPowerSourceRun(source string) (int64, bool) {
	if !strings.HasPrefix(source, constUtil.P7ReRushPowerSource) {
		return 0, false
	}
	runID, err := strconv.ParseInt(strings.TrimPrefix(source, constUtil.P7ReRushPowerSource), 10, 64)
	return runID, err == nil && runID > 0 && source == p7ReRushPowerSource(runID)
}

func p7ReRushReadPowerProof(row db.P7ReRushPowerProof) (p7ReRushWin, error) {
	var snapshot p7ReRushPowerSnapshot
	var party pb.BattlePlayer
	var enemy pb.BattleMonster
	bad := errors.New("试炼战力扫荡证明损坏")
	if row.PlayerID <= 0 || row.RunID <= 0 || row.Floor < 1 || row.Floor > constUtil.P7ReRushLastFloor ||
		p6Digest(row.Snapshot) != row.SnapshotHash || json.Unmarshal(row.Snapshot, &snapshot) != nil ||
		snapshot.PlayerID != row.PlayerID || snapshot.RunID != row.RunID || snapshot.Floor != row.Floor ||
		snapshot.Stage.ID != fmt.Sprintf("re-rush:%d:3", row.Floor) || snapshot.Stage.TaskID != fmt.Sprintf("local_rush_%d", row.Floor) ||
		snapshot.Stage.Kind != constUtil.P7RushTrial || snapshot.Stage.Floor != row.Floor || snapshot.Stage.Star != 3 ||
		!snapshot.Stage.Enabled || snapshot.Stage.Source != constUtil.P7ReRushSource || snapshot.Stage.MinLevel < 1 ||
		math.IsNaN(snapshot.Stage.RecommendedPower) || math.IsInf(snapshot.Stage.RecommendedPower, 0) ||
		snapshot.Stage.RecommendedPower <= 0 || float64(snapshot.Power) < snapshot.Stage.RecommendedPower ||
		proto.Unmarshal(snapshot.Party, &party) != nil || party.GetPlayerId() != row.PlayerID || len(party.GetFormation()) == 0 ||
		proto.Unmarshal(snapshot.Enemies, &enemy) != nil || enemy.Inst == nil || len(enemy.GetFormation()) == 0 {
		return p7ReRushWin{}, bad
	}
	return p7ReRushWin{Floor: row.Floor, Star: 3, Fight: p7ReRushPowerSource(row.RunID), Hash: row.SnapshotHash}, nil
}

func p7ReRushCreatePowerProof(tx *gorm.DB, state *db.P7RushState, stage db.P7RushStageRule, party []byte, power int32) (p7ReRushWin, error) {
	snapshot := p7ReRushPowerSnapshot{PlayerID: state.PlayerID, RunID: state.RunID, Floor: stage.Floor, Power: power, Stage: stage, Enemies: stage.Enemies, Party: party}
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return p7ReRushWin{}, err
	}
	row := db.P7ReRushPowerProof{PlayerID: state.PlayerID, RunID: state.RunID, Floor: stage.Floor, Snapshot: raw, SnapshotHash: p6Digest(raw)}
	win, err := p7ReRushReadPowerProof(row)
	if err != nil {
		return p7ReRushWin{}, err
	}
	if err = tx.Create(&row).Error; err != nil {
		return p7ReRushWin{}, err
	}
	return win, nil
}

// 批量读取同轮扫荡引用的战力证明，避免逐层查询。
func p7ReRushPowerWinsForSweeps(tx *gorm.DB, playerID int64, sweeps []db.P7ReRushSweep) (map[p7ReRushSweepKey]p7ReRushWin, error) {
	runs, floors := []int64{}, []int32{}
	seenRuns, seenFloors := map[int64]bool{}, map[int32]bool{}
	for _, sweep := range sweeps {
		if strings.HasPrefix(sweep.SourceFight, constUtil.P7ReRushPowerSource) {
			run, ok := p7ReRushPowerSourceRun(sweep.SourceFight)
			if !ok || sweep.PlayerID != playerID || sweep.Star != 3 {
				return nil, errors.New("试炼战力扫荡来源无效")
			}
			if !seenRuns[run] {
				seenRuns[run] = true
				runs = append(runs, run)
			}
			if !seenFloors[sweep.Floor] {
				seenFloors[sweep.Floor] = true
				floors = append(floors, sweep.Floor)
			}
		}
	}
	wins := map[p7ReRushSweepKey]p7ReRushWin{}
	if len(runs) == 0 {
		return wins, nil
	}
	var proofs []db.P7ReRushPowerProof
	if err := tx.Where("player_id=? AND run_id IN ? AND floor IN ?", playerID, runs, floors).Find(&proofs).Error; err != nil {
		return nil, err
	}
	type key struct {
		run   int64
		floor int32
	}
	byKey := make(map[key]db.P7ReRushPowerProof, len(proofs))
	for _, row := range proofs {
		byKey[key{row.RunID, row.Floor}] = row
	}
	for _, sweep := range sweeps {
		run, ok := p7ReRushPowerSourceRun(sweep.SourceFight)
		if !ok {
			continue
		}
		row, found := byKey[key{run, sweep.Floor}]
		win, err := p7ReRushReadPowerProof(row)
		if !found || err != nil || win.Hash != sweep.SourceHash || win.Star != sweep.Star {
			return nil, errors.New("试炼战力扫荡资格来源冲突")
		}
		wins[p7ReRushSweepKey{sweep.RunID, sweep.Floor}] = win
	}
	return wins, nil
}

// 历史战力证明必须同时存在其来源轮的扫荡记录，重置后才可用于扫至最高。
func p7ReRushHistoricalPowerWins(tx *gorm.DB, playerID int64, from, to int32) (map[int32]p7ReRushWin, error) {
	var proofs []db.P7ReRushPowerProof
	if err := tx.Where("player_id=? AND floor BETWEEN ? AND ?", playerID, from, to).Find(&proofs).Error; err != nil {
		return nil, err
	}
	wins := map[int32]p7ReRushWin{}
	if len(proofs) == 0 {
		return wins, nil
	}
	runs := make([]int64, 0, len(proofs))
	for _, proof := range proofs {
		runs = append(runs, proof.RunID)
	}
	var sweeps []db.P7ReRushSweep
	if err := tx.Where("player_id=? AND run_id IN ? AND floor BETWEEN ? AND ?", playerID, runs, from, to).Find(&sweeps).Error; err != nil {
		return nil, err
	}
	type key struct {
		run   int64
		floor int32
	}
	byKey := make(map[key]db.P7ReRushSweep, len(sweeps))
	for _, sweep := range sweeps {
		byKey[key{sweep.RunID, sweep.Floor}] = sweep
	}
	for _, proof := range proofs {
		win, err := p7ReRushReadPowerProof(proof)
		sweep, found := byKey[key{proof.RunID, proof.Floor}]
		if err != nil || !found || sweep.SourceFight != win.Fight || sweep.SourceHash != win.Hash || sweep.Star != win.Star {
			return nil, errors.New("试炼历史战力扫荡资格来源冲突")
		}
		if wins[win.Floor].Star < win.Star {
			wins[win.Floor] = win
		}
	}
	return wins, nil
}
