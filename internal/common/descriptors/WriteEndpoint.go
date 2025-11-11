package descriptors

import (
	"database/sql"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func createEndpointAttributes(tx *sql.Tx, endpointID int64, securityAttributes []model.ProtocolInformationSecurityAttributes) error {
	if len(securityAttributes) == 0 {
		return nil
	}
	d := goqu.Dialect(dialect)
	rows := make([]goqu.Record, 0, len(securityAttributes))
	for _, val := range securityAttributes {
		rows = append(rows, goqu.Record{
			colEndpointID:    endpointID,
			colSecurityType:  val.Type,
			colSecurityKey:   val.Key,
			colSecurityValue: val.Value,
		})
	}
	sqlStr, args, err := d.Insert(tblSecurityAttributes).Rows(rows).ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlStr, args...)
	return err
}

func createEndpointProtocolVersion(tx *sql.Tx, endpointID int64, endpointProtocolVersion []string) error {
	if len(endpointProtocolVersion) == 0 {
		return nil
	}
	d := goqu.Dialect(dialect)
	rows := make([]goqu.Record, 0, len(endpointProtocolVersion))
	for _, val := range endpointProtocolVersion {
		rows = append(rows, goqu.Record{
			colEndpointID:              endpointID,
			colEndpointProtocolVersion: val,
		})
	}
	sqlStr, args, err := d.Insert(tblEndpointProtocolVersion).Rows(rows).ToSQL()
	if err != nil {
		return err
	}
	_, err = tx.Exec(sqlStr, args...)
	return err
}

func createEndpoints(tx *sql.Tx, descriptorID int64, endpoints []model.Endpoint) error {
	if endpoints == nil {
		return nil
	}
	if len(endpoints) > 0 {
		d := goqu.Dialect(dialect)
		for _, val := range endpoints {
			sqlStr, args, err := d.
				Insert(tblAASDescriptorEndpoint).
				Rows(goqu.Record{
					colDescriptorID:            descriptorID,
					colHref:                    val.ProtocolInformation.Href,
					colEndpointProtocol:        val.ProtocolInformation.EndpointProtocol,
					colSubProtocol:             val.ProtocolInformation.Subprotocol,
					colSubProtocolBody:         val.ProtocolInformation.SubprotocolBody,
					colSubProtocolBodyEncoding: val.ProtocolInformation.SubprotocolBodyEncoding,
					colInterface:               val.Interface,
				}).
				Returning(tAASDescriptorEndpoint.Col(colID)).
				ToSQL()
			if err != nil {
				return err
			}
			var id int64
			if err = tx.QueryRow(sqlStr, args...).Scan(&id); err != nil {
				return err
			}
			if err = createEndpointProtocolVersion(tx, id, val.ProtocolInformation.EndpointProtocolVersion); err != nil {
				return err
			}
			if err = createEndpointAttributes(tx, id, val.ProtocolInformation.SecurityAttributes); err != nil {
				return err
			}
		}
	}
	return nil
}
