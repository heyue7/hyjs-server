package model

type LocalPlayActivityView struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Status      string `json:"status"`
	CanOpen     bool   `json:"canOpen"`
	CanRefresh  bool   `json:"canRefresh"`
	Description string `json:"description"`
	StartsAt    int64  `json:"startsAt"`
	EndsAt      int64  `json:"endsAt"`
}

type LocalPlayActivityResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type LocalPlayActivitiesView struct {
	Activities []LocalPlayActivityView   `json:"activities"`
	Results    []LocalPlayActivityResult `json:"results"`
	TestOnly   bool                      `json:"testOnly"`
}
