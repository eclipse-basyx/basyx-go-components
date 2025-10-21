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

// Author: Aaron Zielstorff ( Fraunhofer IESE ), Jannik Fried ( Fraunhofer IESE )
package builder

import (
	"encoding/json"
	"fmt"

	gen "github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

type ExtensionsBuilder struct {
	extensions    map[int64]*gen.Extension    // Maps database IDs to extension objects
	refBuilderMap map[int64]*ReferenceBuilder // Maps reference database IDs to their builders
}

func NewExtensionsBuilder() *ExtensionsBuilder {
	return &ExtensionsBuilder{extensions: make(map[int64]*gen.Extension), refBuilderMap: make(map[int64]*ReferenceBuilder)}
}

func (b *ExtensionsBuilder) AddExtension(extensionDbId int64, name string, valueType string, value string) (*ExtensionsBuilder, error) {
	_, exists := b.extensions[extensionDbId]
	if !exists {
		b.extensions[extensionDbId] = &gen.Extension{
			Name:  name,
			Value: value,
		}
		if valueType != "" {
			ValueType, err := gen.NewDataTypeDefXsdFromValue(valueType)
			if err != nil {
				fmt.Println(err)
				return nil, fmt.Errorf("error parsing ValueType for Extension '%d' to Go Struct. See console for details", extensionDbId)
			}
			b.extensions[extensionDbId].ValueType = ValueType
		}
	} else {
		fmt.Printf("[Warning] Extension with id '%d' already exists - skipping.", extensionDbId)
	}
	return b, nil
}

func (b *ExtensionsBuilder) AddSemanticId(extensionDbId int64, semanticIdRows json.RawMessage, semanticIdReferredSemanticIdRows json.RawMessage) (*ExtensionsBuilder, error) {
	extension := b.extensions[extensionDbId]

	semanticId, err := b.createExactlyOneReference(extensionDbId, semanticIdRows, semanticIdReferredSemanticIdRows, "SemanticID")

	if err != nil {
		return nil, err
	}

	extension.SemanticId = semanticId

	return b, nil
}
func (b *ExtensionsBuilder) AddSupplementalSemanticIds(extensionDbId int64, supplementalSemanticIdsRows json.RawMessage, supplementalSemanticIdsReferredSemanticIdRows json.RawMessage) (*ExtensionsBuilder, error) {
	extension, exists := b.extensions[extensionDbId]

	if !exists {
		return nil, fmt.Errorf("tried to add SupplementalSemanticIds to Extension '%d' before creating the Extension itself", extensionDbId)
	}

	refs, err := ParseReferences(supplementalSemanticIdsRows, b.refBuilderMap)

	if err != nil {
		return nil, err
	}

	if len(supplementalSemanticIdsReferredSemanticIdRows) > 0 {
		ParseReferredReferences(supplementalSemanticIdsReferredSemanticIdRows, b.refBuilderMap)
	}

	suppl := []gen.Reference{}

	for _, el := range refs {
		suppl = append(suppl, *el)
	}

	extension.SupplementalSemanticIds = suppl

	return b, nil
}
func (b *ExtensionsBuilder) AddRefersTo(extensionDbId int64, refersToRows json.RawMessage, refersToReferredRows json.RawMessage) (*ExtensionsBuilder, error) {
	extension, exists := b.extensions[extensionDbId]

	if !exists {
		return nil, fmt.Errorf("tried to add RefersTo to Extension '%d' before creating the Extension itself", extensionDbId)
	}

	refs, err := ParseReferences(refersToRows, b.refBuilderMap)

	if err != nil {
		return nil, err
	}

	if len(refersToReferredRows) > 0 {
		ParseReferredReferences(refersToReferredRows, b.refBuilderMap)
	}

	suppl := []gen.Reference{}

	for _, el := range refs {
		suppl = append(suppl, *el)
	}

	extension.RefersTo = suppl

	return b, nil
}

func (b *ExtensionsBuilder) createExactlyOneReference(extensionDbId int64, refRows json.RawMessage, referredRefRows json.RawMessage, Type string) (*gen.Reference, error) {
	_, exists := b.extensions[extensionDbId]

	if !exists {
		return nil, fmt.Errorf("tried to add %s to Extension '%d' before creating the Extension itself", Type, extensionDbId)
	}

	refs, err := ParseReferences(refRows, b.refBuilderMap)

	if err != nil {
		return nil, err
	}

	if len(referredRefRows) > 0 {
		ParseReferredReferences(referredRefRows, b.refBuilderMap)
	}

	if len(refs) != 1 {
		return nil, fmt.Errorf("expected exactly one or no %s for Extension '%d' but got %d", Type, extensionDbId, len(refs))
	}

	return refs[0], nil
}

func (b *ExtensionsBuilder) Build() []gen.Extension {

	for _, builder := range b.refBuilderMap {
		builder.BuildNestedStructure()
	}

	extensions := make([]gen.Extension, 0, len(b.extensions))
	for _, extension := range b.extensions {
		extensions = append(extensions, *extension)
	}

	return extensions
}
