/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

type DataSpecificationContent struct {
	PreferredName      []LangStringTextType `json:"preferredName"`
	ShortName          []LangStringTextType `json:"shortName,omitempty"`
	Unit               string               `json:"unit,omitempty"`
	UnitId             string               `json:"unitId,omitempty"`
	SourceOfDefinition string               `json:"sourceOfDefinition,omitempty"`
	Symbol             string               `json:"symbol,omitempty"`
	DataType           string               `json:"dataType,omitempty"`
	Definition         string               `json:"definition,omitempty"`
	ValueFormat        string               `json:"valueFormat,omitempty"`
	ValueList          string               `json:"valueList,omitempty"`
	Value              string               `json:"value,omitempty"`
	LevelType          string               `json:"levelType,omitempty"`
}
