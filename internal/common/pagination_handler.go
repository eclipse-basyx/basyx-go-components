//nolint:revive
package common

// PagedResult represents a paginated response containing a cursor for the next page
type PagedResult struct {
	Cursor string `json:"cursor"`
	Result any    `json:"result"`
}
