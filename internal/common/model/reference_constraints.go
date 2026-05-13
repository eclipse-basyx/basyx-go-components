package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/aas-core-works/aas-core3.1-golang/types"
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
