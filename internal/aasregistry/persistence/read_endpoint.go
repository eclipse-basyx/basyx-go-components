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

	sqlStr, args, err := d.
		From(goqu.T(tblAASDescriptorEndpoint).As("e")).
		Select(
			goqu.I("e."+colID),
			goqu.I("e."+colHref),
			goqu.I("e."+colEndpointProtocol),
			goqu.I("e."+colSubProtocol),
			goqu.I("e."+colSubProtocolBody),
			goqu.I("e."+colSubProtocolBodyEncoding),
			goqu.I("e."+colInterface),
		).
		Where(goqu.I("e." + colDescriptorID).Eq(descriptorID)).
		Order(goqu.I("e." + colID).Asc()).
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

	sqlStr, args, err := d.
		From(goqu.T(tblEndpointProtocolVersion).As("v")).
		Select(goqu.I("v." + colEndpointProtocolVersion)).
		Where(goqu.I("v." + colEndpointID).Eq(endpointID)).
		Order(goqu.I("v." + colID).Asc()).
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
		var v sql.NullString
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		if v.Valid {
			out = append(out, v.String)
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

	sqlStr, args, err := d.
		From(goqu.T(tblSecurityAttributes).As("s")).
		Select(
			goqu.I("s."+colSecurityType),
			goqu.I("s."+colSecurityKey),
			goqu.I("s."+colSecurityValue),
		).
		Where(goqu.I("s." + colEndpointID).Eq(endpointID)).
		Order(goqu.I("s." + colID).Asc()).
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
