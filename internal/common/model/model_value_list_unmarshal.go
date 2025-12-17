package model

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"
)

// UnmarshalJSON implements custom unmarshaling for ValueList that can handle both
// a direct array of ValueReferencePair and an object with valueReferencePair field
func (vl *ValueList) UnmarshalJSON(data []byte) error {
	// First try to unmarshal as array directly
	var pairs []*ValueReferencePair
	var json = jsoniter.ConfigCompatibleWithStandardLibrary
	if err := json.Unmarshal(data, &pairs); err == nil {
		vl.ValueReferencePairs = pairs
		return nil
	}

	// If that fails, try to unmarshal as object with valueReferencePair field
	var obj struct {
		ValueReferencePairs []*ValueReferencePair `json:"valueReferencePairs"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return fmt.Errorf("value list must be either array of value reference pairs or object with valueReferencePairs field: %w", err)
	}

	vl.ValueReferencePairs = obj.ValueReferencePairs
	return nil
}
