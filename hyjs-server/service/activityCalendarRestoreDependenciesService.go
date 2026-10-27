package service

import (
	"sort"
	"strings"

	"gorm.io/gorm"
	"hyjs-server/db"
)

// ActivityCalendarRestoreDependency is indexed inventory evidence, without private payloads.
// A future writer must lock and hash actual dependency rows; these counts cannot authorize CAS.
type ActivityCalendarRestoreDependency struct {
	Table             string `json:"table"`
	Rows              int64  `json:"rows"`
	RequiredTreatment string `json:"requiredTreatment"`
}

func activityCalendarRestoreDependencies(tx *gorm.DB, plan *ActivityCalendarRestorePlan) error {
	keys := map[string]map[string]bool{}
	for _, window := range plan.Windows {
		domain, key, ok := strings.Cut(window.ConsumerID, ":")
		if !ok || key == "" {
			continue
		}
		if keys[domain] == nil {
			keys[domain] = map[string]bool{}
		}
		keys[domain][key] = true
	}
	list := func(domain string) []string {
		out := []string{}
		for key := range keys[domain] {
			out = append(out, key)
		}
		sort.Strings(out)
		return out
	}
	count := func(model any, column string, ids []string, treatment string) error {
		if len(ids) == 0 {
			return nil
		}
		statement := &gorm.Statement{DB: tx}
		if err := statement.Parse(model); err != nil {
			return err
		}
		table := statement.Schema.Table
		if !adminActivitySchemaHasTable(tx, model) {
			plan.Blockers = append(plan.Blockers, "dependency schema missing: "+table)
			return nil
		}
		var total int64
		for start := 0; start < len(ids); start += 400 {
			var n int64
			if err := tx.Model(model).Where(column+" IN ?", ids[start:min(start+400, len(ids))]).Count(&n).Error; err != nil {
				return err
			}
			total += n
		}
		plan.Dependencies = append(plan.Dependencies, ActivityCalendarRestoreDependency{table, total, treatment})
		return nil
	}
	periods := list("period")
	checks := []struct {
		model     any
		column    string
		ids       []string
		treatment string
	}{
		{&db.PeriodInstance{}, "id", periods, "patch live/Frozen dates together; preserve goals, rewards and RewardSourceID"},
		{&db.PeriodProgress{}, "instance_id", periods, "patch Snapshot dates; preserve Value/LastDay/RewardID and verify daily/payment replay"},
		{&db.PeriodSettlement{}, "instance_id", periods, "keep historical ranking settlement immutable; reopening requires explicit compatibility"},
		{&db.GameplayPool{}, "id", list("pool"), "patch only dates; preserve prices and reward children"},
		{&db.TurntableState{}, "pool_id", list("pool"), "preserve pity/score/cooldown/free-day; verify no extra free draw"},
		{&db.TurntablePersonal{}, "pool_id", list("pool"), "preserve private stock/jackpot/settlement history"},
		{&db.TurntableGlobal{}, "pool_id", list("pool"), "preserve shared stock/final ranks; never silently reopen settled pool"},
		{&db.TurntableDrawReceipt{}, "pool_id", list("pool"), "preserve every request/result byte-for-byte"},
		{&db.P4CommerceRule{}, "id", append(list("commerce"), list("shop-gate")...), "patch live/Frozen dates; preserve Rules and prices"},
		{&db.P4CommerceState{}, "rule_id", append(list("commerce"), list("shop-gate")...), "preserve purchases and frozen quotes; resolve old quote expiry"},
		{&db.P4CommerceStock{}, "rule_id", append(list("commerce"), list("shop-gate")...), "preserve shared used stock; never reset purchases"},
	}
	for _, check := range checks {
		if err := count(check.model, check.column, check.ids, check.treatment); err != nil {
			return err
		}
	}
	if len(periods) > 0 && adminActivitySchemaHasTable(tx, &db.PeriodInstance{}) {
		sources := map[string]bool{}
		for start := 0; start < len(periods); start += 400 {
			var configs []db.PeriodInstance
			if err := tx.Select("id", "reward_source_id", "goal", "milestones", "rewards").Where("id IN ?", periods[start:min(start+400, len(periods))]).Find(&configs).Error; err != nil {
				return err
			}
			for _, cfg := range configs {
				for _, source := range p1PeriodSources(cfg) {
					sources[source] = true
				}
				// Ranking settlements use the period ID rather than RewardSourceID.
				sources["rank:"+cfg.ID] = true
			}
		}
		ids := []string{}
		for source := range sources {
			ids = append(ids, source)
		}
		sort.Strings(ids)
		if err := count(&db.RewardDelivery{}, "source_key", ids, "preserve claimed deliveries; map unclaimed expiry by exact activity/tier/rank source without recreating rewards"); err != nil {
			return err
		}
	}
	for domain := range keys {
		if domain != "period" && domain != "pool" && domain != "commerce" && domain != "shop-gate" && domain != "character-plan" && domain != "opening" {
			plan.Blockers = append(plan.Blockers, "unmapped consumer domain: "+domain+"; refuse apply")
		}
	}
	return nil
}
