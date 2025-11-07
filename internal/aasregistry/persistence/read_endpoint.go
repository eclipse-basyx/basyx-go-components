package aasregistrydatabase

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	"github.com/lib/pq"
)

func readEndpointsByDescriptorID(
	ctx context.Context,
	db *sql.DB,
	descriptorID int64,
) ([]model.Endpoint, error) {

	v, err := readEndpointsByDescriptorIDs(ctx, db, []int64{descriptorID})
	return v[descriptorID], err
}
func readEndpointsByDescriptorIDs(
	ctx context.Context,
	db *sql.DB,
	descriptorIDs []int64,
) (map[int64][]model.Endpoint, error) {
	start := time.Now() // ⏱ start timing

	out := make(map[int64][]model.Endpoint, len(descriptorIDs))
	if len(descriptorIDs) == 0 {
		return out, nil
	}

	// Deduplicate descriptor IDs
	uniq := make([]int64, 0, len(descriptorIDs))
	seen := make(map[int64]struct{}, len(descriptorIDs))
	for _, id := range descriptorIDs {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			uniq = append(uniq, id)
		}
	}

	const q = `
    SELECT
        e.descriptor_id,
        e.id,
        COALESCE(e.href, '') AS href,
        COALESCE(e.endpoint_protocol, '') AS endpoint_protocol,
        COALESCE(e.sub_protocol, '') AS sub_protocol,
        COALESCE(e.sub_protocol_body, '') AS sub_protocol_body,
        COALESCE(e.sub_protocol_body_encoding, '') AS sub_protocol_body_encoding,
        COALESCE(e.interface, '') AS interface,
        COALESCE(ARRAY_AGG(v.endpoint_protocol_version
                 ORDER BY v.id) FILTER (WHERE v.endpoint_protocol_version IS NOT NULL),
                 '{}') AS versions,
        COALESCE(JSON_AGG(JSON_BUILD_OBJECT(
                 'type', s.security_type,
                 'key', s.security_key,
                 'value', s.security_value
               ) ORDER BY s.id)
               FILTER (WHERE s.security_type IS NOT NULL), '[]') AS sec_attrs
    FROM aas_descriptor_endpoint e
    LEFT JOIN endpoint_protocol_version v
           ON v.endpoint_id = e.id
    LEFT JOIN security_attributes s
           ON s.endpoint_id = e.id
    WHERE e.descriptor_id = ANY($1)
    GROUP BY e.descriptor_id, e.id, e.href, e.endpoint_protocol,
             e.sub_protocol, e.sub_protocol_body, e.sub_protocol_body_encoding, e.interface
    ORDER BY e.descriptor_id ASC, e.id ASC;
    `

	rows, err := db.QueryContext(ctx, q, pq.Array(uniq))
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	type secAttr struct {
		Type  string `json:"type"`
		Key   string `json:"key"`
		Value string `json:"value"`
	}

	for rows.Next() {
		var (
			descID, endpointID                                int64
			href, proto, subProto, subBody, subBodyEnc, iface string
			versions                                          pq.StringArray
			secJSON                                           []byte
		)
		if err := rows.Scan(
			&descID, &endpointID,
			&href, &proto, &subProto, &subBody, &subBodyEnc, &iface,
			&versions, &secJSON,
		); err != nil {
			return nil, err
		}

		var secAttrs []secAttr
		if err := json.Unmarshal(secJSON, &secAttrs); err != nil {
			return nil, err
		}

		converted := make([]model.ProtocolInformationSecurityAttributes, len(secAttrs))
		for i, a := range secAttrs {
			converted[i] = model.ProtocolInformationSecurityAttributes{
				Type:  a.Type,
				Key:   a.Key,
				Value: a.Value,
			}
		}

		out[descID] = append(out[descID], model.Endpoint{
			Interface: iface,
			ProtocolInformation: model.ProtocolInformation{
				Href:                    href,
				EndpointProtocol:        proto,
				Subprotocol:             subProto,
				SubprotocolBody:         subBody,
				SubprotocolBodyEncoding: subBodyEnc,
				EndpointProtocolVersion: []string(versions),
				SecurityAttributes:      converted,
			},
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Ensure keys for all requested descriptors
	for _, id := range uniq {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}

	// ⏱ print time taken
	duration := time.Since(start)
	fmt.Printf("endpoint block took %v to complete\n", duration)

	return out, nil
}
