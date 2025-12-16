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

import "encoding/json"

// MultiLanguagePropertyValue represents the Value-Only serialization of a MultiLanguageProperty.
// According to spec: Serialized as array of JSON objects with language and localized string.
type MultiLanguagePropertyValue []LangStringTextType

// MarshalValueOnly serializes MultiLanguagePropertyValue in Value-Only format
// Serializes as array of objects with language code as key and text as value
func (m MultiLanguagePropertyValue) MarshalValueOnly() ([]byte, error) {
	// Convert to compact format: [{"en-us":"text"}, {"de":"text"}]
	result := make([]map[string]string, len(m))
	for i, item := range m {
		result[i] = map[string]string{item.Language: item.Text}
	}
	return json.Marshal(result)
}

// MarshalJSON implements custom JSON marshaling for MultiLanguagePropertyValue
func (m MultiLanguagePropertyValue) MarshalJSON() ([]byte, error) {
	return m.MarshalValueOnly()
}

// AssertMultiLanguagePropertyValueRequired checks if the required fields are not zero-ed
func AssertMultiLanguagePropertyValueRequired(obj MultiLanguagePropertyValue) error {
	for _, el := range obj {
		if err := AssertLangStringTextTypeRequired(el); err != nil {
			return err
		}
	}
	return nil
}

// AssertMultiLanguagePropertyValueConstraints checks if the values respects the defined constraints
func AssertMultiLanguagePropertyValueConstraints(obj MultiLanguagePropertyValue) error {
	for _, el := range obj {
		if err := AssertLangStringTextTypeConstraints(el); err != nil {
			return err
		}
	}
	return nil
}
