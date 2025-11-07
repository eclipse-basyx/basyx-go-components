package aasregistrydatabase

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/doug-martin/goqu/v9"
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

	d := goqu.Dialect("postgres")

	// Build the SQL with goqu; keep expressions as literals where it’s simpler
	ds := d.
		From(goqu.T("aas_descriptor_endpoint").As("e")).
		LeftJoin(
			goqu.T("endpoint_protocol_version").As("v"),
			goqu.On(goqu.I("v.endpoint_id").Eq(goqu.I("e.id"))),
		).
		LeftJoin(
			goqu.T("security_attributes").As("s"),
			goqu.On(goqu.I("s.endpoint_id").Eq(goqu.I("e.id"))),
		).
		Where(goqu.I("e.descriptor_id").In(uniq)).
		Select(
			goqu.I("e.descriptor_id"),
			goqu.I("e.id"),
			goqu.L(`COALESCE(e.href, '')`).As("href"),
			goqu.L(`COALESCE(e.endpoint_protocol, '')`).As("endpoint_protocol"),
			goqu.L(`COALESCE(e.sub_protocol, '')`).As("sub_protocol"),
			goqu.L(`COALESCE(e.sub_protocol_body, '')`).As("sub_protocol_body"),
			goqu.L(`COALESCE(e.sub_protocol_body_encoding, '')`).As("sub_protocol_body_encoding"),
			goqu.L(`COALESCE(e.interface, '')`).As(`interface`),

			// versions
			goqu.L(
				`COALESCE(ARRAY_AGG(v.endpoint_protocol_version ORDER BY v.id)
                  FILTER (WHERE v.endpoint_protocol_version IS NOT NULL), '{}')`,
			).As("versions"),

			// sec_attrs
			goqu.L(
				`COALESCE(JSON_AGG(JSON_BUILD_OBJECT(
                    'type', s.security_type,
                    'key', s.security_key,
                    'value', s.security_value
                  ) ORDER BY s.id)
                  FILTER (WHERE s.security_type IS NOT NULL), '[]')`,
			).As("sec_attrs"),
		).
		GroupBy(
			goqu.I("e.descriptor_id"),
			goqu.I("e.id"),
			goqu.I("e.href"),
			goqu.I("e.endpoint_protocol"),
			goqu.I("e.sub_protocol"),
			goqu.I("e.sub_protocol_body"),
			goqu.I("e.sub_protocol_body_encoding"),
			goqu.I("e.interface"),
		).
		Order(
			goqu.I("e.descriptor_id").Asc(),
			goqu.I("e.id").Asc(),
		).
		Prepared(true)

	sqlStr, args, err := ds.ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

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

	for _, id := range uniq {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}

	return out, nil
}
