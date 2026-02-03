/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */
//nolint:all
package model

import (
	"encoding/json"

	"github.com/FriedJannik/aas-go-sdk/types"
)

// MultiLanguagePropertyValue represents the Value-Only serialization of a MultiLanguageProperty.
// According to spec: Serialized as array of JSON objects with language and localized string.
type MultiLanguagePropertyValue []map[string]string

// MarshalValueOnly serializes MultiLanguagePropertyValue in Value-Only format
// Serializes as array of objects with language code as key and text as value
func (m MultiLanguagePropertyValue) MarshalValueOnly() ([]byte, error) {
	// The data is already in the correct format: [{"en-us":"text"}, {"de":"text"}]
	// Each map element already has language code as key and text as value
	return json.Marshal([]map[string]string(m))
}

// MarshalJSON implements custom JSON marshaling for MultiLanguagePropertyValue
func (m MultiLanguagePropertyValue) MarshalJSON() ([]byte, error) {
	return m.MarshalValueOnly()
}

// GetModelType returns the model type name for MultiLanguageProperty
func (m MultiLanguagePropertyValue) GetModelType() types.ModelType {
	return types.ModelTypeMultiLanguageProperty
}
