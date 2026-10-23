/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
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

package aol

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/jsonization"
	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/FriedJannik/aas-go-sdk/verification"
)

// Draft contains client-extracted AAS content and source provenance, never source documents.
type Draft struct {
	Shell     map[string]any    `json:"shell"`
	Submodels []map[string]any  `json:"submodels"`
	Templates map[string]string `json:"templates"`
	Sources   []Source          `json:"sources,omitempty"`
}

// Source records extraction provenance without treating it as an instruction.
type Source struct {
	Document string `json:"document"`
	Page     int    `json:"page"`
	Location string `json:"location"`
}

// Cardinality matches submodel_info output contract 2.x.
type Cardinality struct {
	Raw string `json:"raw"`
	Min *int   `json:"min_occurs"`
	Max *int   `json:"max_occurs"`
}

// Node describes one template element.
type Node struct {
	IDShort         string      `json:"id_short"`
	ModelType       string      `json:"model_type"`
	ValueType       string      `json:"value_type"`
	SemanticID      string      `json:"semantic_id"`
	Cardinality     Cardinality `json:"cardinality"`
	Children        []Node      `json:"children"`
	Item            *Node       `json:"item_schema"`
	ListElementType string      `json:"list_element_type"`
	ListSemanticID  string      `json:"list_element_semantic_id"`
}

// Template is an allowlisted structure and its client-facing cheat sheet.
type Template struct {
	Slug       string          `json:"schema_slug"`
	Version    string          `json:"schema_version"`
	Root       Node            `json:"root"`
	CheatSheet string          `json:"cheat_sheet"`
	Structure  json.RawMessage `json:"structure,omitempty"`
}

// Issue identifies a failed draft contract at a JSON location.
type Issue struct {
	Level    string `json:"level"`
	Code     string `json:"code"`
	Location string `json:"location"`
	Message  string `json:"message"`
}

// Report is side-effect-free and must be valid before a plan can be approved.
type Report struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues"`
}

// Validator checks AAS metamodel and allowlisted template structures.
type Validator struct{ Templates map[string]Template }

// LoadTemplates reads only administrator-selected local artifacts, bounded in size.
func LoadTemplates(directory string, slugs []string) (map[string]Template, error) {
	templates := make(map[string]Template)
	for _, slug := range slugs {
		t, err := loadTemplate(directory, slug)
		if err != nil {
			return nil, err
		}
		templates[slug] = t
	}
	return templates, nil
}
func loadTemplate(directory, slug string) (Template, error) {
	if slug == "" || strings.ContainsAny(slug, "/\\.") {
		return Template{}, Failure("AOL-TEMPLATE-SLUG", "invalid allowlist slug", "Use a simple template slug.")
	}
	raw, err := readArtifact(filepath.Join(directory, slug+".structure.json"))
	if err != nil {
		return Template{}, err
	}
	var t Template
	if err = json.Unmarshal(raw, &t); err != nil {
		return t, fmt.Errorf("AOL-TEMPLATE-DECODE: %w", err)
	}
	if t.Slug != slug || t.Root.ModelType != "Submodel" || t.Root.SemanticID == "" {
		return t, Failure("AOL-TEMPLATE-CONTRACT", "invalid template root or slug", "Install a qualified submodel_info structure artifact.")
	}
	if err = qualifyNode(t.Root, 0); err != nil {
		return t, err
	}
	cheat, err := readArtifact(filepath.Join(directory, slug+".cheatsheet.md"))
	if err != nil {
		return t, err
	}
	t.Structure = raw
	t.CheatSheet = string(cheat)
	return t, nil
}
func readArtifact(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("AOL-TEMPLATE-READ: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > 1048576 {
		return nil, Failure("AOL-TEMPLATE-SIZE", "artifact is not a regular file within 1 MiB", "Install a bounded read-only artifact.")
	}
	//nolint:gosec // Paths come only from the administrator template directory and validated allowlist.
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("AOL-TEMPLATE-READ: %w", err)
	}
	return b, nil
}
func qualifyNode(n Node, depth int) error {
	if depth > 32 {
		return Failure("AOL-TEMPLATE-DEPTH", "template nesting exceeds 32", "Use a qualified template.")
	}
	for _, child := range n.Children {
		if child.Cardinality.Min == nil || child.Cardinality.Raw == "Unknown" {
			return Failure("AOL-TEMPLATE-CARDINALITY", "unknown cardinality at "+child.IDShort, "Qualify the template before enabling writes.")
		}
		if err := qualifyNode(child, depth+1); err != nil {
			return err
		}
	}
	if n.Item != nil {
		return qualifyNode(*n.Item, depth+1)
	}
	return nil
}

// Validate applies strict metamodel validation regardless of REST verification mode.
func (v Validator) Validate(draft Draft) Report {
	report := Report{Issues: []Issue{}}
	verifyJSON(&report, "/shell", draft.Shell, jsonization.AssetAdministrationShellFromJsonable)
	ids := map[string]bool{}
	smIDs := map[string]bool{}
	registerID(&report, ids, draft.Shell, "/shell")
	for i, sm := range draft.Submodels {
		location := fmt.Sprintf("/submodels/%d", i)
		registerID(&report, ids, sm, location)
		smIDs[stringField(sm, "id")] = true
		verifyJSON(&report, location, sm, jsonization.SubmodelFromJsonable)
		slug := draft.Templates[stringField(sm, "id")]
		template, ok := v.Templates[slug]
		if !ok {
			addIssue(&report, "L2", location, "template is not allowlisted")
			continue
		}
		validateNode(&report, template.Root, sm, location)
	}
	refs, _ := draft.Shell["submodels"].([]any)
	for i, ref := range refs {
		if !smIDs[referenceID(ref)] {
			addIssue(&report, "L1", fmt.Sprintf("/shell/submodels/%d", i), "reference must target a submodel in this changeset")
		}
	}
	for i, source := range draft.Sources {
		if source.Document == "" || source.Page < 1 {
			addIssue(&report, "L1", fmt.Sprintf("/sources/%d", i), "document name and positive page are required")
		}
	}
	report.Valid = len(report.Issues) == 0
	return report
}
func verifyJSON[T types.IClass](report *Report, path string, value map[string]any, decode func(any) (T, error)) {
	instance, err := decode(value)
	if err != nil {
		addIssue(report, "L1", path, err.Error())
		return
	}
	verification.Verify(instance, func(err *verification.VerificationError) bool {
		addIssue(report, "L1", path, err.Error())
		return false
	})
}
func registerID(report *Report, ids map[string]bool, value map[string]any, path string) {
	id := stringField(value, "id")
	if id == "" || ids[id] {
		addIssue(report, "L1", path+"/id", "identifier must be present and unique")
	}
	ids[id] = true
}
func addIssue(report *Report, level, path, message string) {
	report.Issues = append(report.Issues, Issue{Level: level, Code: "AOL-VALIDATE-" + level, Location: path, Message: message})
}
func validateNode(report *Report, node Node, value map[string]any, path string) {
	valueTypeKey := "valueType"
	if node.ModelType == "SubmodelElementList" {
		valueTypeKey = "valueTypeListElement"
	}
	for _, check := range []struct{ key, expected string }{{"modelType", node.ModelType}, {valueTypeKey, node.ValueType}} {
		if check.expected != "" && stringField(value, check.key) != check.expected {
			addIssue(report, "L2", path+"/"+check.key, "expected "+check.expected)
		}
	}
	if node.SemanticID != "" && referenceID(value["semanticId"]) != node.SemanticID {
		addIssue(report, "L2", path+"/semanticId", "expected "+node.SemanticID)
	}
	if node.ModelType == "SubmodelElementList" {
		validateList(report, node, value, path)
		return
	}
	children, _ := value[childField(node.ModelType)].([]any)
	validateChildren(report, node.Children, children, path+"/"+childField(node.ModelType))
}
func childField(modelType string) string {
	if modelType == "Submodel" {
		return "submodelElements"
	}
	if modelType == "Entity" {
		return "statements"
	}
	if modelType == "AnnotatedRelationshipElement" {
		return "annotations"
	}
	return "value"
}
func validateChildren(report *Report, nodes []Node, values []any, path string) {
	validateUniqueChildren(report, values, path)
	for _, node := range nodes {
		count := 0
		for i, raw := range values {
			value, _ := raw.(map[string]any)
			if stringField(value, "idShort") == node.IDShort {
				count++
				validateNode(report, node, value, fmt.Sprintf("%s/%d", path, i))
			}
		}
		if node.Cardinality.Min == nil || count < *node.Cardinality.Min || (node.Cardinality.Max != nil && count > *node.Cardinality.Max) {
			addIssue(report, "L2", path+"/"+node.IDShort, "template cardinality violated")
		}
	}
	for i, raw := range values {
		value, _ := raw.(map[string]any)
		if !knownChild(nodes, stringField(value, "idShort")) {
			addIssue(report, "L2", fmt.Sprintf("%s/%d", path, i), "element is not defined by the template")
		}
	}
}
func knownChild(nodes []Node, id string) bool {
	for _, node := range nodes {
		if node.IDShort == id {
			return true
		}
	}
	return false
}
func validateList(report *Report, node Node, value map[string]any, path string) {
	if node.ListElementType != "" && stringField(value, "typeValueListElement") != node.ListElementType {
		addIssue(report, "L2", path+"/typeValueListElement", "expected "+node.ListElementType)
	}
	semantic := referenceID(value["semanticIdListElement"])
	if node.ListSemanticID != "" && semantic != node.ListSemanticID {
		addIssue(report, "L2", path+"/semanticIdListElement", "expected "+node.ListSemanticID)
	}
	values, _ := value["value"].([]any)
	if node.Item == nil {
		if len(values) > 0 {
			addIssue(report, "L2", path, "list item schema is unavailable")
		}
		return
	}
	if node.Item.Cardinality.Min == nil || len(values) < *node.Item.Cardinality.Min || (node.Item.Cardinality.Max != nil && len(values) > *node.Item.Cardinality.Max) {
		addIssue(report, "L2", path+"/value", "template item cardinality violated")
	}
	validateListItems(report, *node.Item, values, semantic, path)
}
func validateListItems(report *Report, node Node, values []any, semantic, path string) {
	for i, raw := range values {
		item, _ := raw.(map[string]any)
		itemNode := node
		if referenceID(item["semanticId"]) == "" && semantic == node.SemanticID {
			itemNode.SemanticID = ""
		}
		validateNode(report, itemNode, item, fmt.Sprintf("%s/value/%d", path, i))
	}
}

func validateUniqueChildren(report *Report, values []any, path string) {
	seen := map[string]bool{}
	for i, raw := range values {
		value, _ := raw.(map[string]any)
		id := stringField(value, "idShort")
		if id == "" || seen[id] {
			addIssue(report, "L1", fmt.Sprintf("%s/%d/idShort", path, i), "idShort must be present and unique")
		}
		seen[id] = true
	}
}
