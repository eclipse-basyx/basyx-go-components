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

import (
	"errors"
	"strconv"
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
)

func assertReferenceKeysNotEmpty(reference types.IReference, path string) error {
	if reference == nil {
		return nil
	}
	if len(reference.Keys()) == 0 {
		return errors.New("COMMON-REFCONSTRAINTS-EMPTYKEYS " + path + ".keys must not be empty")
	}
	return assertReferenceKeysNotEmpty(reference.ReferredSemanticID(), path+".referredSemanticId")
}

func assertReferenceKeyValuesNotEmpty(reference types.IReference, path string) error {
	if reference == nil {
		return nil
	}
	for idx, key := range reference.Keys() {
		if key == nil {
			return errors.New("COMMON-REFCONSTRAINTS-NILKEY " + path + ".keys contains nil key")
		}
		if strings.TrimSpace(key.Value()) == "" {
			return errors.New("COMMON-REFCONSTRAINTS-EMPTYKEYVALUE " + path + ".keys[" + strconv.Itoa(idx) + "].value must not be empty")
		}
	}
	return assertReferenceKeyValuesNotEmpty(reference.ReferredSemanticID(), path+".referredSemanticId")
}

func assertSpecificAssetIDReferencesHaveKeys(specificAssetIDs []types.ISpecificAssetID) error {
	for _, specificAssetID := range specificAssetIDs {
		if specificAssetID == nil {
			continue
		}
		if err := assertReferenceKeysNotEmpty(specificAssetID.SemanticID(), "specificAssetIds.semanticId"); err != nil {
			return err
		}
		if err := assertReferenceKeysNotEmpty(specificAssetID.ExternalSubjectID(), "specificAssetIds.externalSubjectId"); err != nil {
			return err
		}
		for _, reference := range specificAssetID.SupplementalSemanticIDs() {
			if err := assertReferenceKeysNotEmpty(reference, "specificAssetIds.supplementalSemanticIds"); err != nil {
				return err
			}
		}
	}
	return nil
}
