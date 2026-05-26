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

// NormalizePayloadNullFields removes null-valued fields recursively from JSON-like payloads.
func NormalizePayloadNullFields(payload any) {
	normalizedPayload := normalizePayloadNullFieldsIterative(payload)
	rootMap, rootIsMap := payload.(map[string]any)
	normalizedMap, normalizedIsMap := normalizedPayload.(map[string]any)
	if !rootIsMap || !normalizedIsMap {
		return
	}

	clear(rootMap)
	for key, value := range normalizedMap {
		rootMap[key] = value
	}
}

// NormalizeAASPayloadOptionalArrays normalizes AAS payloads by removing null-valued fields.
func NormalizeAASPayloadOptionalArrays(payload any) {
	NormalizePayloadNullFields(payload)
}

// NormalizeSubmodelPayloadOptionalArrays normalizes submodel payloads by removing null-valued fields.
func NormalizeSubmodelPayloadOptionalArrays(payload any) {
	NormalizePayloadNullFields(payload)
}

type normalizerFrameKind uint8

const (
	normalizerFrameKindRoot normalizerFrameKind = iota
	normalizerFrameKindMap
	normalizerFrameKindSlice
)

type normalizerFrame struct {
	stage             uint8
	value             any
	result            any
	kind              normalizerFrameKind
	parent            *normalizerFrame
	parentMapKey      string
	parentSliceIndex  int
	mapKeys           []string
	mapChildResults   map[string]any
	sliceChildResults []any
}

func normalizePayloadNullFieldsIterative(payload any) any {
	rootFrame := &normalizerFrame{
		value: payload,
		kind:  normalizerFrameKindRoot,
	}
	stack := []*normalizerFrame{rootFrame}

	for len(stack) > 0 {
		activeFrame := stack[len(stack)-1]
		switch activeFrame.stage {
		case 0:
			switch typedValue := activeFrame.value.(type) {
			case map[string]any:
				activeFrame.stage = 1
				activeFrame.mapChildResults = make(map[string]any, len(typedValue))
				activeFrame.mapKeys = make([]string, 0, len(typedValue))

				for key, value := range typedValue {
					activeFrame.mapKeys = append(activeFrame.mapKeys, key)
					childFrame := &normalizerFrame{
						value:        value,
						kind:         normalizerFrameKindMap,
						parent:       activeFrame,
						parentMapKey: key,
					}
					stack = append(stack, childFrame)
				}
			case []any:
				activeFrame.stage = 1
				activeFrame.sliceChildResults = make([]any, len(typedValue))

				for index, value := range typedValue {
					childFrame := &normalizerFrame{
						value:            value,
						kind:             normalizerFrameKindSlice,
						parent:           activeFrame,
						parentSliceIndex: index,
					}
					stack = append(stack, childFrame)
				}
			default:
				activeFrame.result = typedValue
				stack = stack[:len(stack)-1]
				propagateNormalizationResultToParent(activeFrame)
			}
		default:
			switch activeFrame.value.(type) {
			case map[string]any:
				normalizedMap := make(map[string]any, len(activeFrame.mapChildResults))
				for _, key := range activeFrame.mapKeys {
					normalizedChildValue := activeFrame.mapChildResults[key]
					if normalizedChildValue == nil {
						continue
					}
					normalizedMap[key] = normalizedChildValue
				}
				activeFrame.result = normalizedMap
			case []any:
				normalizedSlice := make([]any, 0, len(activeFrame.sliceChildResults))
				for _, normalizedChildValue := range activeFrame.sliceChildResults {
					if normalizedChildValue == nil {
						continue
					}
					normalizedSlice = append(normalizedSlice, normalizedChildValue)
				}
				activeFrame.result = normalizedSlice
			}

			stack = stack[:len(stack)-1]
			propagateNormalizationResultToParent(activeFrame)
		}
	}

	return rootFrame.result
}

func propagateNormalizationResultToParent(frame *normalizerFrame) {
	if frame == nil || frame.parent == nil {
		return
	}

	switch frame.kind {
	case normalizerFrameKindMap:
		frame.parent.mapChildResults[frame.parentMapKey] = frame.result
	case normalizerFrameKindSlice:
		frame.parent.sliceChildResults[frame.parentSliceIndex] = frame.result
	}
}
