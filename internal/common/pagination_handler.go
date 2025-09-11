package common

type PagedResult struct {
	Cursor string `json:"cursor"`
	Result any    `json:"result"`
}
