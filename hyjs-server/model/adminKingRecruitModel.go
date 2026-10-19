package model

// AdminKingRecruitResource is an original recruitable character and its world.
type AdminKingRecruitResource struct {
	AdminItemDTO
	Space      int64  `json:"space"`      // Space uses original Hero.space: 1=SAO, 2=ALO, 3=GGO.
	SpaceLabel string `json:"spaceLabel"` // SpaceLabel is the original world label.
}

// AdminKingRecruitResourcesResponse lists valid costType3 candidates only.
type AdminKingRecruitResourcesResponse struct {
	Items []AdminKingRecruitResource `json:"items"` // Items excludes main characters, unsupported qualities and other worlds.
}
