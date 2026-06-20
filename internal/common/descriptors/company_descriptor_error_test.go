/*******************************************************************************
* Copyright (C) 2026 the Eclipse BaSyx Authors and Fraunhofer IESE
*
* SPDX-License-Identifier: MIT
******************************************************************************/

package descriptors

import (
	"fmt"
	"testing"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestMapInsertCompanyDescriptorErrorMapsPGXUniqueViolation(t *testing.T) {
	t.Parallel()

	err := mapInsertCompanyDescriptorError(
		fmt.Errorf("insert company: %w", &pgconn.PgError{Code: "23505"}),
	)
	if !common.IsErrConflict(err) {
		t.Fatalf("expected conflict error, got %v", err)
	}
}
