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

package model

import "github.com/FriedJannik/aas-go-sdk/types"

// ValueOnlyPatch is the decoded ValueOnly value of a single stored submodel element.
// Pointer fields that are nil were absent in the payload and leave the stored value untouched.
type ValueOnlyPatch interface {
	GetModelType() types.ModelType
}

// ValueOnlyUpdate binds a decoded ValueOnly value to the stored submodel element it updates.
type ValueOnlyUpdate struct {
	ElementID   int64
	IDShortPath string
	ModelType   types.ModelType
	Value       ValueOnlyPatch
}

// ScalarPatch is an optional lexical value: Set reports presence, a nil Value clears the stored value.
type ScalarPatch struct {
	Set   bool
	Value *string
}

// RangeValuePatch updates the bounds of a Range.
type RangeValuePatch struct {
	ValueType types.DataTypeDefXSD
	Min       ScalarPatch
	Max       ScalarPatch
}

// GetModelType returns the model type of the patched element.
func (RangeValuePatch) GetModelType() types.ModelType { return types.ModelTypeRange }

// FileValuePatch updates the content type and path of a File.
type FileValuePatch struct {
	ContentType *string
	Value       *string
}

// GetModelType returns the model type of the patched element.
func (FileValuePatch) GetModelType() types.ModelType { return types.ModelTypeFile }

// BlobValuePatch updates the content type and binary content of a Blob.
type BlobValuePatch struct {
	ContentType *string
	Value       *[]byte
}

// GetModelType returns the model type of the patched element.
func (BlobValuePatch) GetModelType() types.ModelType { return types.ModelTypeBlob }

// ReferenceElementValuePatch replaces the reference of a ReferenceElement; a nil Value clears it.
type ReferenceElementValuePatch struct {
	Value types.IReference
}

// GetModelType returns the model type of the patched element.
func (ReferenceElementValuePatch) GetModelType() types.ModelType {
	return types.ModelTypeReferenceElement
}

// RelationshipValuePatch updates the references of a RelationshipElement or AnnotatedRelationshipElement.
type RelationshipValuePatch struct {
	ModelType types.ModelType
	First     types.IReference
	Second    types.IReference
}

// GetModelType returns the model type of the patched element.
func (r RelationshipValuePatch) GetModelType() types.ModelType { return r.ModelType }

// EntityValuePatch updates the entity attributes of an Entity; its statements are patched separately.
type EntityValuePatch struct {
	EntityType       *types.EntityType
	GlobalAssetID    *string
	SpecificAssetIDs *[]types.ISpecificAssetID
}

// GetModelType returns the model type of the patched element.
func (EntityValuePatch) GetModelType() types.ModelType { return types.ModelTypeEntity }

// BasicEventElementValuePatch updates the observed reference of a BasicEventElement.
type BasicEventElementValuePatch struct {
	Observed types.IReference
}

// GetModelType returns the model type of the patched element.
func (BasicEventElementValuePatch) GetModelType() types.ModelType {
	return types.ModelTypeBasicEventElement
}
