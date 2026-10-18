package db

type PageInfo struct {
	Curr  int64 `json:"curr"`
	Size  int64 `json:"size"`
	Total int64 `json:"total"`
}

func Page(maxLimit int, page PageInfo) (limit, offset int, curr int64) {
	limit = 20
	curr = 1

	if page.Size > 0 {
		limit = int(page.Size)
		if limit > maxLimit {
			limit = maxLimit
		}
	}
	if page.Curr > 0 {
		curr = page.Curr
		offset = int(page.Curr-1) * limit
	}

	return
}
