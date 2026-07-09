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
	elements := []types.ISubmodelElement{
		metadataProperty(headerDigitalProductPassportID, dppIDSemanticID, header.DigitalProductPassportID, types.DataTypeDefXSDString),
		metadataProperty(headerUniqueProductIdentifier, dppProductIDSemanticID, header.UniqueProductIdentifier, types.DataTypeDefXSDString),
		metadataPropertyWithSupplementalSemanticID(headerGranularity, dppGranularitySemanticID, header.Granularity, types.DataTypeDefXSDString, dppGranularitySupplementalSemanticID),
		metadataProperty(headerDppSchemaVersion, dppSchemaVersionSemanticID, header.DppSchemaVersion, types.DataTypeDefXSDString),
		metadataProperty(headerDppStatus, dppStatusSemanticID, header.DppStatus, types.DataTypeDefXSDString),
		metadataPropertyWithSupplementalSemanticID(headerLastUpdate, dppLastUpdateSemanticID, header.LastUpdate.UTC().Format(time.RFC3339Nano), types.DataTypeDefXSDDateTime, dppAdministrativeUpdateSupplementalSemanticID),
		metadataProperty(headerEconomicOperatorID, dppEconomicOperatorIDSemanticID, header.EconomicOperatorID, types.DataTypeDefXSDString),
	}
	if header.FacilityID != "" {
		elements = append(elements, metadataProperty(headerFacilityID, dppFacilityIDSemanticID, header.FacilityID, types.DataTypeDefXSDString))
	}
	if len(header.ContentSpecificationIDs) > 0 {
		elements = append(elements, metadataContentSpecificationIDs(header.ContentSpecificationIDs))
	}
	submodel.SetSubmodelElements(elements)
	setNewDPPSubmodelAdministration(submodel, header.LastUpdate)
	return submodel
}

func metadataProperty(idShort string, semanticID string, value string, valueType types.DataTypeDefXSD) types.ISubmodelElement {
	property := scalarProperty(idShort, value, valueType)
	property.SetSemanticID(globalReference(semanticID))
	return property
}

func metadataPropertyWithSupplementalSemanticID(idShort string, semanticID string, value string, valueType types.DataTypeDefXSD, supplementalSemanticID string) types.ISubmodelElement {
	property := metadataProperty(idShort, semanticID, value, valueType)
	property.SetSupplementalSemanticIDs([]types.IReference{globalReference(supplementalSemanticID)})
	return property
}

func metadataContentSpecificationIDs(values []string) types.ISubmodelElement {
	list := types.NewSubmodelElementList(types.AASSubmodelElementsProperty)
	idShort := headerContentSpecificationIDs
	list.SetIDShort(&idShort)
	list.SetSemanticID(globalReference(dppContentSpecificationIDsSemanticID))
	list.SetSemanticIDListElement(globalReference(dppContentSpecificationIDSemanticID))
	orderRelevant := false
	list.SetOrderRelevant(&orderRelevant)
	valueType := types.DataTypeDefXSDString
	list.SetValueTypeListElement(&valueType)

	items := make([]types.ISubmodelElement, 0, len(values))
	for _, value := range values {
		item := metadataProperty("", dppContentSpecificationIDSemanticID, value, types.DataTypeDefXSDString)
		item.SetIDShort(nil)
		items = append(items, item)
	}
	list.SetValue(items)
	return list
}

func setNewDPPSubmodelAdministration(submodel types.ISubmodel, timestamp time.Time) {
	formatted := timestamp.UTC().Format(time.RFC3339Nano)
	administration := types.NewAdministrativeInformation()
	administration.SetCreatedAt(&formatted)
	administration.SetUpdatedAt(&formatted)
	submodel.SetAdministration(administration)
}

func buildContentSubmodel(dppID string, sectionName string, semanticID string, value any) (types.ISubmodel, error) {
	submodel := types.NewSubmodel(contentSubmodelID(dppID, sectionName))
	idShort := contentSectionIDShort(sectionName)
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
