package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"hyjs-server/constUtil"
	"hyjs-server/db"
	"hyjs-server/model"
	"sort"
)

type turntablePityProgress struct {
	Key        string `json:"key"`
	Since      int32  `json:"since"`
	Paused     bool   `json:"paused,omitempty"`
	Recovering bool   `json:"recovering,omitempty"`
}
type turntablePityDue struct {
	ID        int32
	Remaining int32
}

func turntablePityProbability(r model.TurntablePrizePityRule, since int32) int64 {
	n := since + 1
	if n >= r.HardPity {
		return 1000000
	}
	if n < r.SoftStart {
		return int64(r.BasePPM)
	}
	x := int64(n - r.SoftStart)
	d := int64(r.HardPity - 1 - r.SoftStart)
	return int64(r.BasePPM) + int64(r.SoftMaxPPM-r.BasePPM)*x*x/(d*d)
}

// turntablePitySchedule 提前安排临近期限，拒绝已经无法逐抽兑现的存档。
func turntablePitySchedule(due []turntablePityDue) (int32, error) {
	sort.Slice(due, func(i, j int) bool {
		if due[i].Remaining == due[j].Remaining {
			return due[i].ID < due[j].ID
		}
		return due[i].Remaining < due[j].Remaining
	})
	forced := false
	for i, d := range due {
		if d.Remaining < int32(i+1) {
			return -1, errors.New(constUtil.TurntablePityScheduleInvalid)
		}
		if d.Remaining == int32(i+1) {
			forced = true
		}
	}
	if forced {
		return due[0].ID, nil
	}
	return -1, nil
}

func turntablePityRules(raw string, jackpot int32) ([]model.TurntablePrizePityRule, error) {
	var rules []model.TurntablePrizePityRule
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &rules); err != nil {
			return nil, err
		}
	}
	if !model.TurntablePrizePityValid(rules, int64(jackpot)) {
		return nil, errors.New(constUtil.TurntablePityRulesInvalid)
	}
	return rules, nil
}

// turntablePityDraw 仅处理开启递增的新规则；调用方在原抽奖事务内提交全部进度。
func turntablePityDraw(rules []model.TurntablePrizePityRule, progress map[int32]turntablePityProgress, rewards []db.GameplayPoolReward, stock map[int32]int32, pool int64, jackpot int32, dice int64) (int32, error) {
	enabled := map[int32]model.TurntablePrizePityRule{}
	active := map[int32]bool{}
	due := []turntablePityDue{}
	fallback := false
	for _, r := range rules {
		if r.Enabled {
			enabled[r.PrizeID] = r
		}
	}
	for _, r := range rewards {
		available := r.Stock == 0 || stock[r.ID] < r.Stock
		if _, ok := enabled[r.ID]; !ok && available {
			fallback = true
		}
		if _, ok := enabled[r.ID]; ok && available {
			active[r.ID] = true
		}
	}
	if !fallback {
		return -1, errors.New(constUtil.TurntablePityFallbackInsufficient)
	}
	if _, ok := enabled[9]; ok && pool > 0 {
		active[9] = true
	}
	for id, r := range enabled {
		key := fmt.Sprintf("%d:diamond", id)
		if id != 9 {
			for _, reward := range rewards {
				if reward.ID == id {
					key = fmt.Sprintf("%d:%s", id, reward.ResourceID)
				}
			}
		}
		st := progress[id]
		if st.Key != key {
			st = turntablePityProgress{Key: key}
		}
		if st.Since < 0 || st.Since >= r.HardPity {
			return -1, errors.New(constUtil.TurntablePityProgressInvalid)
		}
		if !active[id] {
			st.Paused = true
		} else if st.Paused {
			st.Paused = false
			st.Recovering = true
		}
		progress[id] = st
		if active[id] {
			due = append(due, turntablePityDue{id, r.HardPity - st.Since})
		}
	}
	chosen, err := turntablePitySchedule(due)
	recoveringConflict := false
	if err != nil {
		stable := []turntablePityDue{}
		recovered := false
		for _, d := range due {
			if progress[d.ID].Recovering {
				recovered = true
			} else {
				stable = append(stable, d)
			}
		}
		if _, stableErr := turntablePitySchedule(stable); !recovered || stableErr != nil {
			return -1, err
		}
		// 恢复奖位插入后只延期本抽无法同时兑现的计数，保留累计进度。
		chosen = due[0].ID
		recoveringConflict = true
	}
	if chosen < 0 {
		// 排序保证同一骰子与配置得到稳定的分桶结果。
		sort.Slice(due, func(i, j int) bool { return due[i].ID < due[j].ID })
		edge := int64(0)
		for _, d := range due {
			edge += turntablePityProbability(enabled[d.ID], progress[d.ID].Since)
			if dice < edge {
				chosen = d.ID
				break
			}
		}
		if chosen < 0 {
			if _, ok := enabled[9]; !ok && pool > 0 {
				edge += int64(jackpot)
				if dice < edge {
					chosen = 9
				}
			}
		}
	}
	for id := range active {
		st := progress[id]
		if id == chosen {
			st.Since = 0
			st.Recovering = false
		} else if recoveringConflict && st.Since+1 >= enabled[id].HardPity {
			st.Recovering = true
		} else {
			st.Since++
		}
		progress[id] = st
	}
	return chosen, nil
}

// turntablePityPauseDisabled 关闭期间保留累计数，标记由合法规则关闭引起的恢复。
func turntablePityPauseDisabled(rules []model.TurntablePrizePityRule, progress map[int32]turntablePityProgress) {
	enabled := map[int32]bool{}
	for _, r := range rules {
		enabled[r.PrizeID] = r.Enabled
	}
	for id, st := range progress {
		if !enabled[id] {
			st.Paused = true
			progress[id] = st
		}
	}
}
