package model

// AdminItemDetailRequest identifies one original catalogue item.
type AdminItemDetailRequest struct {
	ID string `json:"id"` // ID is an exact original catalogue identity, never a file path.
}

// AdminItemDetailField is one readable fact recovered from an original table.
type AdminItemDetailField struct {
	Label string `json:"label"` // Label describes the value in operator-facing Chinese.
	Value string `json:"value"` // Value preserves source precision and explicitly marks unknown data.
}

// AdminItemDetailSection groups related original properties without exposing raw table JSON.
type AdminItemDetailSection struct {
	Title  string                 `json:"title"`  // Title is the human-readable section name.
	Fields []AdminItemDetailField `json:"fields"` // Fields contains only interpreted, source-backed facts.
}

// AdminItemDetailResponse combines the existing item presentation with readable source facts.
type AdminItemDetailResponse struct {
	Item     AdminItemDTO             `json:"item"`     // Item uses the existing catalogue name, quality and original art.
	Sections []AdminItemDetailSection `json:"sections"` // Sections never invent missing attributes, growth or rewards.
}
