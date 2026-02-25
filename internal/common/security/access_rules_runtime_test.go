package auth

import (
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

func TestDematerializeRulesFromModelPayloadExpandsDefinitions(t *testing.T) {
	payload := []byte(`{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [
				{ "name": "role_attr", "attributes": [ { "CLAIM": "role" } ] }
			],
			"DEFOBJECTS": [
				{ "name": "desc", "objects": [ { "ROUTE": "/description" } ] }
			],
			"DEFACLS": [
				{ "name": "read_acl", "acl": { "USEATTRIBUTES": "role_attr", "RIGHTS": ["READ"], "ACCESS": "ALLOW" } }
			],
			"DEFFORMULAS": [
				{ "name": "always_true", "formula": { "$boolean": true } }
			],
			"rules": [
				{ "USEACL": "read_acl", "USEOBJECTS": ["desc"], "USEFORMULA": "always_true" }
			]
		}
	}`)

	rules, err := dematerializeRulesFromModelPayload(payload)
	if err != nil {
		t.Fatalf("dematerializeRulesFromModelPayload() error = %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	rule := rules[0]
	if rule.ACL == nil {
		t.Fatalf("expected ACL to be materialized")
	}
	if rule.ACL.USEATTRIBUTES != nil {
		t.Fatalf("expected USEATTRIBUTES to be removed after dematerialization")
	}
	if len(rule.ACL.ATTRIBUTES) != 1 {
		t.Fatalf("expected ACL attributes to be materialized, got %d", len(rule.ACL.ATTRIBUTES))
	}
	if rule.FORMULA == nil {
		t.Fatalf("expected FORMULA to be materialized")
	}
	if rule.USEFORMULA != nil {
		t.Fatalf("expected USEFORMULA to be removed")
	}
	if rule.USEACL != nil {
		t.Fatalf("expected USEACL to be removed")
	}
	if len(rule.USEOBJECTS) != 0 {
		t.Fatalf("expected USEOBJECTS to be removed")
	}
	if len(rule.OBJECTS) != 1 {
		t.Fatalf("expected OBJECTS to be materialized, got %d", len(rule.OBJECTS))
	}
}

func TestNewStoredRuleRoundTripsDematerializedRuleJSON(t *testing.T) {
	payload := []byte(`{
		"AllAccessPermissionRules": {
			"DEFATTRIBUTES": [
				{ "name": "anonym_attr", "attributes": [ { "GLOBAL": "ANONYMOUS" } ] }
			],
			"DEFOBJECTS": [
				{ "name": "description", "objects": [ { "ROUTE": "/description" } ] }
			],
			"DEFACLS": [
				{ "name": "read_anonymous", "acl": { "USEATTRIBUTES": "anonym_attr", "RIGHTS": ["READ"], "ACCESS": "ALLOW" } }
			],
			"DEFFORMULAS": [
				{ "name": "always_true", "formula": { "$boolean": true } }
			],
			"rules": [
				{ "USEACL": "read_anonymous", "USEOBJECTS": ["description"], "USEFORMULA": "always_true" }
			]
		}
	}`)

	rules, err := dematerializeRulesFromModelPayload(payload)
	if err != nil {
		t.Fatalf("dematerializeRulesFromModelPayload() error = %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(rules))
	}

	stored, err := newStoredRule(rules[0])
	if err != nil {
		t.Fatalf("newStoredRule() error = %v", err)
	}

	var got grammar.AccessPermissionRule
	if err := common.UnmarshalAndDisallowUnknownFields([]byte(stored.json), &got); err != nil {
		t.Fatalf("stored rule json should round-trip via strict unmarshal, got error: %v\njson=%s", err, stored.json)
	}
}
