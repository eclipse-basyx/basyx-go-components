/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* Permission is hereby granted, free of charge, to any person obtaining
* a copy of this software and associated documentation files (the
* "Software"), to deal in the Software without restriction, including
* without limitation the rights to use, copy, modify, merge, publish,
* distribute, sublicense, and/or sell copies of the Software.
*
* SPDX-License-Identifier: MIT
******************************************************************************/
// Author: Jannik Fried ( Fraunhofer IESE ), Aaron Zielstorff ( Fraunhofer IESE )

package dppapi

import (
	"fmt"
	"time"

	"github.com/FriedJannik/aas-go-sdk/types"
)

func buildAAS(header dppHeader, submodelRefs []types.IReference) types.IAssetAdministrationShell {
	assetInformation := types.NewAssetInformation(granularityAssetKind(header.Granularity))
	assetInformation.SetGlobalAssetID(&header.UniqueProductIdentifier)

	aas := types.NewAssetAdministrationShell(header.DigitalProductPassportID, assetInformation)
	idShort := sanitizeIDShort(header.DigitalProductPassportID, "Dpp")
	aas.SetIDShort(&idShort)
	aas.SetSubmodels(submodelRefs)
	return aas
}

func buildMetadataSubmodel(dppID string, header dppHeader) types.ISubmodel {
	id := metadataSubmodelID(dppID)
	submodel := types.NewSubmodel(id)
	idShort := dppMetadataIDShort
	submodel.SetIDShort(&idShort)
	submodel.SetSemanticID(globalReference(dppMetadataSemanticID))
	submodel.SetSubmodelElements([]types.ISubmodelElement{
		stringProperty(headerDigitalProductPassportID, header.DigitalProductPassportID),
		stringProperty(headerUniqueProductIdentifier, header.UniqueProductIdentifier),
		stringProperty(headerGranularity, header.Granularity),
		stringProperty(headerDppSchemaVersion, header.DppSchemaVersion),
		stringProperty(headerDppStatus, header.DppStatus),
		stringProperty(headerLastUpdate, header.LastUpdate.UTC().Format(time.RFC3339Nano)),
		stringProperty(headerEconomicOperatorID, header.EconomicOperatorID),
		stringProperty(headerFacilityID, header.FacilityID),
		stringList(headerContentSpecificationIDs, header.ContentSpecificationIDs),
	})
	return submodel
}

func buildContentSubmodel(dppID string, sectionName string, semanticID string, value any) (types.ISubmodel, error) {
	submodel := types.NewSubmodel(contentSubmodelID(dppID, sectionName))
	idShort := upperFirst(sectionName)
	submodel.SetIDShort(&idShort)
	if semanticID != "" {
		submodel.SetSemanticID(globalReference(semanticID))
	}

	object, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("DPP-BUILDSM-CONTENTSECTION content section %s must be a JSON object", sectionName)
	}
	if err := rejectExpandedDataElementShape(sectionName, object); err != nil {
		return nil, err
	}
	elements := make([]types.ISubmodelElement, 0, len(object))
	keys := sortedKeys(object)
	for _, key := range keys {
		element, err := inferElement(key, object[key])
		if err != nil {
			return nil, err
		}
		elements = append(elements, element)
	}
	submodel.SetSubmodelElements(elements)
	return submodel, nil
}

func granularityAssetKind(granularity string) types.AssetKind {
	switch granularity {
	case "Model":
		return types.AssetKindType
	case "Batch":
		return types.AssetKindBatch
	default:
		return types.AssetKindInstance
	}
}
