package descriptors

import (
	"context"
	"testing"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func TestBuildAASDescriptorInsertRecord_DoesNotWriteCreatedAtByDefault(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2024, time.January, 10, 15, 30, 0, 0, time.UTC)
	record := buildAASDescriptorInsertRecord(
		context.Background(),
		42,
		model.AssetAdministrationShellDescriptor{
			Id:        "aas-id",
			CreatedAt: &createdAt,
		},
	)

	if _, ok := record[common.ColCreatedAt]; ok {
		t.Fatalf("expected %q to be absent without override context", common.ColCreatedAt)
	}
}

func TestBuildAASDescriptorInsertRecord_WritesCreatedAtWhenOverrideEnabled(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2024, time.January, 10, 15, 30, 0, 0, time.UTC)
	ctx := WithAllowAASDescriptorCreatedAtOverride(context.Background())
	record := buildAASDescriptorInsertRecord(
		ctx,
		42,
		model.AssetAdministrationShellDescriptor{
			Id:        "aas-id",
			CreatedAt: &createdAt,
		},
	)

	got, ok := record[common.ColCreatedAt]
	if !ok {
		t.Fatalf("expected %q to be present when override context is enabled", common.ColCreatedAt)
	}

	gotTime, ok := got.(time.Time)
	if !ok {
		t.Fatalf("expected %q value to be time.Time, got %T", common.ColCreatedAt, got)
	}
	if !gotTime.Equal(createdAt) {
		t.Fatalf("expected createdAt %v, got %v", createdAt, gotTime)
	}
}

func TestBuildAASDescriptorInsertRecord_DoesNotWriteCreatedAtWhenMissing(t *testing.T) {
	t.Parallel()

	ctx := WithAllowAASDescriptorCreatedAtOverride(context.Background())
	record := buildAASDescriptorInsertRecord(
		ctx,
		42,
		model.AssetAdministrationShellDescriptor{
			Id: "aas-id",
		},
	)

	if _, ok := record[common.ColCreatedAt]; ok {
		t.Fatalf("expected %q to be absent when payload createdAt is nil", common.ColCreatedAt)
	}
}
