/*******************************************************************************
* Copyright (C) 2025 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software, and to
* permit persons to whom the Software is furnished to do so, subject to
* the following conditions:
*
* The above copyright notice and this permission notice shall be
* included in all copies or substantial portions of the Software.
*
* THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
* EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF
* MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
* NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT HOLDERS BE
* LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER IN AN ACTION
* OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN CONNECTION
* WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.
*
* SPDX-License-Identifier: MIT
******************************************************************************/
// Author: Martin Stemmer ( Fraunhofer IESE )

package auth

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"time"
)

type ACL struct {
	// ACCESS corresponds to the JSON schema field "ACCESS".
	ACCESS ACLACCESS `json:"ACCESS" yaml:"ACCESS" mapstructure:"ACCESS"`

	// ATTRIBUTES corresponds to the JSON schema field "ATTRIBUTES".
	ATTRIBUTES []AttributeItem `json:"ATTRIBUTES,omitempty" yaml:"ATTRIBUTES,omitempty" mapstructure:"ATTRIBUTES,omitempty"`

	// RIGHTS corresponds to the JSON schema field "RIGHTS".
	RIGHTS []RightsEnum `json:"RIGHTS" yaml:"RIGHTS" mapstructure:"RIGHTS"`

	// USEATTRIBUTES corresponds to the JSON schema field "USEATTRIBUTES".
	USEATTRIBUTES *string `json:"USEATTRIBUTES,omitempty" yaml:"USEATTRIBUTES,omitempty" mapstructure:"USEATTRIBUTES,omitempty"`
}

type ACLACCESS string

const ACLACCESSALLOW ACLACCESS = "ALLOW"
const ACLACCESSDISABLED ACLACCESS = "DISABLED"

var enumValues_ACLACCESS = []interface{}{
	"ALLOW",
	"DISABLED",
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *ACLACCESS) UnmarshalJSON(value []byte) error {
	var v string
	if err := json.Unmarshal(value, &v); err != nil {
		return err
	}
	var ok bool
	for _, expected := range enumValues_ACLACCESS {
		if reflect.DeepEqual(v, expected) {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("invalid value (expected one of %#v): %#v", enumValues_ACLACCESS, v)
	}
	*j = ACLACCESS(v)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *ACL) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["ACCESS"]; raw != nil && !ok {
		return fmt.Errorf("field ACCESS in ACL: required")
	}
	if _, ok := raw["RIGHTS"]; raw != nil && !ok {
		return fmt.Errorf("field RIGHTS in ACL: required")
	}
	type Plain ACL
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = ACL(plain)
	return nil
}

type AccessPermissionRule struct {
	// ACL corresponds to the JSON schema field "ACL".
	ACL *ACL `json:"ACL,omitempty" yaml:"ACL,omitempty" mapstructure:"ACL,omitempty"`

	// FILTER corresponds to the JSON schema field "FILTER".
	FILTER *AccessPermissionRuleFILTER `json:"FILTER,omitempty" yaml:"FILTER,omitempty" mapstructure:"FILTER,omitempty"`

	// FORMULA corresponds to the JSON schema field "FORMULA".
	FORMULA *LogicalExpression `json:"FORMULA,omitempty" yaml:"FORMULA,omitempty" mapstructure:"FORMULA,omitempty"`

	// OBJECTS corresponds to the JSON schema field "OBJECTS".
	OBJECTS []ObjectItem `json:"OBJECTS,omitempty" yaml:"OBJECTS,omitempty" mapstructure:"OBJECTS,omitempty"`

	// USEACL corresponds to the JSON schema field "USEACL".
	USEACL *string `json:"USEACL,omitempty" yaml:"USEACL,omitempty" mapstructure:"USEACL,omitempty"`

	// USEFORMULA corresponds to the JSON schema field "USEFORMULA".
	USEFORMULA *string `json:"USEFORMULA,omitempty" yaml:"USEFORMULA,omitempty" mapstructure:"USEFORMULA,omitempty"`

	// USEOBJECTS corresponds to the JSON schema field "USEOBJECTS".
	USEOBJECTS []string `json:"USEOBJECTS,omitempty" yaml:"USEOBJECTS,omitempty" mapstructure:"USEOBJECTS,omitempty"`
}

func (j *AccessPermissionRule) UnmarshalJSON(value []byte) error {

	type Plain AccessPermissionRule
	var plain Plain

	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}

	isStrSet := func(p *string) bool {
		return p != nil && strings.TrimSpace(*p) != ""
	}

	hasACL := plain.ACL != nil
	hasUseACL := isStrSet(plain.USEACL)
	hasFormula := plain.FORMULA != nil
	hasUseFormula := isStrSet(plain.USEFORMULA)

	if hasACL == hasUseACL {
		if hasACL {
			return fmt.Errorf("AccessPermissionRule: only one of ACL or USEACL may be defined, not both")
		}
		return fmt.Errorf("AccessPermissionRule: exactly one of ACL or USEACL must be defined")
	}

	if hasFormula == hasUseFormula {
		if hasFormula {
			return fmt.Errorf("AccessPermissionRule: only one of FORMULA or USEFORMULA may be defined, not both")
		}
		return fmt.Errorf("AccessPermissionRule: exactly one of FORMULA or USEFORMULA must be defined")
	}

	*j = AccessPermissionRule(plain)
	return nil
}

type AccessPermissionRuleFILTER struct {
	// CONDITION corresponds to the JSON schema field "CONDITION".
	CONDITION *LogicalExpression `json:"CONDITION,omitempty" yaml:"CONDITION,omitempty" mapstructure:"CONDITION,omitempty"`

	// FRAGMENT corresponds to the JSON schema field "FRAGMENT".
	FRAGMENT *string `json:"FRAGMENT,omitempty" yaml:"FRAGMENT,omitempty" mapstructure:"FRAGMENT,omitempty"`

	// USEFORMULA corresponds to the JSON schema field "USEFORMULA".
	USEFORMULA *string `json:"USEFORMULA,omitempty" yaml:"USEFORMULA,omitempty" mapstructure:"USEFORMULA,omitempty"`
}

// This schema contains the AAS Access Rule Language.
type AccessRuleModelSchemaJson struct {
	// AllAccessPermissionRules corresponds to the JSON schema field
	// "AllAccessPermissionRules".
	AllAccessPermissionRules AccessRuleModelSchemaJsonAllAccessPermissionRules `json:"AllAccessPermissionRules" yaml:"AllAccessPermissionRules" mapstructure:"AllAccessPermissionRules"`
}

type AccessRuleModelSchemaJsonAllAccessPermissionRules struct {
	// DEFACLS corresponds to the JSON schema field "DEFACLS".
	DEFACLS []AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFACLSElem `json:"DEFACLS,omitempty" yaml:"DEFACLS,omitempty" mapstructure:"DEFACLS,omitempty"`

	// DEFATTRIBUTES corresponds to the JSON schema field "DEFATTRIBUTES".
	DEFATTRIBUTES []AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFATTRIBUTESElem `json:"DEFATTRIBUTES,omitempty" yaml:"DEFATTRIBUTES,omitempty" mapstructure:"DEFATTRIBUTES,omitempty"`

	// DEFFORMULAS corresponds to the JSON schema field "DEFFORMULAS".
	DEFFORMULAS []AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFFORMULASElem `json:"DEFFORMULAS,omitempty" yaml:"DEFFORMULAS,omitempty" mapstructure:"DEFFORMULAS,omitempty"`

	// DEFOBJECTS corresponds to the JSON schema field "DEFOBJECTS".
	DEFOBJECTS []AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem `json:"DEFOBJECTS,omitempty" yaml:"DEFOBJECTS,omitempty" mapstructure:"DEFOBJECTS,omitempty"`

	// Rules corresponds to the JSON schema field "rules".
	Rules []AccessPermissionRule `json:"rules" yaml:"rules" mapstructure:"rules"`
}

type AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFACLSElem struct {
	// Acl corresponds to the JSON schema field "acl".
	Acl ACL `json:"acl" yaml:"acl" mapstructure:"acl"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name" yaml:"name" mapstructure:"name"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFACLSElem) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["acl"]; raw != nil && !ok {
		return fmt.Errorf("field acl in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFACLSElem: required")
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFACLSElem: required")
	}
	type Plain AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFACLSElem
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFACLSElem(plain)
	return nil
}

type AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFATTRIBUTESElem struct {
	// Attributes corresponds to the JSON schema field "attributes".
	Attributes []AttributeItem `json:"attributes" yaml:"attributes" mapstructure:"attributes"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name" yaml:"name" mapstructure:"name"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFATTRIBUTESElem) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["attributes"]; raw != nil && !ok {
		return fmt.Errorf("field attributes in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFATTRIBUTESElem: required")
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFATTRIBUTESElem: required")
	}
	type Plain AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFATTRIBUTESElem
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFATTRIBUTESElem(plain)
	return nil
}

type AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFFORMULASElem struct {
	// Formula corresponds to the JSON schema field "formula".
	Formula LogicalExpression `json:"formula" yaml:"formula" mapstructure:"formula"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name" yaml:"name" mapstructure:"name"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFFORMULASElem) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["formula"]; raw != nil && !ok {
		return fmt.Errorf("field formula in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFFORMULASElem: required")
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFFORMULASElem: required")
	}
	type Plain AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFFORMULASElem
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFFORMULASElem(plain)
	return nil
}

type AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem struct {
	// USEOBJECTS corresponds to the JSON schema field "USEOBJECTS".
	USEOBJECTS []string `json:"USEOBJECTS,omitempty" yaml:"USEOBJECTS,omitempty" mapstructure:"USEOBJECTS,omitempty"`

	// Name corresponds to the JSON schema field "name".
	Name string `json:"name" yaml:"name" mapstructure:"name"`

	// Objects corresponds to the JSON schema field "objects".
	Objects []ObjectItem `json:"objects,omitempty" yaml:"objects,omitempty" mapstructure:"objects,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["name"]; raw != nil && !ok {
		return fmt.Errorf("field name in AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem: required")
	}
	type Plain AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJsonAllAccessPermissionRulesDEFOBJECTSElem(plain)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *AccessRuleModelSchemaJsonAllAccessPermissionRules) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["rules"]; raw != nil && !ok {
		return fmt.Errorf("field rules in AccessRuleModelSchemaJsonAllAccessPermissionRules: required")
	}
	type Plain AccessRuleModelSchemaJsonAllAccessPermissionRules
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJsonAllAccessPermissionRules(plain)
	return nil
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *AccessRuleModelSchemaJson) UnmarshalJSON(value []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(value, &raw); err != nil {
		return err
	}
	if _, ok := raw["AllAccessPermissionRules"]; raw != nil && !ok {
		return fmt.Errorf("field AllAccessPermissionRules in AccessRuleModelSchemaJson: required")
	}
	type Plain AccessRuleModelSchemaJson
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	*j = AccessRuleModelSchemaJson(plain)
	return nil
}

type AttributeValue interface{}

type ATTRTYPE string

const (
	ATTRCLAIM     ATTRTYPE = "CLAIM"
	ATTRGLOBAL    ATTRTYPE = "GLOBAL"
	ATTRREFERENCE ATTRTYPE = "REFERENCE"
)

type AttributeItem struct {
	Kind  ATTRTYPE
	Value string
}

var allowedAttrKeys = map[string]struct{}{
	string(ATTRCLAIM):     {},
	string(ATTRGLOBAL):    {},
	string(ATTRREFERENCE): {},
}

var allowedGlobalVals = map[string]struct{}{
	"LOCALNOW":  {},
	"UTCNOW":    {},
	"CLIENTNOW": {},
	"ANONYMOUS": {},
}

func (a *AttributeItem) UnmarshalJSON(b []byte) error {
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	if len(raw) != 1 {
		return fmt.Errorf("AttributeItem: expected exactly one key, got %d", len(raw))
	}

	for k, v := range raw {
		if _, ok := allowedAttrKeys[k]; !ok {
			return fmt.Errorf("AttributeItem: invalid key %q (allowed: CLAIM, GLOBAL, REFERENCE)", k)
		}

		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("AttributeItem: value for %q must be a string", k)
		}

		if k == string(ATTRGLOBAL) {
			if _, ok := allowedGlobalVals[s]; !ok {
				return fmt.Errorf("AttributeItem: GLOBAL must be one of LOCALNOW, UTCNOW, CLIENTNOW, ANONYMOUS (got %q)", s)
			}
		}

		a.Kind = ATTRTYPE(k)
		a.Value = s
		break
	}
	return nil
}

type ComparisonItems []Value

type DateTimeLiteralPattern time.Time

type HexLiteralPattern string

func (j *HexLiteralPattern) UnmarshalJSON(value []byte) error {
	type Plain HexLiteralPattern
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	if matched, _ := regexp.MatchString(`^16#[0-9A-F]+$`, string(plain)); !matched {
		return fmt.Errorf("field %s pattern match: must match %s", "", `^16#[0-9A-F]+$`)
	}
	*j = HexLiteralPattern(plain)
	return nil
}

type LogicalExpression struct {
	// And corresponds to the JSON schema field "$and".
	And []LogicalExpression `json:"$and,omitempty" yaml:"$and,omitempty" mapstructure:"$and,omitempty"`

	// Boolean corresponds to the JSON schema field "$boolean".
	Boolean *bool `json:"$boolean,omitempty" yaml:"$boolean,omitempty" mapstructure:"$boolean,omitempty"`

	// Contains corresponds to the JSON schema field "$contains".
	Contains StringItems `json:"$contains,omitempty" yaml:"$contains,omitempty" mapstructure:"$contains,omitempty"`

	// EndsWith corresponds to the JSON schema field "$ends-with".
	EndsWith StringItems `json:"$ends-with,omitempty" yaml:"$ends-with,omitempty" mapstructure:"$ends-with,omitempty"`

	// Eq corresponds to the JSON schema field "$eq".
	Eq ComparisonItems `json:"$eq,omitempty" yaml:"$eq,omitempty" mapstructure:"$eq,omitempty"`

	// Ge corresponds to the JSON schema field "$ge".
	Ge ComparisonItems `json:"$ge,omitempty" yaml:"$ge,omitempty" mapstructure:"$ge,omitempty"`

	// Gt corresponds to the JSON schema field "$gt".
	Gt ComparisonItems `json:"$gt,omitempty" yaml:"$gt,omitempty" mapstructure:"$gt,omitempty"`

	// Le corresponds to the JSON schema field "$le".
	Le ComparisonItems `json:"$le,omitempty" yaml:"$le,omitempty" mapstructure:"$le,omitempty"`

	// Lt corresponds to the JSON schema field "$lt".
	Lt ComparisonItems `json:"$lt,omitempty" yaml:"$lt,omitempty" mapstructure:"$lt,omitempty"`

	// Match corresponds to the JSON schema field "$match".
	Match []MatchExpression `json:"$match,omitempty" yaml:"$match,omitempty" mapstructure:"$match,omitempty"`

	// Ne corresponds to the JSON schema field "$ne".
	Ne ComparisonItems `json:"$ne,omitempty" yaml:"$ne,omitempty" mapstructure:"$ne,omitempty"`

	// Not corresponds to the JSON schema field "$not".
	Not *LogicalExpression `json:"$not,omitempty" yaml:"$not,omitempty" mapstructure:"$not,omitempty"`

	// Or corresponds to the JSON schema field "$or".
	Or []LogicalExpression `json:"$or,omitempty" yaml:"$or,omitempty" mapstructure:"$or,omitempty"`

	// Regex corresponds to the JSON schema field "$regex".
	Regex StringItems `json:"$regex,omitempty" yaml:"$regex,omitempty" mapstructure:"$regex,omitempty"`

	// StartsWith corresponds to the JSON schema field "$starts-with".
	StartsWith StringItems `json:"$starts-with,omitempty" yaml:"$starts-with,omitempty" mapstructure:"$starts-with,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *LogicalExpression) UnmarshalJSON(value []byte) error {
	type Plain LogicalExpression
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	if plain.And != nil && len(plain.And) < 2 {
		return fmt.Errorf("field %s length: must be >= %d", "$and", 2)
	}
	if plain.Match != nil && len(plain.Match) < 1 {
		return fmt.Errorf("field %s length: must be >= %d", "$match", 1)
	}
	if plain.Or != nil && len(plain.Or) < 2 {
		return fmt.Errorf("field %s length: must be >= %d", "$or", 2)
	}
	*j = LogicalExpression(plain)
	return nil
}

type MatchExpression struct {
	// Boolean corresponds to the JSON schema field "$boolean".
	Boolean *bool `json:"$boolean,omitempty" yaml:"$boolean,omitempty" mapstructure:"$boolean,omitempty"`

	// Contains corresponds to the JSON schema field "$contains".
	Contains StringItems `json:"$contains,omitempty" yaml:"$contains,omitempty" mapstructure:"$contains,omitempty"`

	// EndsWith corresponds to the JSON schema field "$ends-with".
	EndsWith StringItems `json:"$ends-with,omitempty" yaml:"$ends-with,omitempty" mapstructure:"$ends-with,omitempty"`

	// Eq corresponds to the JSON schema field "$eq".
	Eq ComparisonItems `json:"$eq,omitempty" yaml:"$eq,omitempty" mapstructure:"$eq,omitempty"`

	// Ge corresponds to the JSON schema field "$ge".
	Ge ComparisonItems `json:"$ge,omitempty" yaml:"$ge,omitempty" mapstructure:"$ge,omitempty"`

	// Gt corresponds to the JSON schema field "$gt".
	Gt ComparisonItems `json:"$gt,omitempty" yaml:"$gt,omitempty" mapstructure:"$gt,omitempty"`

	// Le corresponds to the JSON schema field "$le".
	Le ComparisonItems `json:"$le,omitempty" yaml:"$le,omitempty" mapstructure:"$le,omitempty"`

	// Lt corresponds to the JSON schema field "$lt".
	Lt ComparisonItems `json:"$lt,omitempty" yaml:"$lt,omitempty" mapstructure:"$lt,omitempty"`

	// Match corresponds to the JSON schema field "$match".
	Match []MatchExpression `json:"$match,omitempty" yaml:"$match,omitempty" mapstructure:"$match,omitempty"`

	// Ne corresponds to the JSON schema field "$ne".
	Ne ComparisonItems `json:"$ne,omitempty" yaml:"$ne,omitempty" mapstructure:"$ne,omitempty"`

	// Regex corresponds to the JSON schema field "$regex".
	Regex StringItems `json:"$regex,omitempty" yaml:"$regex,omitempty" mapstructure:"$regex,omitempty"`

	// StartsWith corresponds to the JSON schema field "$starts-with".
	StartsWith StringItems `json:"$starts-with,omitempty" yaml:"$starts-with,omitempty" mapstructure:"$starts-with,omitempty"`
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *MatchExpression) UnmarshalJSON(value []byte) error {
	type Plain MatchExpression
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	if plain.Match != nil && len(plain.Match) < 1 {
		return fmt.Errorf("field %s length: must be >= %d", "$match", 1)
	}
	*j = MatchExpression(plain)
	return nil
}

type ModelStringPattern string

// UnmarshalJSON implements json.Unmarshaler.
func (j *ModelStringPattern) UnmarshalJSON(value []byte) error {
	type Plain ModelStringPattern
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	if matched, _ := regexp.MatchString(`^(?:\$aas#(?:idShort|id|assetInformation\.assetKind|assetInformation\.assetType|assetInformation\.globalAssetId|assetInformation\.(?:specificAssetIds\[[0-9]*\](?:\.(?:name|value|externalSubjectId(?:\.type|\.keys\[\d*\](?:\.(?:type|value))?)?)?)|submodels\.(?:type|keys\[\d*\](?:\.(?:type|value))?))|submodels\.(type|keys\[\d*\](?:\.(type|value))?))|(?:\$sm#(?:semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id))|(?:\$sme(?:\.[a-zA-Z][a-zA-Z0-9_]*\[[0-9]*\]?(?:\.[a-zA-Z][a-zA-Z0-9_]*\[[0-9]*\]?)*)?#(?:semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|value|valueType|language))|(?:\$cd#(?:idShort|id)))|(?:\$aasdesc#(?:idShort|id|assetKind|assetType|globalAssetId|specificAssetIds\[[0-9]*\]?(?:\.(name|value|externalSubjectId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?)?)|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href)|submodelDescriptors\[[0-9]*\]\.(semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href))))|(?:\$smdesc#(?:semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href)))$`, string(plain)); !matched {
		return fmt.Errorf("field %s pattern match: must match %s", "", `^(?:\$aas#(?:idShort|id|assetInformation\.assetKind|assetInformation\.assetType|assetInformation\.globalAssetId|assetInformation\.(?:specificAssetIds\[[0-9]*\](?:\.(?:name|value|externalSubjectId(?:\.type|\.keys\[\d*\](?:\.(?:type|value))?)?)?)|submodels\.(?:type|keys\[\d*\](?:\.(?:type|value))?))|submodels\.(type|keys\[\d*\](?:\.(type|value))?))|(?:\$sm#(?:semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id))|(?:\$sme(?:\.[a-zA-Z][a-zA-Z0-9_]*\[[0-9]*\]?(?:\.[a-zA-Z][a-zA-Z0-9_]*\[[0-9]*\]?)*)?#(?:semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|value|valueType|language))|(?:\$cd#(?:idShort|id)))|(?:\$aasdesc#(?:idShort|id|assetKind|assetType|globalAssetId|specificAssetIds\[[0-9]*\]?(?:\.(name|value|externalSubjectId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?)?)|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href)|submodelDescriptors\[[0-9]*\]\.(semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href))))|(?:\$smdesc#(?:semanticId(?:\.type|\.keys\[\d*\](?:\.(type|value))?)?|idShort|id|endpoints\[[0-9]*\]\.(interface|protocolinformation\.href)))$`)
	}
	*j = ModelStringPattern(plain)
	return nil
}

type OBJECTTYPE string

type ObjectItem struct {
	Kind  OBJECTTYPE
	Value string
}

const Route OBJECTTYPE = "ROUTE"
const Identifiable OBJECTTYPE = "IDENTIFIABLE"
const Refarable OBJECTTYPE = "REFERABLE"
const Fragment OBJECTTYPE = "FRAGMENT"
const Descriptor OBJECTTYPE = "DESCRIPTOR"

func (o *ObjectItem) UnmarshalJSON(b []byte) error {
	// Expect a single-key object with a string value.
	var raw map[string]interface{}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	if len(raw) != 1 {
		return fmt.Errorf("ObjectItem: expected exactly one key, got %d", len(raw))
	}

	// Convert allowed keys into a quick lookup map for validation
	allowed := map[string]struct{}{
		"ROUTE":        {},
		"IDENTIFIABLE": {},
		"REFERABLE":    {},
		"FRAGMENT":     {},
		"DESCRIPTOR":   {},
	}

	for k, v := range raw {
		if _, ok := allowed[k]; !ok {
			return fmt.Errorf("ObjectItem: invalid key %q (allowed: ROUTE, IDENTIFIABLE, REFERABLE, FRAGMENT, DESCRIPTOR)", k)
		}

		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("ObjectItem: value for %q must be a string", k)
		}

		o.Kind = OBJECTTYPE(k)
		o.Value = s
		break
	}

	return nil
}

type RightsEnum string

const RightsEnumALL RightsEnum = "ALL"
const RightsEnumCREATE RightsEnum = "CREATE"
const RightsEnumDELETE RightsEnum = "DELETE"
const RightsEnumEXECUTE RightsEnum = "EXECUTE"
const RightsEnumREAD RightsEnum = "READ"
const RightsEnumTREE RightsEnum = "TREE"
const RightsEnumUPDATE RightsEnum = "UPDATE"
const RightsEnumVIEW RightsEnum = "VIEW"

var enumValues_RightsEnum = []interface{}{
	"CREATE",
	"READ",
	"UPDATE",
	"DELETE",
	"EXECUTE",
	"VIEW",
	"ALL",
	"TREE",
}

// UnmarshalJSON implements json.Unmarshaler.
func (j *RightsEnum) UnmarshalJSON(value []byte) error {
	var v string
	if err := json.Unmarshal(value, &v); err != nil {
		return err
	}
	var ok bool
	for _, expected := range enumValues_RightsEnum {
		if reflect.DeepEqual(v, expected) {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("invalid value (expected one of %#v): %#v", enumValues_RightsEnum, v)
	}
	*j = RightsEnum(v)
	return nil
}

type StandardString string

// UnmarshalJSON implements json.Unmarshaler.
func (j *StandardString) UnmarshalJSON(value []byte) error {
	type Plain StandardString
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	if matched, _ := regexp.MatchString(`^([^$].*|)$`, string(plain)); !matched {
		fmt.Println("---lol")
		return fmt.Errorf("field %s pattern match: must match %s", "", `^([^$].*|)$`)
	}
	*j = StandardString(plain)
	return nil
}

type StringItems []StringValue

type StringValue struct {
	// Attribute corresponds to the JSON schema field "$attribute".
	Attribute AttributeValue `json:"$attribute,omitempty" yaml:"$attribute,omitempty" mapstructure:"$attribute,omitempty"`

	// Field corresponds to the JSON schema field "$field".
	Field *ModelStringPattern `json:"$field,omitempty" yaml:"$field,omitempty" mapstructure:"$field,omitempty"`

	// StrCast corresponds to the JSON schema field "$strCast".
	StrCast *Value `json:"$strCast,omitempty" yaml:"$strCast,omitempty" mapstructure:"$strCast,omitempty"`

	// StrVal corresponds to the JSON schema field "$strVal".
	StrVal *StandardString `json:"$strVal,omitempty" yaml:"$strVal,omitempty" mapstructure:"$strVal,omitempty"`
}

type TimeLiteralPattern string

// UnmarshalJSON implements json.Unmarshaler.
func (j *TimeLiteralPattern) UnmarshalJSON(value []byte) error {
	type Plain TimeLiteralPattern
	var plain Plain
	if err := json.Unmarshal(value, &plain); err != nil {
		return err
	}
	if matched, _ := regexp.MatchString(`^[0-9][0-9]:[0-9][0-9](:[0-9][0-9])?$`, string(plain)); !matched {
		return fmt.Errorf("field %s pattern match: must match %s", "", `^[0-9][0-9]:[0-9][0-9](:[0-9][0-9])?$`)
	}
	*j = TimeLiteralPattern(plain)
	return nil
}

type Value struct {
	// Attribute corresponds to the JSON schema field "$attribute".
	Attribute AttributeValue `json:"$attribute,omitempty" yaml:"$attribute,omitempty" mapstructure:"$attribute,omitempty"`

	// BoolCast corresponds to the JSON schema field "$boolCast".
	BoolCast *Value `json:"$boolCast,omitempty" yaml:"$boolCast,omitempty" mapstructure:"$boolCast,omitempty"`

	// Boolean corresponds to the JSON schema field "$boolean".
	Boolean *bool `json:"$boolean,omitempty" yaml:"$boolean,omitempty" mapstructure:"$boolean,omitempty"`

	// DateTimeCast corresponds to the JSON schema field "$dateTimeCast".
	DateTimeCast *Value `json:"$dateTimeCast,omitempty" yaml:"$dateTimeCast,omitempty" mapstructure:"$dateTimeCast,omitempty"`

	// DateTimeVal corresponds to the JSON schema field "$dateTimeVal".
	DateTimeVal *DateTimeLiteralPattern `json:"$dateTimeVal,omitempty" yaml:"$dateTimeVal,omitempty" mapstructure:"$dateTimeVal,omitempty"`

	// DayOfMonth corresponds to the JSON schema field "$dayOfMonth".
	DayOfMonth *DateTimeLiteralPattern `json:"$dayOfMonth,omitempty" yaml:"$dayOfMonth,omitempty" mapstructure:"$dayOfMonth,omitempty"`

	// DayOfWeek corresponds to the JSON schema field "$dayOfWeek".
	DayOfWeek *DateTimeLiteralPattern `json:"$dayOfWeek,omitempty" yaml:"$dayOfWeek,omitempty" mapstructure:"$dayOfWeek,omitempty"`

	// Field corresponds to the JSON schema field "$field".
	Field *ModelStringPattern `json:"$field,omitempty" yaml:"$field,omitempty" mapstructure:"$field,omitempty"`

	// HexCast corresponds to the JSON schema field "$hexCast".
	HexCast *Value `json:"$hexCast,omitempty" yaml:"$hexCast,omitempty" mapstructure:"$hexCast,omitempty"`

	// HexVal corresponds to the JSON schema field "$hexVal".
	HexVal *HexLiteralPattern `json:"$hexVal,omitempty" yaml:"$hexVal,omitempty" mapstructure:"$hexVal,omitempty"`

	// Month corresponds to the JSON schema field "$month".
	Month *DateTimeLiteralPattern `json:"$month,omitempty" yaml:"$month,omitempty" mapstructure:"$month,omitempty"`

	// NumCast corresponds to the JSON schema field "$numCast".
	NumCast *Value `json:"$numCast,omitempty" yaml:"$numCast,omitempty" mapstructure:"$numCast,omitempty"`

	// NumVal corresponds to the JSON schema field "$numVal".
	NumVal *float64 `json:"$numVal,omitempty" yaml:"$numVal,omitempty" mapstructure:"$numVal,omitempty"`

	// StrCast corresponds to the JSON schema field "$strCast".
	StrCast *Value `json:"$strCast,omitempty" yaml:"$strCast,omitempty" mapstructure:"$strCast,omitempty"`

	// StrVal corresponds to the JSON schema field "$strVal".
	StrVal *StandardString `json:"$strVal,omitempty" yaml:"$strVal,omitempty" mapstructure:"$strVal,omitempty"`

	// TimeCast corresponds to the JSON schema field "$timeCast".
	TimeCast *Value `json:"$timeCast,omitempty" yaml:"$timeCast,omitempty" mapstructure:"$timeCast,omitempty"`

	// TimeVal corresponds to the JSON schema field "$timeVal".
	TimeVal *TimeLiteralPattern `json:"$timeVal,omitempty" yaml:"$timeVal,omitempty" mapstructure:"$timeVal,omitempty"`

	// Year corresponds to the JSON schema field "$year".
	Year *DateTimeLiteralPattern `json:"$year,omitempty" yaml:"$year,omitempty" mapstructure:"$year,omitempty"`
}
