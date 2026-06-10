package smregistrypostgresql

import "testing"

func TestBuildSubmodelDescriptorUpsertLockSQLUsesPostgresPlaceholders(t *testing.T) {
	t.Parallel()

	query, args, err := buildSubmodelDescriptorUpsertLockSQL("submodel-1")

	if err != nil {
		t.Fatalf("buildSubmodelDescriptorUpsertLockSQL returned error: %v", err)
	}
	if query != "SELECT pg_advisory_xact_lock(hashtextextended($1, $2))" {
		t.Fatalf("unexpected query: %s", query)
	}
	if len(args) != 2 || args[0] != "submodel_descriptor:submodel-1" || args[1] != int64(0) {
		t.Fatalf("unexpected args: %#v", args)
	}
}
