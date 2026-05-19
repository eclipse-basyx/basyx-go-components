package asyncbulk

import (
	"testing"
	"time"

	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
	"github.com/stretchr/testify/require"
)

func TestStartCreatesOpaqueHandle(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)

	handleID, err := manager.Start("owner-a")
	require.NoError(t, err)
	require.Contains(t, handleID, "ASYNC-TEST-")
	require.NotContains(t, handleID, "|")
}

func TestGetForOwnerHidesForeignHandle(t *testing.T) {
	manager := NewManager("ASYNC-TEST", time.Minute)

	handleID, err := manager.Start("owner-a")
	require.NoError(t, err)

	_, found := manager.GetForOwner(handleID, "owner-b")
	require.False(t, found)

	_, found = manager.GetForOwner(handleID, "owner-a")
	require.True(t, found)
}

func TestOwnerKeyFromClaimsBuildsStableKey(t *testing.T) {
	key := OwnerKeyFromClaims(auth.Claims{
		"iss":     "issuer-a",
		"sub":     "subject-a",
		"Edc-Bpn": "BPNL000000000001",
	})

	require.Equal(t, "iss=issuer-a|sub=subject-a|Edc-Bpn=BPNL000000000001", key)
}
