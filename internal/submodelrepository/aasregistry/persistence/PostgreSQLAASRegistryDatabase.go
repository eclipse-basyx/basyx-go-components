package persistence_postgresql

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/eclipse-basyx/basyx-go-components/internal/common"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
	persistence_utils "github.com/eclipse-basyx/basyx-go-components/internal/submodelrepository/persistence/utils"
	_ "github.com/lib/pq" // PostgreSQL Treiber
)

type PostgreSQLAASRegistryDatabase struct {
	db           *sql.DB
	cacheEnabled bool
}

func NewPostgreSQLAASRegistryDatabase(dsn string, maxOpenConns, maxIdleConns int, connMaxLifetimeMinutes int, cacheEnabled bool) (*PostgreSQLAASRegistryDatabase, error) {
	db, err := sql.Open("postgres", dsn)
	//Set Max Connection
	db.SetMaxOpenConns(500)
	db.SetMaxIdleConns(500)
	db.SetConnMaxLifetime(time.Minute * 5)

	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}

	dir, osErr := os.Getwd()

	if osErr != nil {
		return nil, osErr
	}

	queryString, fileError := os.ReadFile(dir + "/resources/sql/aasregistryschema.sql")

	if fileError != nil {
		return nil, fileError
	}

	_, dbError := db.Exec(string(queryString))

	if dbError != nil {
		return nil, dbError
	}

	return &PostgreSQLAASRegistryDatabase{db: db, cacheEnabled: cacheEnabled}, nil
}

func (p *PostgreSQLAASRegistryDatabase) InsertAdministrationShellDescriptor(ctx context.Context, aasd model.AssetAdministrationShellDescriptor) error {

	tx, err := p.db.BeginTx(ctx, nil)

	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to start postgres transaction. See console for information.")
	}
	defer func() {
		if p := recover(); p != nil {
			tx.Rollback()
			panic(p) // re-throw panic after rollback
		} else if err != nil {
			tx.Rollback()
		}
	}()

	var descriptorId int64

	err = tx.QueryRow(`
		INSERT INTO descriptor DEFAULT VALUES returning id;
	`,
	).Scan(&descriptorId)
	if err != nil {
		return err
	}
	desc := aasd.Description

	fmt.Println(desc)

	var displayNameId, descriptionId, administrationId sql.NullInt64
	displayNameId, err = persistence_utils.CreateLangStringNameTypes(tx, aasd.DisplayName)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
	}

	// Handle possibly nil Description
	descriptionId, err = persistence_utils.CreateLangStringTextTypes(tx, aasd.Description)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
	}

	administrationId, err = persistence_utils.CreateAdministrativeInformation(tx, &aasd.Administration)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
	}

	fmt.Println(displayNameId)
	fmt.Println(descriptionId)
	fmt.Println(administrationId)

	_, err = tx.Exec(`
		INSERT INTO aas_descriptor (
		descriptor_id,
		description_id,
		displayname_id,
		administrative_information_id,
		asset_kind,
		asset_type,
		globalAssetId,
		id_short,
		id
		) VALUES (
		$1, $2, $3, $4, $5, $6, $7, $8, $9
		)
	`,
		descriptorId,
		descriptionId,
		displayNameId,
		administrationId,
		aasd.AssetKind,
		aasd.AssetType,
		aasd.GlobalAssetId,
		aasd.IdShort,
		aasd.Id,
	)

	if err != nil {
		return err
	}

	err = CreateEndpoints(tx, descriptorId, aasd.Endpoints)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Endpoints - no changes applied - see console for details")
	}

	err = CreateSpecificAssetId(tx, descriptorId, aasd.SpecificAssetIds)
	if err != nil {
		fmt.Println(err)
		return common.NewInternalServerError("Failed to create Specific Asset Ids - no changes applied - see console for details")
	}

	err = CreateExtensions(tx, descriptorId, aasd.Extensions)
	if err != nil {
		return err
	}

	err = CreateSubModelDescriptors(tx, descriptorId, aasd.SubmodelDescriptors)
	if err != nil {
		return err
	}
	/*
		if err = tx.Commit(); err != nil {
			return err
		}
	*/

	return nil
}
func CreateEndpointAttributes(tx *sql.Tx, endpointId int64, securityAttributes []model.ProtocolInformationSecurityAttributes) error {
	if securityAttributes == nil {
		return nil
	}

	if len(securityAttributes) > 0 {
		var err error
		for _, val := range securityAttributes {
			_, err = tx.Exec(`
				INSERT INTO security_attributes (
				endpoint_id,
				securityType,
				securityKey,
				securityValue
				) VALUES (
				$1, $2, $3, $4
				)
			`,
				endpointId,
				val.Type,
				val.Key,
				val.Value,
			)

			if err != nil {
				return err
			}
		}

	}

	return nil

}

func CreateEndpointProtocolVersion(tx *sql.Tx, endpointId int64, endpointProtocolVersion []string) error {
	if endpointProtocolVersion == nil {
		return nil
	}

	if len(endpointProtocolVersion) > 0 {
		var err error
		for _, val := range endpointProtocolVersion {
			_, err = tx.Exec(`
				INSERT INTO endpoint_protocol_version (
				endpoint_id,
				endpoint_protocol_version
				) VALUES (
				$1, $2
				)
			`,
				endpointId,
				val,
			)

			if err != nil {
				return err
			}
		}

	}

	return nil

}

func CreateEndpoints(tx *sql.Tx, descriptorId int64, endpoints []model.Endpoint) error {
	if endpoints == nil {
		return nil
	}

	if len(endpoints) > 0 {
		var err error

		for _, val := range endpoints {
			var id int64
			err = tx.QueryRow(`
				INSERT INTO aas_descriptor_endpoint (
					descriptor_id,
					href,
					endpoint_protocol,
					sub_protocol,
					sub_protocol_body,
					sub_protocol_body_encoding,
					interface
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7
				)
				RETURNING id
			`,
				descriptorId,
				val.ProtocolInformation.Href,
				val.ProtocolInformation.EndpointProtocol,
				val.ProtocolInformation.Subprotocol,
				val.ProtocolInformation.SubprotocolBody,
				val.ProtocolInformation.SubprotocolBodyEncoding,
				val.Interface,
			).Scan(&id)

			if err != nil {
				return err
			}
			err = CreateEndpointProtocolVersion(tx, id, val.ProtocolInformation.EndpointProtocolVersion)
			if err != nil {
				return err
			}
			err = CreateEndpointAttributes(tx, id, val.ProtocolInformation.SecurityAttributes)
			if err != nil {
				return err
			}
		}

	}
	return nil
}

func CreateSpecificAssetIdSupplementalSemantic(tx *sql.Tx, specificAssetId int64, references []model.Reference) error {
	if references == nil {
		return nil
	}

	if len(references) > 0 {
		var err error
		for _, val := range references {
			var referenceID sql.NullInt64
			referenceID, err = persistence_utils.CreateReference(tx, &val)

			if err != nil {
				return err
			}
			_, err = tx.Exec(`
				INSERT INTO specific_asset_id_supplemental_semantic_id (
				specific_asset_id_id,
				reference_id
				) VALUES (
				$1, $2
				)
			`,
				specificAssetId,
				referenceID,
			)

			if err != nil {
				return err
			}
		}

	}

	return nil

}

func CreateSpecificAssetId(tx *sql.Tx, descriptorId int64, specificAssetIds []model.SpecificAssetId) error {
	if specificAssetIds == nil {
		return nil
	}

	if len(specificAssetIds) > 0 {
		var err error

		for _, val := range specificAssetIds {

			var externalSubjectReferenceId sql.NullInt64
			externalSubjectReferenceId, err = persistence_utils.CreateReference(tx, val.ExternalSubjectId)

			if err != nil {
				return err
			}
			// hasSemantics
			var semanticId sql.NullInt64
			semanticId, err = persistence_utils.CreateReference(tx, val.SemanticId)

			if err != nil {
				return err
			}
			var id int64
			err = tx.QueryRow(`
				INSERT INTO specific_asset_id (
					descriptor_id,
					semantic_id,
					name,
					value,
					external_subject_ref
				) VALUES (
					$1, $2, $3, $4, $5
				)
				RETURNING id
			`,
				descriptorId,
				semanticId,
				val.Name,
				val.Value,
				externalSubjectReferenceId,
			).Scan(&id)

			if err != nil {
				return err
			}
			err = CreateSpecificAssetIdSupplementalSemantic(tx, id, val.SupplementalSemanticIds)
			if err != nil {
				return err
			}

		}

	}
	return nil
}

func CreatesubModelDescriptorSupplementalSemantic(tx *sql.Tx, subModelDescriptorId int64, references []model.Reference) error {
	if references == nil {
		return nil
	}

	if len(references) > 0 {
		var err error
		for _, val := range references {
			var referenceID sql.NullInt64
			referenceID, err = persistence_utils.CreateReference(tx, &val)

			if err != nil {
				return err
			}
			_, err = tx.Exec(`
				INSERT INTO submodel_descriptor_supplemental_semantic_id (
				descriptor_id,
				reference_id
				) VALUES (
				$1, $2
				)
			`,
				subModelDescriptorId,
				referenceID,
			)

			if err != nil {
				return err
			}
		}

	}

	return nil

}

func CreateSubModelDescriptors(tx *sql.Tx, aasDescriptorId int64, submodelDescriptors []model.SubmodelDescriptor) error {
	if submodelDescriptors == nil {
		return nil
	}

	if len(submodelDescriptors) > 0 {
		var err error

		for _, val := range submodelDescriptors {

			var semanticId, displayNameId, descriptionId, administrationId sql.NullInt64
			displayNameId, err = persistence_utils.CreateLangStringNameTypes(tx, val.DisplayName)
			if err != nil {
				fmt.Println(err)
				return common.NewInternalServerError("Failed to create DisplayName - no changes applied - see console for details")
			}

			// Handle possibly nil Description
			descriptionId, err = persistence_utils.CreateLangStringTextTypes(tx, val.Description)
			if err != nil {
				fmt.Println(err)
				return common.NewInternalServerError("Failed to create Description - no changes applied - see console for details")
			}

			administrationId, err = persistence_utils.CreateAdministrativeInformation(tx, &val.Administration)
			if err != nil {
				fmt.Println(err)
				return common.NewInternalServerError("Failed to create Administration - no changes applied - see console for details")
			}

			semanticId, err = persistence_utils.CreateReference(tx, val.SemanticId)

			if err != nil {
				return err
			}

			var submodelDescriptorId int64

			err = tx.QueryRow(`
				INSERT INTO descriptor DEFAULT VALUES returning id;
			`,
			).Scan(&submodelDescriptorId)
			if err != nil {
				return err
			}
			_, err = tx.Exec(`
				INSERT INTO submodel_descriptor (
					descriptor_id,
					aas_descriptor_id,
					description_id,
					displayname_id,
					administrative_information_id,
					id_short,
					id,
					semantic_id
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8
				)
				RETURNING id
			`,

				aasDescriptorId,
				descriptionId,
				descriptionId,
				displayNameId,
				administrationId,
				val.IdShort,
				val.Id,
				semanticId,
			)

			if err != nil {
				return err
			}
			err = CreatesubModelDescriptorSupplementalSemantic(tx, submodelDescriptorId, val.SupplementalSemanticId)
			if err != nil {
				return err
			}

			err = CreateExtensions(tx, submodelDescriptorId, val.Extensions)
			if err != nil {
				return err
			}

			if len(val.Endpoints) <= 0 {
				return common.NewErrBadRequest("Submodel Descriptor needs at least 1 Endpoint.")
			}
			err = CreateEndpoints(tx, submodelDescriptorId, val.Endpoints)
			if err != nil {
				return err
			}

		}

	}
	return nil
}

func CreateExtensionReferences(tx *sql.Tx, extensionId int64, references []model.Reference) error {
	if references == nil {
		return nil
	}

	if len(references) > 0 {
		var err error
		for _, val := range references {

			var referenceId sql.NullInt64
			referenceId, err = persistence_utils.CreateReference(tx, &val)
			if err != nil {
				return err
			}
			_, err = tx.Exec(`
				INSERT INTO extension_reference (
				extensionId,
				reference_id
				) VALUES (
				$1, $2
				)
			`,
				extensionId,
				referenceId,
			)

			if err != nil {
				return err
			}
		}

	}

	return nil

}

func CreateExtensions(tx *sql.Tx, descriptorId int64, extensions []model.Extension) error {
	if extensions == nil {
		return nil
	}

	if len(extensions) > 0 {
		var err error

		for _, val := range extensions {

			var semanticId sql.NullInt64
			semanticId, err = persistence_utils.CreateReference(tx, val.SemanticId)
			var valueText, valueNum, valueBool, valueTime, valueDatetime, valueType sql.NullString
			valueType = sql.NullString{String: string(val.ValueType), Valid: val.ValueType != ""}
			fillValueBasedOnType(val, &valueText, &valueNum, &valueBool, &valueTime, &valueDatetime)

			if err != nil {
				return err
			}
			var id int64
			fmt.Println(valueText)
			fmt.Println(valueNum)
			fmt.Println(valueBool)
			fmt.Println(valueTime)
			fmt.Println(valueDatetime)
			err = tx.QueryRow(`
				INSERT INTO extension (
					semantic_id,
					name,
					value_type,
					value_text,
					value_num,
					value_bool,
					value_time,
					value_datetime
				) VALUES (
					$1, $2, $3, $4, $5, $6, $7, $8
				)
				RETURNING id
			`,
				semanticId,
				val.Name,
				valueType,
				valueText,
				valueNum,
				valueBool,
				valueTime,
				valueDatetime,
			).Scan(&id)

			if err != nil {
				return err
			}
			err = CreateExtensionReferences(tx, id, val.SupplementalSemanticIds)
			if err != nil {
				return err
			}

			err = CreateExtensionReferences(tx, id, val.RefersTo)
			if err != nil {
				return err
			}

		}

	}
	return nil
}
func fillValueBasedOnType(extension model.Extension, valueText *sql.NullString, valueNum *sql.NullString, valueBool *sql.NullString, valueTime *sql.NullString, valueDatetime *sql.NullString) {
	switch extension.ValueType {
	case "xs:string", "xs:anyURI", "xs:base64Binary", "xs:hexBinary":
		*valueText = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	case "xs:int", "xs:integer", "xs:long", "xs:short", "xs:byte",
		"xs:unsignedInt", "xs:unsignedLong", "xs:unsignedShort", "xs:unsignedByte",
		"xs:positiveInteger", "xs:negativeInteger", "xs:nonNegativeInteger", "xs:nonPositiveInteger",
		"xs:decimal", "xs:double", "xs:float":
		*valueNum = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	case "xs:boolean":
		*valueBool = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	case "xs:time":
		*valueTime = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	case "xs:date", "xs:dateTime", "xs:duration", "xs:gDay", "xs:gMonth",
		"xs:gMonthDay", "xs:gYear", "xs:gYearMonth":
		*valueDatetime = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	default:
		// Fallback to text for unknown types
		*valueText = sql.NullString{String: extension.Value, Valid: extension.Value != ""}
	}
}
