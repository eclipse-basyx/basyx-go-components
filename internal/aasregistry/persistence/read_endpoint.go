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
