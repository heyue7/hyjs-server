package model

type ExampleListRequest struct {
	FirstName string `json:"firstName"`
}

type ExampleListResponse struct {
	LastName string `json:"lastName"`
}
