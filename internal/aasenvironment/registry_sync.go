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
	"strings"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/registrysync"
)

const externalURLKey = "general.externalUrl"

// RegistrySyncConfig controls repository-to-registry synchronization behavior and endpoint generation.
type RegistrySyncConfig = registrysync.Config

// ValidateStandaloneAASRepositoryRegistrySyncConfig validates standalone AAS repository toggle usage.
func ValidateStandaloneAASRepositoryRegistrySyncConfig(cfg *common.Config) error {
	if cfg == nil {
		return common.NewErrBadRequest("AASENV-REGSYNCCFG-NILCFG configuration must not be nil")
	}
	if cfg.General.SubmodelRegistryIntegration {
		return common.NewErrBadRequest("AASENV-REGSYNCCFG-AASREPO-INVALIDTOGGLE unsupported standalone toggle: general.submodelRegistryIntegration=true")
	}
	return nil
}

// ValidateStandaloneSubmodelRepositoryRegistrySyncConfig validates standalone submodel repository toggle usage.
func ValidateStandaloneSubmodelRepositoryRegistrySyncConfig(cfg *common.Config) error {
	if cfg == nil {
		return common.NewErrBadRequest("AASENV-REGSYNCCFG-NILCFG configuration must not be nil")
	}
	if cfg.General.AASRegistryIntegration {
		return common.NewErrBadRequest("AASENV-REGSYNCCFG-SMREPO-INVALIDTOGGLE unsupported standalone toggle: general.aasRegistryIntegration=true")
	}
	return nil
}

// NewRegistrySyncConfig validates sync-related settings and normalizes configured external base URLs.
func NewRegistrySyncConfig(
	aasRegistryIntegration bool,
	submodelRegistryIntegration bool,
	rawExternalURL string,
) (RegistrySyncConfig, error) {
	return registrysync.NewConfig(aasRegistryIntegration, submodelRegistryIntegration, rawExternalURL)
}

func registrySyncDescriptorsEqual(previous any, submitted any) (bool, error) {
	return registrysync.DescriptorsEqual(previous, submitted)
}

func assetKindPointer(assetKind types.AssetKind) *types.AssetKind {
	copyValue := assetKind
	return &copyValue
}

func readOptionalString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}
