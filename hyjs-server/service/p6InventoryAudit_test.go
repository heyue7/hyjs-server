package service

import (
	"encoding/json"
	"fmt"
	"hyjs-server/constUtil"
	"os"
	"strconv"
	"testing"
)

// Export the actual engine's structural admission for every original talent slot and
// SkillCast. Admission alone leaves native effects, content reachability and playback pending.
func TestP6FullInventoryFactoryAudit(t *testing.T) {
	path := os.Getenv("HYJS_P6_FACTORY_AUDIT_OUTPUT")
	if path == "" {
		t.Skip("explicit full-inventory structural export disabled")
	}
	s := p6OriginalTables(t)
	e := &combatRoundEngine{service: s}
	rows := []map[string]string{}
	for _, talent := range s.Tables["Talent"] {
		for index := 0; index < 4; index++ {
			key := fmt.Sprintf("Buff%d", index)
			if talent[key] == "" {
				continue
			}
			trigger := combatRoundTrigger{buff: talent[key], timer: int32(tableNumber(talent, key+"TriggerTimer")), selectStrategy: int32(tableNumber(talent, key+"Select")), condition: int32(tableNumber(talent, key+"TriggerCondition")), args: talent[key+"ConditionArgs"], chance: tableNumber(talent, key+"Percent") * .01, group: talent["Group"]}
			row := map[string]string{"table": "Talent", "id": talent["Id"], "slot": strconv.Itoa(index), "buff": trigger.buff, "status": "factory_accepted_native_pending"}
			if err := e.validateDynamicTrigger(trigger); err != nil {
				row["status"], row["error"] = "factory_rejected", err.Error()
			}
			if trigger.timer == 0 {
				row["status"] = "static_panel_owned_by_P5_P9_not_dynamic_test"
			}
			rows = append(rows, row)
		}
	}
	for _, cast := range s.Tables["SkillCast"] {
		row := map[string]string{"table": "SkillCast", "id": cast["SkillCastId"], "skill": cast["SkillId"], "status": "factory_accepted_native_pending"}
		if err := e.validateCastBuffs(cast); err != nil {
			row["status"], row["error"] = "factory_rejected", err.Error()
		}
		rows = append(rows, row)
	}
	if len(s.Tables["Talent"]) != 780 || len(s.Tables["SkillCast"]) != 2470 {
		t.Fatal("original APK inventory counts drifted")
	}
	raw, err := json.MarshalIndent(map[string]any{"engineVersion": constUtil.P6EngineVersion, "nativeCompared": false, "authoritative": false, "source": "original 10056262.apk 3.0.0.42620 tables; current engine structural validation only", "rows": rows}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, append(raw, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("exported %d original structural admission rows", len(rows))
}
