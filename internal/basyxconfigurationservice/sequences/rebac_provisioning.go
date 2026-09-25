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
// Author: Aaron Zielstorff ( Fraunhofer IESE )

package sequences

import (
	"fmt"
	"log/slog"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/security/rebac"
)

// ReBACModelProvisioning creates or finds the OpenFGA store of the ReBAC
// scope, writes the embedded authorization model and binds the database to
// both. The step is a no-op unless rebac.provisionModel is enabled. OpenFGA's
// own datastore schema is migrated by `openfga migrate`, not by BaSyx.
type ReBACModelProvisioning struct {
	ctx *ExecutionContext
}

// NewReBACModelProvisioning creates the provisioning step.
func NewReBACModelProvisioning(ctx *ExecutionContext) *ReBACModelProvisioning {
	return &ReBACModelProvisioning{ctx: ctx}
}

// Execute provisions the model when enabled.
func (p *ReBACModelProvisioning) Execute(_ int) (int, error) {
	if p.ctx == nil || p.ctx.Config == nil || !p.ctx.Config.ReBAC.ProvisionModel {
		return 0, nil
	}
	if p.ctx.DB == nil {
		return 1, fmt.Errorf("BASYXCFG-REBAC-NODB: database connection is not initialized")
	}
	activation, err := rebac.Provision(p.ctx.Context, p.ctx.Config.ReBAC, p.ctx.DB, "basyxconfigurationservice")
	if err != nil {
		return 1, fmt.Errorf("BASYXCFG-REBAC-PROVISION: %w", err)
	}
	slog.Info("ReBAC authorization model provisioned",
		"rebac.scope", activation.Scope, "rebac.store_id", activation.StoreID, "rebac.model_id", activation.ModelID)
	return 0, nil
}

// GetDescription returns the step description for console output.
func (p *ReBACModelProvisioning) GetDescription(stepIndex int) string {
	return fmt.Sprintf("[Step %d] Provisioning ReBAC authorization model", stepIndex)
}
