/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// AbstractLangString struct
type AbstractLangString struct {
	Language string `json:"language" validate:"regexp=^(([a-zA-Z]{2,3}(-[a-zA-Z]{3}(-[a-zA-Z]{3}){0,2})?|[a-zA-Z]{4}|[a-zA-Z]{5,8})(-[a-zA-Z]{4})?(-([a-zA-Z]{2}|[0-9]{3}))?(-(([a-zA-Z0-9]){5,8}|[0-9]([a-zA-Z0-9]){3}))*(-[0-9A-WY-Za-wy-z](-([a-zA-Z0-9]){2,8})+)*(-[xX](-([a-zA-Z0-9]){1,8})+)?|[xX](-([a-zA-Z0-9]){1,8})+|((en-GB-oed|i-ami|i-bnn|i-default|i-enochian|i-hak|i-klingon|i-lux|i-mingo|i-navajo|i-pwn|i-tao|i-tay|i-tsu|sgn-BE-FR|sgn-BE-NL|sgn-CH-DE)|(art-lojban|cel-gaulish|no-bok|no-nyn|zh-guoyu|zh-hakka|zh-min|zh-min-nan|zh-xiang)))$"`

	Text string `json:"text" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`
}

// AssertAbstractLangStringRequired checks if the required fields are not zero-ed
func AssertAbstractLangStringRequired(obj AbstractLangString) error {
	elements := map[string]interface{}{
		"language": obj.Language,
		"text":     obj.Text,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	return nil
}

// AssertAbstractLangStringConstraints checks if the values respects the defined constraints
//
//nolint:all
func AssertAbstractLangStringConstraints(obj AbstractLangString) error {
	return nil
}
