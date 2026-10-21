package service

import (
	"bytes"
	"encoding/json"
	"reflect"

	"gorm.io/gorm"
	"hyjs-server/db"
	"hyjs-server/model"
	pb "hyjs-server/model/protocol"
)

// adminSavedIdentity 将实例与集合成员按稳定身份配对，保留未暴露到编辑器的存档字段。
func adminSavedIdentity(value any) string {
	fields, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range []string{"guid", "id", "missionId", "chapterId", "key", "formatPos", "pos"} {
		if v, exists := fields[key]; exists {
			encoded, _ := json.Marshal(v)
			return key + ":" + string(encoded)
		}
	}
	return ""
}

func adminMergeSavedValue(saved, before, after any) any {
	if reflect.DeepEqual(before, after) {
		return saved
	}
	if next, ok := after.(map[string]any); ok {
		old, _ := before.(map[string]any)
		out, _ := saved.(map[string]any)
		if out == nil {
			out = map[string]any{}
		}
		for key := range old {
			if _, exists := next[key]; !exists {
				delete(out, key)
			}
		}
		for key, value := range next {
			if previous, exists := old[key]; exists {
				if reflect.DeepEqual(previous, value) {
					continue
				}
				out[key] = adminMergeSavedValue(out[key], previous, value)
			} else {
				out[key] = value
			}
		}
		return out
	}
	if next, ok := after.([]any); ok {
		old, _ := before.([]any)
		original, _ := saved.([]any)
		byID := map[string]int{}
		for i, value := range old {
			if key := adminSavedIdentity(value); key != "" {
				byID[key] = i
			}
		}
		out := make([]any, 0, len(next))
		for i, value := range next {
			index := i
			if key := adminSavedIdentity(value); key != "" {
				var exists bool
				index, exists = byID[key]
				if !exists {
					out = append(out, value)
					continue
				}
			}
			if index < len(old) && index < len(original) {
				out = append(out, adminMergeSavedValue(original[index], old[index], value))
			} else {
				out = append(out, value)
			}
		}
		return out
	}
	return after
}

// adminMergeSavedJSON 只叠加本次编辑差量，不丢失原存档中的未知字段或未编辑集合成员。
func adminMergeSavedJSON(saved string, before, after any) (string, error) {
	oldJSON, err := json.Marshal(before)
	if err != nil {
		return "", err
	}
	newJSON, err := json.Marshal(after)
	if err != nil {
		return "", err
	}
	if bytes.Equal(oldJSON, newJSON) {
		return saved, nil
	}
	values := make([]any, 3)
	for i, raw := range [][]byte{[]byte(saved), oldJSON, newJSON} {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&values[i]); err != nil {
			return "", err
		}
	}
	out, err := json.Marshal(adminMergeSavedValue(values[0], values[1], values[2]))
	return string(out), err
}

// adminSavePlayerChanges 在已持有角色行锁的运营事务中保存差量，避免触发游戏领奖和消费事件。
func adminSavePlayerChanges(tx *gorm.DB, row *db.Profile, beforePlayer *pb.Player, beforeGame *model.GameState, player *pb.Player, game *model.GameState) error {
	playerJSON, err := adminMergeSavedJSON(*row.Player, beforePlayer, player)
	if err != nil {
		return err
	}
	gameJSON, err := adminMergeSavedJSON(*row.Game, beforeGame, game)
	if err != nil {
		return err
	}
	updates := map[string]any{"player": playerJSON, "game": gameJSON, "created_at": row.CreatedAt}
	if err := tx.Model(row).Updates(updates).Error; err != nil {
		return err
	}
	row.Player, row.Game = &playerJSON, &gameJSON
	return syncGlobalAccountRole(tx, row)
}
