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

package common

import (
	"bytes"
	"fmt"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

func swaggerSupportsSecurity(specContent []byte) (bool, error) {
	var metadata struct {
		Security bool `yaml:"x-basyx-security"`
	}
	if err := yaml.Unmarshal(specContent, &metadata); err != nil {
		return false, fmt.Errorf("SWAGGER-SPEC-PARSE: %w", err)
	}
	return metadata.Security, nil
}

func configureSwaggerEdcBpnHeader(specContent []byte, enabled bool) ([]byte, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(specContent, &document); err != nil {
		return nil, fmt.Errorf("SWAGGER-EDCBPN-PARSE: %w", err)
	}
	if len(document.Content) == 0 {
		return specContent, nil
	}

	root := document.Content[0]
	definitions := swaggerMappingValue(root, "components", "parameters")
	removeSwaggerMappingKey(definitions, "EdcBpnHeader")
	if enabled {
		definitions = ensureSwaggerMapping(ensureSwaggerMapping(root, "components"), "parameters")
		definitions.Content = append(definitions.Content, swaggerStringNode("EdcBpnHeader"), swaggerEdcBpnHeader())
	}
	if paths := swaggerMappingValue(root, "paths"); paths != nil {
		for i := 1; i < len(paths.Content); i += 2 {
			configureSwaggerEdcBpnPath(paths.Content[i], enabled)
		}
	}

	var output bytes.Buffer
	encoder := yaml.NewEncoder(&output)
	encoder.SetIndent(2)
	if err := encoder.Encode(&document); err != nil {
		return nil, fmt.Errorf("SWAGGER-EDCBPN-ENCODE: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("SWAGGER-EDCBPN-CLOSE: %w", err)
	}
	return output.Bytes(), nil
}

func swaggerEdcBpnHeader() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
		Content: []*yaml.Node{
			swaggerStringNode("name"), swaggerStringNode("Edc-Bpn"),
			swaggerStringNode("in"), swaggerStringNode("header"),
			swaggerStringNode("required"), {Kind: yaml.ScalarNode, Tag: "!!bool", Value: "false"},
			swaggerStringNode("description"), swaggerStringNode("Overrides the Edc-Bpn claim when GENERAL_ENABLECUSTOMMIDDLEWAREHEADERINJECTION=true and ABAC is enabled. Supplied by trusted infrastructure; caller-controlled values can change access decisions."),
			swaggerStringNode("schema"), {Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
				swaggerStringNode("type"), swaggerStringNode("string"),
			}},
		},
	}
}

func ensureSwaggerMapping(parent *yaml.Node, key string) *yaml.Node {
	if node := swaggerMappingValue(parent, key); node != nil {
		return node
	}
	node := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	parent.Content = append(parent.Content, swaggerStringNode(key), node)
	return node
}

func configureSwaggerEdcBpnPath(pathItem *yaml.Node, enabled bool) {
	removeSwaggerEdcBpnParameters(pathItem)
	for _, method := range []string{"get", "put", "post", "delete", "options", "head", "patch", "trace"} {
		removeSwaggerEdcBpnParameters(swaggerMappingValue(pathItem, method))
	}
	if !enabled {
		return
	}
	parameters := swaggerMappingValue(pathItem, "parameters")
	if parameters == nil {
		parameters = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		pathItem.Content = append(pathItem.Content, swaggerStringNode("parameters"), parameters)
	}
	parameters.Content = append(parameters.Content, &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
		Content: []*yaml.Node{
			swaggerStringNode("$ref"), swaggerStringNode("#/components/parameters/EdcBpnHeader"),
		},
	})
}

func swaggerStringNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
}

func removeSwaggerEdcBpnParameters(operation *yaml.Node) {
	parameters := swaggerMappingValue(operation, "parameters")
	if parameters == nil || parameters.Kind != yaml.SequenceNode {
		return
	}
	parameters.Content = slices.DeleteFunc(parameters.Content, isSwaggerEdcBpnParameter)
	if len(parameters.Content) == 0 {
		removeSwaggerMappingKey(operation, "parameters")
	}
}

func isSwaggerEdcBpnParameter(parameter *yaml.Node) bool {
	if ref := swaggerMappingValue(parameter, "$ref"); ref != nil && ref.Value == "#/components/parameters/EdcBpnHeader" {
		return true
	}
	name := swaggerMappingValue(parameter, "name")
	location := swaggerMappingValue(parameter, "in")
	return name != nil && location != nil && strings.EqualFold(name.Value, "Edc-Bpn") && location.Value == "header"
}

func swaggerMappingValue(node *yaml.Node, keys ...string) *yaml.Node {
	current := node
	for _, key := range keys {
		if current == nil || current.Kind != yaml.MappingNode {
			return nil
		}
		var next *yaml.Node
		for i := 0; i < len(current.Content); i += 2 {
			if current.Content[i].Value == key {
				next = current.Content[i+1]
				break
			}
		}
		current = next
	}
	return current
}

func removeSwaggerMappingKey(node *yaml.Node, key string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content = slices.Delete(node.Content, i, i+2)
			return
		}
	}
}
