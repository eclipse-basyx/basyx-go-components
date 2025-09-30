/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

type DataSpecificationIec61360 struct {
	ModelType string `json:"modelType" validate:"regexp=^DataSpecificationIec61360$"`

	PreferredName []LangStringPreferredNameTypeIec61360 `json:"preferredName"`

	ShortName []LangStringShortNameTypeIec61360 `json:"shortName,omitempty"`

	Unit string `json:"unit,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	UnitId Reference `json:"unitId,omitempty"`

	SourceOfDefinition string `json:"sourceOfDefinition,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	Symbol string `json:"symbol,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	DataType DataTypeIec61360 `json:"dataType,omitempty"`

	Definition []LangStringDefinitionTypeIec61360 `json:"definition,omitempty"`

	ValueFormat string `json:"valueFormat,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	ValueList ValueList `json:"valueList,omitempty"`

	Value string `json:"value,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	LevelType LevelType `json:"levelType,omitempty"`
}

// AssertDataSpecificationIec61360Required checks if the required fields are not zero-ed
func AssertDataSpecificationIec61360Required(obj DataSpecificationIec61360) error {
	elements := map[string]interface{}{
		"modelType":     obj.ModelType,
		"preferredName": obj.PreferredName,
	}
	for name, el := range elements {
		if isZero := IsZeroValue(el); isZero {
			return &RequiredError{Field: name}
		}
	}

	for _, el := range obj.PreferredName {
		if err := AssertLangStringPreferredNameTypeIec61360Required(el); err != nil {
			return err
		}
	}
	for _, el := range obj.ShortName {
		if err := AssertLangStringShortNameTypeIec61360Required(el); err != nil {
			return err
		}
	}
	if err := AssertReferenceRequired(obj.UnitId); err != nil {
		return err
	}
	for _, el := range obj.Definition {
		if err := AssertLangStringDefinitionTypeIec61360Required(el); err != nil {
			return err
		}
	}
	if err := AssertValueListRequired(obj.ValueList); err != nil {
		return err
	}
	if err := AssertLevelTypeRequired(obj.LevelType); err != nil {
		return err
	}
	return nil
}

// AssertDataSpecificationIec61360Constraints checks if the values respects the defined constraints
func AssertDataSpecificationIec61360Constraints(obj DataSpecificationIec61360) error {
	for _, el := range obj.PreferredName {
		if err := AssertLangStringPreferredNameTypeIec61360Constraints(el); err != nil {
			return err
		}
	}
	for _, el := range obj.ShortName {
		if err := AssertLangStringShortNameTypeIec61360Constraints(el); err != nil {
			return err
		}
	}
	if err := AssertReferenceConstraints(obj.UnitId); err != nil {
		return err
	}
	for _, el := range obj.Definition {
		if err := AssertLangStringDefinitionTypeIec61360Constraints(el); err != nil {
			return err
		}
	}
	if err := AssertValueListConstraints(obj.ValueList); err != nil {
		return err
	}
	if err := AssertLevelTypeConstraints(obj.LevelType); err != nil {
		return err
	}
	return nil
}
