/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

import "encoding/json"

type DataSpecificationIec61360 struct {
	ModelType string `json:"modelType" validate:"regexp=^DataSpecificationIec61360$"`

	PreferredName []LangStringPreferredNameTypeIec61360 `json:"preferredName"`

	ShortName []LangStringShortNameTypeIec61360 `json:"shortName,omitempty"`

	Unit string `json:"unit,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	UnitId *Reference `json:"unitId,omitempty"`

	SourceOfDefinition string `json:"sourceOfDefinition,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	Symbol string `json:"symbol,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	DataType DataTypeIec61360 `json:"dataType,omitempty"`

	Definition []LangStringDefinitionTypeIec61360 `json:"definition,omitempty"`

	ValueFormat string `json:"valueFormat,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	ValueList *ValueList `json:"valueList,omitempty"`

	Value string `json:"value,omitempty" validate:"regexp=^([\\\\x09\\\\x0a\\\\x0d\\\\x20-\\\\ud7ff\\\\ue000-\\\\ufffd]|\\\\ud800[\\\\udc00-\\\\udfff]|[\\\\ud801-\\\\udbfe][\\\\udc00-\\\\udfff]|\\\\udbff[\\\\udc00-\\\\udfff])*$"`

	LevelType *LevelType `json:"levelType,omitempty"`
}

func (d DataSpecificationIec61360) GetPrefferedName() []LangStringPreferredNameTypeIec61360 {
	return d.PreferredName
}

func (d DataSpecificationIec61360) GetShortName() []LangStringShortNameTypeIec61360 {
	return d.ShortName
}

func (d DataSpecificationIec61360) GetUnit() string {
	return d.Unit
}

func (d DataSpecificationIec61360) GetUnitId() *Reference {
	return d.UnitId
}

func (d DataSpecificationIec61360) GetSourceOfDefinition() string {
	return d.SourceOfDefinition
}

func (d DataSpecificationIec61360) GetSymbol() string {
	return d.Symbol
}

func (d DataSpecificationIec61360) GetDataType() DataTypeIec61360 {
	return d.DataType
}

func (d DataSpecificationIec61360) GetDefinition() []LangStringDefinitionTypeIec61360 {
	return d.Definition
}

func (d DataSpecificationIec61360) GetValueFormat() string {
	return d.ValueFormat
}

func (d DataSpecificationIec61360) GetValueList() *ValueList {
	return d.ValueList
}

func (d DataSpecificationIec61360) GetLevelType() *LevelType {
	return d.LevelType
}

func (d *DataSpecificationIec61360) SetPrefferedName(preferredName []LangStringPreferredNameTypeIec61360) {
	d.PreferredName = preferredName
}

func (d *DataSpecificationIec61360) SetShortName(shortName []LangStringShortNameTypeIec61360) {
	d.ShortName = shortName
}

func (d *DataSpecificationIec61360) SetUnit(unit string) {
	d.Unit = unit
}

func (d *DataSpecificationIec61360) SetUnitId(unitId *Reference) {
	d.UnitId = unitId
}

func (d *DataSpecificationIec61360) SetSourceOfDefinition(sourceOfDefinition string) {
	d.SourceOfDefinition = sourceOfDefinition
}

func (d *DataSpecificationIec61360) SetSymbol(symbol string) {
	d.Symbol = symbol
}

func (d *DataSpecificationIec61360) SetDataType(dataType *DataTypeIec61360) {
	d.DataType = *dataType
}

func (d *DataSpecificationIec61360) SetDefinition(definition []LangStringDefinitionTypeIec61360) {
	d.Definition = definition
}

func (d *DataSpecificationIec61360) SetValueFormat(valueFormat string) {
	d.ValueFormat = valueFormat
}

func (d *DataSpecificationIec61360) SetValueList(valueList *ValueList) {
	if valueList != nil {
		d.ValueList = valueList
	} else {
		d.ValueList = &ValueList{}
	}
}

func (d *DataSpecificationIec61360) SetLevelType(levelType *LevelType) {
	d.LevelType = levelType
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
	if obj.UnitId != nil {
		if err := AssertReferenceRequired(*obj.UnitId); err != nil {
			return err
		}
	}
	for _, el := range obj.Definition {
		if err := AssertLangStringDefinitionTypeIec61360Required(el); err != nil {
			return err
		}
	}
	if obj.ValueList != nil {
		if err := AssertValueListRequired(*obj.ValueList); err != nil {
			return err
		}
	}
	if obj.LevelType != nil {
		if err := AssertLevelTypeRequired(*obj.LevelType); err != nil {
			return err
		}
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
	if obj.UnitId != nil {
		if err := AssertReferenceConstraints(*obj.UnitId); err != nil {
			return err
		}
	}
	for _, el := range obj.Definition {
		if err := AssertLangStringDefinitionTypeIec61360Constraints(el); err != nil {
			return err
		}
	}
	if obj.ValueList != nil {
		if err := AssertValueListConstraints(*obj.ValueList); err != nil {
			return err
		}
	}
	if obj.LevelType != nil {
		if err := AssertLevelTypeConstraints(*obj.LevelType); err != nil {
			return err
		}
	}
	return nil
}

func (d *DataSpecificationIec61360) UnmarshalJSON(data []byte) error {
	aux := &struct {
		ModelType          string                                `json:"modelType"`
		PreferredName      []LangStringPreferredNameTypeIec61360 `json:"preferredName"`
		ShortName          []LangStringShortNameTypeIec61360     `json:"shortName,omitempty"`
		Unit               string                                `json:"unit,omitempty"`
		UnitId             *Reference                            `json:"unitId,omitempty"`
		SourceOfDefinition string                                `json:"sourceOfDefinition,omitempty"`
		Symbol             string                                `json:"symbol,omitempty"`
		DataType           DataTypeIec61360                      `json:"dataType,omitempty"`
		Definition         []LangStringDefinitionTypeIec61360    `json:"definition,omitempty"`
		ValueFormat        string                                `json:"valueFormat,omitempty"`
		ValueList          *ValueList                            `json:"valueList,omitempty"`
		Value              string                                `json:"value,omitempty"`
		LevelType          *LevelType                            `json:"levelType,omitempty"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Set default model type if not provided
	if aux.ModelType == "" {
		d.ModelType = "DataSpecificationIec61360"
	} else {
		d.ModelType = aux.ModelType
	}

	// Copy all other fields
	d.PreferredName = aux.PreferredName
	d.ShortName = aux.ShortName
	d.Unit = aux.Unit
	d.UnitId = aux.UnitId
	d.SourceOfDefinition = aux.SourceOfDefinition
	d.Symbol = aux.Symbol
	d.DataType = aux.DataType
	d.Definition = aux.Definition
	d.ValueFormat = aux.ValueFormat
	d.ValueList = aux.ValueList
	d.Value = aux.Value
	d.LevelType = aux.LevelType

	return nil
}
