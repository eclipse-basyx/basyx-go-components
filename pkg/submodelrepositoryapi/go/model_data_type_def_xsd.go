/*
 * DotAAS Part 2 | HTTP/REST | Submodel Repository Service Specification
 *
 * The entire Submodel Repository Service Specification as part of the [Specification of the Asset Administration Shell: Part 2](http://industrialdigitaltwin.org/en/content-hub).   Publisher: Industrial Digital Twin Association (IDTA) 2023
 *
 * API version: V3.0.3_SSP-001
 * Contact: info@idtwin.org
 */

package openapi

import (
	"fmt"
)

type DataTypeDefXsd string

// List of DataTypeDefXsd
const (
	DATATYPEDEFXSD_XS_ANY_URI              DataTypeDefXsd = "xs:anyURI"
	DATATYPEDEFXSD_XS_BASE64_BINARY        DataTypeDefXsd = "xs:base64Binary"
	DATATYPEDEFXSD_XS_BOOLEAN              DataTypeDefXsd = "xs:boolean"
	DATATYPEDEFXSD_XS_BYTE                 DataTypeDefXsd = "xs:byte"
	DATATYPEDEFXSD_XS_DATE                 DataTypeDefXsd = "xs:date"
	DATATYPEDEFXSD_XS_DATE_TIME            DataTypeDefXsd = "xs:dateTime"
	DATATYPEDEFXSD_XS_DECIMAL              DataTypeDefXsd = "xs:decimal"
	DATATYPEDEFXSD_XS_DOUBLE               DataTypeDefXsd = "xs:double"
	DATATYPEDEFXSD_XS_DURATION             DataTypeDefXsd = "xs:duration"
	DATATYPEDEFXSD_XS_FLOAT                DataTypeDefXsd = "xs:float"
	DATATYPEDEFXSD_XS_G_DAY                DataTypeDefXsd = "xs:gDay"
	DATATYPEDEFXSD_XS_G_MONTH              DataTypeDefXsd = "xs:gMonth"
	DATATYPEDEFXSD_XS_G_MONTH_DAY          DataTypeDefXsd = "xs:gMonthDay"
	DATATYPEDEFXSD_XS_G_YEAR               DataTypeDefXsd = "xs:gYear"
	DATATYPEDEFXSD_XS_G_YEAR_MONTH         DataTypeDefXsd = "xs:gYearMonth"
	DATATYPEDEFXSD_XS_HEX_BINARY           DataTypeDefXsd = "xs:hexBinary"
	DATATYPEDEFXSD_XS_INT                  DataTypeDefXsd = "xs:int"
	DATATYPEDEFXSD_XS_INTEGER              DataTypeDefXsd = "xs:integer"
	DATATYPEDEFXSD_XS_LONG                 DataTypeDefXsd = "xs:long"
	DATATYPEDEFXSD_XS_NEGATIVE_INTEGER     DataTypeDefXsd = "xs:negativeInteger"
	DATATYPEDEFXSD_XS_NON_NEGATIVE_INTEGER DataTypeDefXsd = "xs:nonNegativeInteger"
	DATATYPEDEFXSD_XS_NON_POSITIVE_INTEGER DataTypeDefXsd = "xs:nonPositiveInteger"
	DATATYPEDEFXSD_XS_POSITIVE_INTEGER     DataTypeDefXsd = "xs:positiveInteger"
	DATATYPEDEFXSD_XS_SHORT                DataTypeDefXsd = "xs:short"
	DATATYPEDEFXSD_XS_STRING               DataTypeDefXsd = "xs:string"
	DATATYPEDEFXSD_XS_TIME                 DataTypeDefXsd = "xs:time"
	DATATYPEDEFXSD_XS_UNSIGNED_BYTE        DataTypeDefXsd = "xs:unsignedByte"
	DATATYPEDEFXSD_XS_UNSIGNED_INT         DataTypeDefXsd = "xs:unsignedInt"
	DATATYPEDEFXSD_XS_UNSIGNED_LONG        DataTypeDefXsd = "xs:unsignedLong"
	DATATYPEDEFXSD_XS_UNSIGNED_SHORT       DataTypeDefXsd = "xs:unsignedShort"
)

// AllowedDataTypeDefXsdEnumValues is all the allowed values of DataTypeDefXsd enum
var AllowedDataTypeDefXsdEnumValues = []DataTypeDefXsd{
	"xs:anyURI",
	"xs:base64Binary",
	"xs:boolean",
	"xs:byte",
	"xs:date",
	"xs:dateTime",
	"xs:decimal",
	"xs:double",
	"xs:duration",
	"xs:float",
	"xs:gDay",
	"xs:gMonth",
	"xs:gMonthDay",
	"xs:gYear",
	"xs:gYearMonth",
	"xs:hexBinary",
	"xs:int",
	"xs:integer",
	"xs:long",
	"xs:negativeInteger",
	"xs:nonNegativeInteger",
	"xs:nonPositiveInteger",
	"xs:positiveInteger",
	"xs:short",
	"xs:string",
	"xs:time",
	"xs:unsignedByte",
	"xs:unsignedInt",
	"xs:unsignedLong",
	"xs:unsignedShort",
}

// validDataTypeDefXsdEnumValue provides a map of DataTypeDefXsds for fast verification of use input
var validDataTypeDefXsdEnumValues = map[DataTypeDefXsd]struct{}{
	"xs:anyURI":             {},
	"xs:base64Binary":       {},
	"xs:boolean":            {},
	"xs:byte":               {},
	"xs:date":               {},
	"xs:dateTime":           {},
	"xs:decimal":            {},
	"xs:double":             {},
	"xs:duration":           {},
	"xs:float":              {},
	"xs:gDay":               {},
	"xs:gMonth":             {},
	"xs:gMonthDay":          {},
	"xs:gYear":              {},
	"xs:gYearMonth":         {},
	"xs:hexBinary":          {},
	"xs:int":                {},
	"xs:integer":            {},
	"xs:long":               {},
	"xs:negativeInteger":    {},
	"xs:nonNegativeInteger": {},
	"xs:nonPositiveInteger": {},
	"xs:positiveInteger":    {},
	"xs:short":              {},
	"xs:string":             {},
	"xs:time":               {},
	"xs:unsignedByte":       {},
	"xs:unsignedInt":        {},
	"xs:unsignedLong":       {},
	"xs:unsignedShort":      {},
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v DataTypeDefXsd) IsValid() bool {
	_, ok := validDataTypeDefXsdEnumValues[v]
	return ok
}

// NewDataTypeDefXsdFromValue returns a pointer to a valid DataTypeDefXsd
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewDataTypeDefXsdFromValue(v string) (DataTypeDefXsd, error) {
	ev := DataTypeDefXsd(v)
	if ev.IsValid() {
		return ev, nil
	}

	return "", fmt.Errorf("invalid value '%v' for DataTypeDefXsd: valid values are %v", v, AllowedDataTypeDefXsdEnumValues)
}

// AssertDataTypeDefXsdRequired checks if the required fields are not zero-ed
func AssertDataTypeDefXsdRequired(obj DataTypeDefXsd) error {
	return nil
}

// AssertDataTypeDefXsdConstraints checks if the values respects the defined constraints
func AssertDataTypeDefXsdConstraints(obj DataTypeDefXsd) error {
	return nil
}
