package descriptors

func applyCursorLimit[T any](items []T, limit int32, cursorFn func(T) string) ([]T, string) {
	if limit < 0 {
		return items, ""
	}
	intLimit := int(limit)
	if len(items) > intLimit {
		nextCursor := cursorFn(items[intLimit])
		return items[:intLimit], nextCursor
	}
	return items, ""
}
