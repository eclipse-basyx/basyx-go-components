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

package aasenvironment

import (
	"context"
	"strings"

	aastypes "github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func serializationReBACEnabled(ctx context.Context) bool {
	cfg, ok := common.ConfigFromContext(ctx)
	return ok && cfg != nil && cfg.ReBAC.Enabled
}

func (s *SerializationAPIService) loadCompleteReBACEnvironment(ctx context.Context, aasIDs []string, submodelIDs []string, includeConceptDescriptions bool) (aastypes.IEnvironment, error) {
	if s == nil || s.persistence == nil {
		return nil, common.NewInternalServerError("AASENV-REBACLOAD-NILSERVICE service must not be nil")
	}
	decodedAASIDs, err := decodeIdentifiers(aasIDs, "AASENV-REBACLOAD-DECODEAASID")
	if err != nil {
		return nil, err
	}
	decodedSubmodelIDs, err := decodeIdentifiers(submodelIDs, "AASENV-REBACLOAD-DECODESUBMODELID")
	if err != nil {
		return nil, err
	}
	planningCtx := auth.ReBACPlanningContext(ctx)
	rawAAS, err := s.loadAssetAdministrationShells(planningCtx, decodedAASIDs)
	if err != nil {
		return nil, err
	}
	rawSubmodels, err := s.loadSubmodels(planningCtx, decodedSubmodelIDs)
	if err != nil {
		return nil, err
	}
	rawConceptDescriptions, err := s.loadConceptDescriptions(planningCtx, includeConceptDescriptions)
	if err != nil {
		return nil, err
	}
	assetAdministrationShells, err := s.authorizedSerializationAAS(ctx, planningCtx, rawAAS)
	if err != nil {
		return nil, err
	}
	submodels, err := s.authorizedSerializationSubmodels(ctx, planningCtx, rawSubmodels)
	if err != nil {
		return nil, err
	}
	conceptDescriptions, err := s.authorizedSerializationConceptDescriptions(ctx, planningCtx, rawConceptDescriptions)
	if err != nil {
		return nil, err
	}
	environment := aastypes.NewEnvironment()
	environment.SetAssetAdministrationShells(assetAdministrationShells)
	environment.SetSubmodels(submodels)
	environment.SetConceptDescriptions(conceptDescriptions)
	return environment, nil
}

func (s *SerializationAPIService) authorizedSerializationAAS(ctx context.Context, planningCtx context.Context, candidates []aastypes.IAssetAdministrationShell) ([]aastypes.IAssetAdministrationShell, error) {
	result := make([]aastypes.IAssetAdministrationShell, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil || strings.TrimSpace(candidate.ID()) == "" {
			return nil, common.NewInternalServerError("AASENV-REBACLOAD-AASID inclusion candidate must have an identifier")
		}
		authorizedCtx, err := auth.AuthorizeReBACReadResource(ctx, "aas", candidate.ID())
		if err != nil {
			return nil, err
		}
		complete, err := s.persistence.AASRepository.GetAssetAdministrationShellByID(planningCtx, candidate.ID())
		if err != nil {
			return nil, err
		}
		visible, err := s.persistence.AASRepository.GetAssetAdministrationShellByID(authorizedCtx, candidate.ID())
		if err != nil {
			return nil, err
		}
		if err = auth.RequireCompleteReBACRepresentation(ctx, complete, visible); err != nil {
			return nil, err
		}
		result = append(result, visible)
	}
	return result, nil
}

func (s *SerializationAPIService) authorizedSerializationSubmodels(ctx context.Context, planningCtx context.Context, candidates []aastypes.ISubmodel) ([]aastypes.ISubmodel, error) {
	result := make([]aastypes.ISubmodel, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil || strings.TrimSpace(candidate.ID()) == "" {
			return nil, common.NewInternalServerError("AASENV-REBACLOAD-SMID inclusion candidate must have an identifier")
		}
		authorizedCtx, err := auth.AuthorizeReBACReadResource(ctx, "submodel", candidate.ID())
		if err != nil {
			return nil, err
		}
		complete, err := s.persistence.SubmodelRepository.GetSubmodelByID(planningCtx, candidate.ID(), "deep", false, true)
		if err != nil {
			return nil, err
		}
		visible, err := s.persistence.SubmodelRepository.GetSubmodelByID(authorizedCtx, candidate.ID(), "deep", false, true)
		if err != nil {
			return nil, err
		}
		if err = auth.RequireCompleteReBACRepresentation(ctx, complete, visible); err != nil {
			return nil, err
		}
		result = append(result, visible)
	}
	return result, nil
}

func (s *SerializationAPIService) authorizedSerializationConceptDescriptions(ctx context.Context, planningCtx context.Context, candidates []aastypes.IConceptDescription) ([]aastypes.IConceptDescription, error) {
	result := make([]aastypes.IConceptDescription, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil || strings.TrimSpace(candidate.ID()) == "" {
			return nil, common.NewInternalServerError("AASENV-REBACLOAD-CDID inclusion candidate must have an identifier")
		}
		authorizedCtx, err := auth.AuthorizeReBACReadResource(ctx, "concept_description", candidate.ID())
		if err != nil {
			return nil, err
		}
		complete, err := s.persistence.ConceptDescriptionRepository.GetConceptDescriptionByID(planningCtx, candidate.ID())
		if err != nil {
			return nil, err
		}
		visible, err := s.persistence.ConceptDescriptionRepository.GetConceptDescriptionByID(authorizedCtx, candidate.ID())
		if err != nil {
			return nil, err
		}
		if err = auth.RequireCompleteReBACRepresentation(ctx, complete, visible); err != nil {
			return nil, err
		}
		result = append(result, visible)
	}
	return result, nil
}

func serializationReBACResourceContext(ctx context.Context, kind, id string) (context.Context, error) {
	if !serializationReBACEnabled(ctx) {
		return ctx, nil
	}
	return auth.AuthorizeReBACReadResource(ctx, kind, id)
}
