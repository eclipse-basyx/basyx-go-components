package asyncbulk

import (
	"fmt"
	"strconv"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

// ExpandAtomicFailures expands a root failure into per-item failures for an
// atomically rolled back bulk request. Each returned item links to one incoming
// request item by index.
func ExpandAtomicFailures(itemIdentifiers []string, rootFailure ItemFailure) []ItemFailure {
	itemCount := len(itemIdentifiers)
	if itemCount == 0 {
		return []ItemFailure{rootFailure}
	}

	if rootFailure.Index < 0 || rootFailure.Index >= itemCount {
		rootFailure.Index = 0
	}
	if rootFailure.Identifier == "" {
		rootFailure.Identifier = itemIdentifiers[rootFailure.Index]
	}

	failures := make([]ItemFailure, 0, itemCount)
	for idx, identifier := range itemIdentifiers {
		if idx == rootFailure.Index {
			failures = append(failures, rootFailure)
			continue
		}

		statusCode := rootFailure.StatusCode
		if statusCode == 0 {
			statusCode = 400
		}

		failures = append(failures, ItemFailure{
			Index:      idx,
			Identifier: identifier,
			StatusCode: statusCode,
			Message: fmt.Sprintf(
				"operation rolled back due to atomic failure at index %d: %s",
				rootFailure.Index,
				rootFailure.Message,
			),
		})
	}

	return failures
}

// ToMessages converts per-item failures into Message objects for API result payloads.
func ToMessages(failures []ItemFailure) []model.Message {
	if len(failures) == 0 {
		return []model.Message{}
	}

	timestamp := time.Now().UTC().Format(time.RFC3339)
	messages := make([]model.Message, 0, len(failures))
	for _, failure := range failures {
		statusCode := failure.StatusCode
		if statusCode == 0 {
			statusCode = 400
		}

		text := fmt.Sprintf("item[%d]: %s", failure.Index, failure.Message)
		if failure.Identifier != "" {
			text = fmt.Sprintf("item[%d] (%s): %s", failure.Index, failure.Identifier, failure.Message)
		}

		messages = append(messages, model.Message{
			Code:          strconv.Itoa(statusCode),
			CorrelationID: fmt.Sprintf("bulk-item-%d", failure.Index),
			MessageType:   "Error",
			Text:          text,
			Timestamp:     timestamp,
		})
	}

	return messages
}
