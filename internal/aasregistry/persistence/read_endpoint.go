package persistence_postgresql

import (
	"context"
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func readEndpointsByDescriptorID(
	ctx context.Context,
	db *sql.DB,
	descriptorID int64,
) ([]model.Endpoint, error) {
	d := goqu.Dialect(dialect)

	e := goqu.T(tblAASDescriptorEndpoint).As("e")

	sqlStr, args, err := d.
		From(e).
		Select(
			e.Col(colID),
			e.Col(colHref),
			e.Col(colEndpointProtocol),
			e.Col(colSubProtocol),
			e.Col(colSubProtocolBody),
			e.Col(colSubProtocolBodyEncoding),
			e.Col(colInterface),
		).
		Where(e.Col(colDescriptorID).Eq(descriptorID)).
		Order(e.Col(colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rowData struct {
		id                            int64
		href, proto, subProto         sql.NullString
		subProtoBody, subProtoBodyEnc sql.NullString
		iface                         sql.NullString
	}
	var results []model.Endpoint

	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.id,
			&r.href,
			&r.proto,
			&r.subProto,
			&r.subProtoBody,
			&r.subProtoBodyEnc,
			&r.iface,
		); err != nil {
			return nil, err
		}

		versions, err := readEndpointProtocolVersions(ctx, db, r.id)
		if err != nil {
			return nil, err
		}
		secAttrs, err := readEndpointSecurityAttributes(ctx, db, r.id)
		if err != nil {
			return nil, err
		}

		results = append(results, model.Endpoint{
			Interface: r.iface.String,
			ProtocolInformation: model.ProtocolInformation{
				Href:                    r.href.String,
				EndpointProtocol:        r.proto.String,
				Subprotocol:             r.subProto.String,
				SubprotocolBody:         r.subProtoBody.String,
				SubprotocolBodyEncoding: r.subProtoBodyEnc.String,
				EndpointProtocolVersion: versions,
				SecurityAttributes:      secAttrs,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func readEndpointProtocolVersions(
	ctx context.Context,
	db *sql.DB,
	endpointID int64,
) ([]string, error) {
	d := goqu.Dialect(dialect)

	v := goqu.T(tblEndpointProtocolVersion).As("v")

	sqlStr, args, err := d.
		From(v).
		Select(v.Col(colEndpointProtocolVersion)).
		Where(v.Col(colEndpointID).Eq(endpointID)).
		Order(v.Col(colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var s sql.NullString
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		if s.Valid {
			out = append(out, s.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func readEndpointSecurityAttributes(
	ctx context.Context,
	db *sql.DB,
	endpointID int64,
) ([]model.ProtocolInformationSecurityAttributes, error) {
	d := goqu.Dialect(dialect)

	s := goqu.T(tblSecurityAttributes).As("s")

	sqlStr, args, err := d.
		From(s).
		Select(
			s.Col(colSecurityType),
			s.Col(colSecurityKey),
			s.Col(colSecurityValue),
		).
		Where(s.Col(colEndpointID).Eq(endpointID)).
		Order(s.Col(colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []model.ProtocolInformationSecurityAttributes
	for rows.Next() {
		var (
			typ, key, val sql.NullString
		)
		if err := rows.Scan(&typ, &key, &val); err != nil {
			return nil, err
		}
		out = append(out, model.ProtocolInformationSecurityAttributes{
			Type:  typ.String,
			Key:   key.String,
			Value: val.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
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

	// dedupe descriptor IDs
	uniqDesc := dedupeInt64(descriptorIDs)

	d := goqu.Dialect(dialect)
	e := goqu.T(tblAASDescriptorEndpoint).As("e")

	// Pull ALL endpoints for the requested descriptors in one go.
	// Include descriptor_id to group later.
	sqlStr, args, err := d.
		From(e).
		Select(
			e.Col(colDescriptorID),            // 0
			e.Col(colID),                      // 1
			e.Col(colHref),                    // 2
			e.Col(colEndpointProtocol),        // 3
			e.Col(colSubProtocol),             // 4
			e.Col(colSubProtocolBody),         // 5
			e.Col(colSubProtocolBodyEncoding), // 6
			e.Col(colInterface),               // 7
		).
		Where(e.Col(colDescriptorID).In(uniqDesc)).
		Order(
			e.Col(colDescriptorID).Asc(),
			e.Col(colID).Asc(),
		).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rowData struct {
		descID                        int64
		endpointID                    int64
		href, proto, subProto         sql.NullString
		subProtoBody, subProtoBodyEnc sql.NullString
		iface                         sql.NullString
	}

	// Gather endpoints & endpointIDs for batch sub-queries
	endpointsPerDesc := make(map[int64][]rowData, len(uniqDesc))
	allEndpointIDs := make([]int64, 0, 256)

	for rows.Next() {
		var r rowData
		if err := rows.Scan(
			&r.descID,
			&r.endpointID,
			&r.href,
			&r.proto,
			&r.subProto,
			&r.subProtoBody,
			&r.subProtoBodyEnc,
			&r.iface,
		); err != nil {
			return nil, err
		}
		endpointsPerDesc[r.descID] = append(endpointsPerDesc[r.descID], r)
		allEndpointIDs = append(allEndpointIDs, r.endpointID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Nothing found
	if len(allEndpointIDs) == 0 {
		// Ensure keys exist for requested descriptors (optional)
		for _, id := range uniqDesc {
			if _, ok := out[id]; !ok {
				out[id] = nil
			}
		}
		return out, nil
	}

	uniqEndpointIDs := dedupeInt64(allEndpointIDs)

	// ---- Bulk fetch versions and security attributes ----
	versionsByEP, err := readEndpointProtocolVersionsByEndpointIDs(ctx, db, uniqEndpointIDs)
	if err != nil {
		return nil, err
	}
	secAttrsByEP, err := readEndpointSecurityAttributesByEndpointIDs(ctx, db, uniqEndpointIDs)
	if err != nil {
		return nil, err
	}

	// ---- Assemble output in stable order ----
	for descID, rowsForDesc := range endpointsPerDesc {
		// rowsForDesc already sorted by endpoint id due to ORDER BY above
		for _, r := range rowsForDesc {
			out[descID] = append(out[descID], model.Endpoint{
				Interface: r.iface.String,
				ProtocolInformation: model.ProtocolInformation{
					Href:                    r.href.String,
					EndpointProtocol:        r.proto.String,
					Subprotocol:             r.subProto.String,
					SubprotocolBody:         r.subProtoBody.String,
					SubprotocolBodyEncoding: r.subProtoBodyEnc.String,
					EndpointProtocolVersion: versionsByEP[r.endpointID],
					SecurityAttributes:      secAttrsByEP[r.endpointID],
				},
			})
		}
	}

	// Ensure keys for requested descriptors (optional)
	for _, id := range uniqDesc {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	return out, nil
}

// ------------- Bulk helpers (endpointIDs -> versions / security) -------------

func readEndpointProtocolVersionsByEndpointIDs(
	ctx context.Context,
	db *sql.DB,
	endpointIDs []int64,
) (map[int64][]string, error) {
	out := make(map[int64][]string, len(endpointIDs))
	if len(endpointIDs) == 0 {
		return out, nil
	}
	uniq := dedupeInt64(endpointIDs)

	d := goqu.Dialect(dialect)
	v := goqu.T(tblEndpointProtocolVersion).As("v")

	sqlStr, args, err := d.
		From(v).
		Select(
			v.Col(colEndpointID),              // 0
			v.Col(colEndpointProtocolVersion), // 1
		).
		Where(v.Col(colEndpointID).In(uniq)).
		Order(v.Col(colEndpointID).Asc(), v.Col(colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			eid int64
			ver sql.NullString
		)
		if err := rows.Scan(&eid, &ver); err != nil {
			return nil, err
		}
		if ver.Valid {
			out[eid] = append(out[eid], ver.String)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Ensure keys exist (optional)
	for _, id := range uniq {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	return out, nil
}

func readEndpointSecurityAttributesByEndpointIDs(
	ctx context.Context,
	db *sql.DB,
	endpointIDs []int64,
) (map[int64][]model.ProtocolInformationSecurityAttributes, error) {
	out := make(map[int64][]model.ProtocolInformationSecurityAttributes, len(endpointIDs))
	if len(endpointIDs) == 0 {
		return out, nil
	}
	uniq := dedupeInt64(endpointIDs)

	d := goqu.Dialect(dialect)
	s := goqu.T(tblSecurityAttributes).As("s")

	sqlStr, args, err := d.
		From(s).
		Select(
			s.Col(colEndpointID),    // 0
			s.Col(colSecurityType),  // 1
			s.Col(colSecurityKey),   // 2
			s.Col(colSecurityValue), //3
		).
		Where(s.Col(colEndpointID).In(uniq)).
		Order(s.Col(colEndpointID).Asc(), s.Col(colID).Asc()).
		ToSQL()
	if err != nil {
		return nil, err
	}

	rows, err := db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			eid           int64
			typ, key, val sql.NullString
		)
		if err := rows.Scan(&eid, &typ, &key, &val); err != nil {
			return nil, err
		}
		out[eid] = append(out[eid], model.ProtocolInformationSecurityAttributes{
			Type:  typ.String,
			Key:   key.String,
			Value: val.String,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Ensure keys exist (optional)
	for _, id := range uniq {
		if _, ok := out[id]; !ok {
			out[id] = nil
		}
	}
	return out, nil
}

// ------------- Small utilities -------------

func dedupeInt64(ids []int64) []int64 {
	if len(ids) == 0 {
		return ids
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
