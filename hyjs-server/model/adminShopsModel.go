package model

// AdminShopDefinition describes an original shop and its editable fields.
type AdminShopDefinition struct {
	ID             string   `json:"id"`             // Original consumer key.
	Label          string   `json:"label"`          // Chinese shop name.
	Notes          string   `json:"notes"`          // Snapshot and consumer restrictions.
	Fields         []string `json:"fields"`         // Supported product controls.
	SettingsFields []string `json:"settingsFields"` // Supported settings controls.
	CanCreate      bool     `json:"canCreate"`      // Whether this shop supports adding products.
}

// AdminShopProduct projects actual shop configuration without raw JSON.
// AdminShopQuantityTier is one weighted quantity with its complete purchase cost.
type AdminShopQuantityTier struct {
	Amount int64               `json:"amount"` // Amount is the resource count selected for one slot.
	Weight int64               `json:"weight"` // Weight is a positive relative weight within this product.
	Cost   AdminResourceAmount `json:"cost"`   // Cost is the complete cost for the selected quantity.
}

// AdminShopRefreshSettings controls the existing refresh engine.
type AdminShopRefreshSettings struct {
	Enabled             bool                 `json:"enabled"`             // Enabled allows explicit refresh operations.
	FreeCap             int64                `json:"freeCap"`             // FreeCap is the maximum free refresh balance.
	FreeRecoverySeconds int64                `json:"freeRecoverySeconds"` // FreeRecoverySeconds is the free balance recovery interval.
	PaidCap             int64                `json:"paidCap"`             // PaidCap is the maximum paid refreshes in Period.
	Cost                AdminResourceAmount  `json:"cost"`                // Cost is the paid refresh price.
	Item                *AdminResourceAmount `json:"item,omitempty"`      // Item optionally replaces the paid resource.
	Period              string               `json:"period"`              // Period is a read-only day/week projection.
}

type AdminShopProduct struct {
	QuantityTiers      []AdminShopQuantityTier `json:"quantityTiers,omitempty"` // QuantityTiers is empty for the legacy fixed quote.
	ID                 string                  `json:"id"`                      // Stable product identity.
	Name               string                  `json:"name"`                    // Readable original resource name.
	Rewards            []AdminResourceAmount   `json:"rewards"`                 // Resources per purchase.
	Cost               AdminResourceAmount     `json:"cost"`                    // Actual purchase currency and amount.
	DailyLimit         int64                   `json:"dailyLimit"`              // Daily purchase count.
	AccountWeeklyLimit int64                   `json:"accountWeeklyLimit"`      // 同账号跨角色区服每周份数，0不限。
	TotalLimit         int64                   `json:"totalLimit"`              // Period cap, zero unlimited.
	Stock              int64                   `json:"stock"`                   // Shared stock, minus one unlimited.
	MinLevel           int64                   `json:"minLevel"`                // Player level requirement.
	MinVip             int64                   `json:"minVip"`                  // VIP requirement.
	Priority           int64                   `json:"priority"`                // Original ordering.
	Weight             int64                   `json:"weight"`                  // Refresh selection weight.
	DiscountBps        int64                   `json:"discountBps"`             // Discount in basis points.
	ContributionCost   int64                   `json:"contributionCost"`        // Guild contribution cost.
	GuildLevel         int64                   `json:"guildLevel"`              // Required guild level.
	Day                string                  `json:"day"`                     // Guild battle offer date.
	BuyCondition       int64                   `json:"buyCondition"`            // 0 all members, 1 deputy/master, 2 master.
	RefreshMillis      int64                   `json:"refreshMillis"`           // Guild battle stock recovery interval.
	Enabled            bool                    `json:"enabled"`                 // Whether the stored product is enabled.
	Source             string                  `json:"source"`                  // Configuration provenance.
	MaxRank            int64                   `json:"maxRank"`                 // Maximum eligible popularity rank.
}

// AdminShopSettings configures the original shop window and its source.
type AdminShopSettings struct {
	Refresh    *AdminShopRefreshSettings `json:"refresh,omitempty"` // Refresh contains supported refresh policy only.
	Unlimited  bool                      `json:"unlimited"`         // true=不限营业时间，false=按配置日期营业；商品资格和限购继续生效。
	Enabled    bool                      `json:"enabled"`           // Whether the current source is enabled.
	Slots      int64                     `json:"slots"`             // Refresh slot count.
	StartAt    int64                     `json:"startAt"`           // Opening time in milliseconds.
	EndAt      int64                     `json:"endAt"`             // Closing time in milliseconds.
	ClaimEndAt int64                     `json:"claimEndAt"`        // Final claim time.
	Source     string                    `json:"source"`            // Operator configuration note.
	PeriodID   string                    `json:"periodId"`          // Stable catalogue identity; edits keep existing purchase counters.
	Frozen     bool                      `json:"frozen"`            // Historical field retained for compatibility; ordinary shop editors remain editable.
}

// AdminShopResponse is a versioned shop configuration.
type AdminShopResponse struct {
	ID        string             `json:"id"`                  // Original consumer key.
	Items     []AdminShopProduct `json:"items"`               // Actual stored products.
	Version   string             `json:"version"`             // Optimistic concurrency digest.
	Settings  AdminShopSettings  `json:"settings"`            // Editable source settings.
	Resources []AdminItemDTO     `json:"resources,omitempty"` // Dedicated original resources where needed.
}

// AdminShopProductSaveRequest changes one actual shop product.
type AdminShopProductSaveRequest struct {
	AdminShopProduct
	Version string `json:"version" binding:"required"` // Last-read shop version.
}

// AdminShopProductDeleteRequest removes a future product, retaining purchase history.
type AdminShopProductDeleteRequest struct {
	Version string `json:"version" binding:"required"` // Last-read shop version.
}

// AdminShopSettingsSaveRequest updates settings or creates a closed successor period.
type AdminShopSettingsSaveRequest struct {
	AdminShopSettings
	Version   string `json:"version" binding:"required"` // Last-read shop version.
	NewPeriod bool   `json:"newPeriod"`                  // Legacy request field retained for compatibility; edits use the current catalogue.
}
