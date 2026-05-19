package asyncbulk

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

var ownerClaimKeys = []string{"iss", "sub", "azp", "client_id", "Edc-Bpn"}

// OwnerKeyFromContext builds a stable owner key from available JWT claims.
func OwnerKeyFromContext(ctx context.Context) string {
	return OwnerKeyFromClaims(auth.ClaimsFromContext(ctx))
}

// OwnerKeyFromClaims builds a stable owner key from relevant claim fields.
func OwnerKeyFromClaims(claims auth.Claims) string {
	if len(claims) == 0 {
		return "anonymous"
	}

	parts := make([]string, 0, len(ownerClaimKeys))
	for _, claimKey := range ownerClaimKeys {
		rawValue, found := claims[claimKey]
		if !found {
			continue
		}

		value := stringifyOwnerClaim(rawValue)
		if value == "" {
			continue
		}

		parts = append(parts, fmt.Sprintf("%s=%s", claimKey, value))
	}

	if len(parts) == 0 {
		return "anonymous"
	}

	return strings.Join(parts, "|")
}

func stringifyOwnerClaim(value any) string {
	switch castValue := value.(type) {
	case string:
		return strings.TrimSpace(castValue)
	case json.Number:
		return castValue.String()
	case fmt.Stringer:
		return strings.TrimSpace(castValue.String())
	case []string:
		if len(castValue) == 0 {
			return ""
		}
		return strings.Join(castValue, ",")
	case []any:
		if len(castValue) == 0 {
			return ""
		}
		parts := make([]string, 0, len(castValue))
		for _, entry := range castValue {
			parsedEntry := stringifyOwnerClaim(entry)
			if parsedEntry == "" {
				continue
			}
			parts = append(parts, parsedEntry)
		}
		return strings.Join(parts, ",")
	default:
		return strings.TrimSpace(fmt.Sprint(castValue))
	}
}
