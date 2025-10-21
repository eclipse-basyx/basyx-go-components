/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

type DataSpecificationContent interface {
	GetPrefferedName() []LangStringPreferredNameTypeIec61360
	GetShortName() []LangStringShortNameTypeIec61360
	GetUnit() string
	GetUnitId() *Reference
	GetSourceOfDefinition() string
	GetSymbol() string
	GetDataType() DataTypeIec61360
	GetDefinition() []LangStringDefinitionTypeIec61360
	GetValueFormat() string
	GetValueList() *ValueList
	GetLevelType() *LevelType

	SetPrefferedName([]LangStringPreferredNameTypeIec61360)
	SetShortName([]LangStringShortNameTypeIec61360)
	SetUnit(string)
	SetUnitId(*Reference)
	SetSourceOfDefinition(string)
	SetSymbol(string)
	SetDataType(*DataTypeIec61360)
	SetDefinition([]LangStringDefinitionTypeIec61360)
	SetValueFormat(string)
	SetValueList(*ValueList)
	SetLevelType(*LevelType)
}
