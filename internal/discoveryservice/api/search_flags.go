// Package api contains discovery service API handlers and helpers.
package api

import "context"

type ctxKey string

const assetLinksAlreadyConstrainedKey ctxKey = "discovery.asset_links_already_constrained"

// WithAssetLinksAlreadyConstrained marks a discovery search context where
// asset-link conditions are already embedded into the active query filter.
func WithAssetLinksAlreadyConstrained(ctx context.Context) context.Context {
	return context.WithValue(ctx, assetLinksAlreadyConstrainedKey, true)
}

// AssetLinksAlreadyConstrainedFromContext indicates whether discovery lookup
// filters already include the asset-link matching semantics.
func AssetLinksAlreadyConstrainedFromContext(ctx context.Context) bool {
	constrained, _ := ctx.Value(assetLinksAlreadyConstrainedKey).(bool)
	return constrained
}
