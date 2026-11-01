package service

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/constUtil"
	"hyjs-server/db"
)

// 每段选项在首次展示时冻结，已展示的旧固定选项沿原规则完成。
type p7ReRushAdditionOptions struct {
	Version string `json:"version"`
	Three   string `json:"three"`
	Six     string `json:"six"`
	Nine    string `json:"nine"`
}

func p7ReRushLegacyOptions() p7ReRushAdditionOptions {
	return p7ReRushAdditionOptions{Version: constUtil.P7ReRushLegacyAddition, Three: "perhp,0.03", Six: "peratt,0.06", Nine: "perpdef,0.09"}
}

func p7ReRushAdditionKey(option string, cost int32) (string, error) {
	parts := strings.Split(option, ",")
	amount := map[int32]string{3: "0.03", 6: "0.06", 9: "0.09"}[cost]
	if len(parts) != 2 || amount == "" || parts[1] != amount {
		return "", errors.New("试炼加成星数或数值无效")
	}
	switch parts[0] {
	case "perhp", "peratt", "perpdef", "permdef", "hit", "dodge", "cri", "antiCri", "damageAdd", "damageReduce":
		return parts[0], nil
	default:
		return "", errors.New("试炼加成属性无效")
	}
}

func p7ReRushValidateOptions(options p7ReRushAdditionOptions) error {
	if options.Version != constUtil.P7ReRushLegacyAddition && options.Version != constUtil.P7ReRushRandomAddition {
		return errors.New("试炼加成选项版本无效")
	}
	if options.Version == constUtil.P7ReRushLegacyAddition && options != p7ReRushLegacyOptions() {
		return errors.New("旧试炼加成选项损坏")
	}
	seen := map[string]bool{}
	for _, item := range []struct {
		option string
		cost   int32
	}{{options.Three, 3}, {options.Six, 6}, {options.Nine, 9}} {
		key, err := p7ReRushAdditionKey(item.option, item.cost)
		if err != nil || seen[key] {
			return errors.New("试炼加成选项重复或损坏")
		}
		seen[key] = true
	}
	return nil
}

func p7ReRushGenerateOptions(source io.Reader) (p7ReRushAdditionOptions, error) {
	keys := []string{"perhp", "peratt", "perpdef", "permdef", "hit", "dodge", "cri", "antiCri", "damageAdd", "damageReduce"}
	selected := make([]string, 3)
	for i := range selected {
		index, err := rand.Int(source, big.NewInt(int64(len(keys)-i)))
		if err != nil {
			return p7ReRushAdditionOptions{}, err
		}
		j := i + int(index.Int64())
		keys[i], keys[j] = keys[j], keys[i]
		selected[i] = keys[i]
	}
	return p7ReRushAdditionOptions{Version: constUtil.P7ReRushRandomAddition, Three: selected[0] + ",0.03", Six: selected[1] + ",0.06", Nine: selected[2] + ",0.09"}, nil
}

func p7ReRushReadOptions(row *db.P7ReRushSection) (p7ReRushAdditionOptions, error) {
	var options p7ReRushAdditionOptions
	if row == nil || row.OptionsJSON == "" || json.Unmarshal([]byte(row.OptionsJSON), &options) != nil {
		return options, errors.New("试炼加成选项尚未冻结")
	}
	return options, p7ReRushValidateOptions(options)
}

func p7ReRushFreezeOptions(tx *gorm.DB, state *db.P7RushState, row *db.P7ReRushSection, opening bool) (p7ReRushAdditionOptions, error) {
	if row.OptionsJSON != "" {
		return p7ReRushReadOptions(row)
	}
	var options p7ReRushAdditionOptions
	var err error
	if state.AdditionOfferSection == row.Section {
		// 升级前已经发出的选项必须与客户端当前界面一致。
		options = p7ReRushLegacyOptions()
	} else if opening {
		options, err = p7ReRushGenerateOptions(rand.Reader)
		if err != nil {
			return options, err
		}
	} else {
		return options, errors.New("请先打开本段试炼加成选项")
	}
	if err = p7ReRushValidateOptions(options); err != nil {
		return options, err
	}
	raw, err := json.Marshal(options)
	if err != nil {
		return options, err
	}
	result := tx.Model(&db.P7ReRushSection{}).Where("player_id=? AND run_id=? AND section=? AND options_json=?", row.PlayerID, row.RunID, row.Section, "").Update("options_json", string(raw))
	if result.Error != nil {
		return options, result.Error
	}
	if result.RowsAffected != 1 {
		return options, errors.New("试炼加成选项已变化")
	}
	row.OptionsJSON = string(raw)
	return options, nil
}

func (options p7ReRushAdditionOptions) selected(cost int32) string {
	switch cost {
	case 3:
		return options.Three
	case 6:
		return options.Six
	case 9:
		return options.Nine
	default:
		return ""
	}
}
