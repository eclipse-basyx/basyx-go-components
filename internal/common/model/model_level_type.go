/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

// LevelType type of LevelType
type LevelType struct {
	Min bool `json:"min"`

	Nom bool `json:"nom"`

	Typ bool `json:"typ"`

	Max bool `json:"max"`
}

// AssertLevelTypeRequired checks if the required fields are not zero-ed
func AssertLevelTypeRequired(obj LevelType) error {
	elements := map[string]interface{}{
		"min": obj.Min,
		"nom": obj.Nom,
		"typ": obj.Typ,
		"max": obj.Max,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	return nil
}

// AssertLevelTypeConstraints checks if the values respects the defined constraints
//
//nolint:all
func AssertLevelTypeConstraints(obj LevelType) error {
	return nil
}
