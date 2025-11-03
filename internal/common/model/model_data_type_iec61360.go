/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package model

import (
	"fmt"
)

// DataTypeIec61360 type of DataTypeIec61360
type DataTypeIec61360 string

// List of DataTypeIec61360
//
//nolint:all
const (
	DATATYPEIEC61360_BLOB                DataTypeIec61360 = "Blob"
	DATATYPEIEC61360_BOOLEAN             DataTypeIec61360 = "Boolean"
	DATATYPEIEC61360_DATE                DataTypeIec61360 = "Date"
	DATATYPEIEC61360_FILE                DataTypeIec61360 = "File"
	DATATYPEIEC61360_HTML                DataTypeIec61360 = "Html"
	DATATYPEIEC61360_INTEGER_COUNT       DataTypeIec61360 = "IntegerCount"
	DATATYPEIEC61360_INTEGER_CURRENCY    DataTypeIec61360 = "IntegerCurrency"
	DATATYPEIEC61360_INTEGER_MEASURE     DataTypeIec61360 = "IntegerMeasure"
	DATATYPEIEC61360_IRDI                DataTypeIec61360 = "Irdi"
	DATATYPEIEC61360_IRI                 DataTypeIec61360 = "Iri"
	DATATYPEIEC61360_RATIONAL            DataTypeIec61360 = "Rational"
	DATATYPEIEC61360_RATIONAL_MEASURE    DataTypeIec61360 = "RationalMeasure"
	DATATYPEIEC61360_REAL_COUNT          DataTypeIec61360 = "RealCount"
	DATATYPEIEC61360_REAL_CURRENCY       DataTypeIec61360 = "RealCurrency"
	DATATYPEIEC61360_REAL_MEASURE        DataTypeIec61360 = "RealMeasure"
	DATATYPEIEC61360_STRING              DataTypeIec61360 = "String"
	DATATYPEIEC61360_STRING_TRANSLATABLE DataTypeIec61360 = "StringTranslatable"
	DATATYPEIEC61360_TIME                DataTypeIec61360 = "Time"
	DATATYPEIEC61360_TIMESTAMP           DataTypeIec61360 = "Timestamp"
)

// AllowedDataTypeIec61360EnumValues is all the allowed values of DataTypeIec61360 enum
var AllowedDataTypeIec61360EnumValues = []DataTypeIec61360{
	"Blob",
	"Boolean",
	"Date",
	"File",
	"Html",
	"IntegerCount",
	"IntegerCurrency",
	"IntegerMeasure",
	"Irdi",
	"Iri",
	"Rational",
	"RationalMeasure",
	"RealCount",
	"RealCurrency",
	"RealMeasure",
	"String",
	"StringTranslatable",
	"Time",
	"Timestamp",
}

// validDataTypeIec61360EnumValue provides a map of DataTypeIec61360s for fast verification of use input
var validDataTypeIec61360EnumValues = map[DataTypeIec61360]struct{}{
	"Blob":               {},
	"Boolean":            {},
	"Date":               {},
	"File":               {},
	"Html":               {},
	"IntegerCount":       {},
	"IntegerCurrency":    {},
	"IntegerMeasure":     {},
	"Irdi":               {},
	"Iri":                {},
	"Rational":           {},
	"RationalMeasure":    {},
	"RealCount":          {},
	"RealCurrency":       {},
	"RealMeasure":        {},
	"String":             {},
	"StringTranslatable": {},
	"Time":               {},
	"Timestamp":          {},
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v DataTypeIec61360) IsValid() bool {
	_, ok := validDataTypeIec61360EnumValues[v]
	return ok
}

// NewDataTypeIec61360FromValue returns a pointer to a valid DataTypeIec61360
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewDataTypeIec61360FromValue(v string) (DataTypeIec61360, error) {
	ev := DataTypeIec61360(v)
	if ev.IsValid() {
		return ev, nil
	}

	return "", fmt.Errorf("invalid value '%v' for DataTypeIec61360: valid values are %v", v, AllowedDataTypeIec61360EnumValues)
}

// AssertDataTypeIec61360Required checks if the required fields are not zero-ed
//
//nolint:all
func AssertDataTypeIec61360Required(obj DataTypeIec61360) error {
	return nil
}

// AssertDataTypeIec61360Constraints checks if the values respects the defined constraints
//
//nolint:all
func AssertDataTypeIec61360Constraints(obj DataTypeIec61360) error {
	return nil
}
