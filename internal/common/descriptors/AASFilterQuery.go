package descriptors

import (
	"context"

	"github.com/doug-martin/goqu/v9"
	"github.com/doug-martin/goqu/v9/exp"
	auth "github.com/eclipse-basyx/basyx-go-components/internal/common/security"
)

func getFilterQueryFromContext(ctx context.Context, d goqu.DialectWrapper, ds *goqu.SelectDataset, tableCol exp.AliasedExpression) (*goqu.SelectDataset, error) {

	p := auth.GetQueryFilter(ctx)
	if p != nil {

		wc, err := p.Formula.EvaluateToExpression()
		if err != nil {
			return nil, err
		}
		existsDataset := d.
			From(goqu.T(tblDescriptor).As("descriptor")).
			LeftJoin(goqu.T(tblAASDescriptor).As("aas_descriptor"),
				goqu.On(goqu.I("aas_descriptor.descriptor_id").Eq(goqu.I("descriptor.id")))).
			LeftJoin(goqu.T(tblSpecificAssetID).As("specific_asset_id"),
				goqu.On(goqu.I("specific_asset_id.descriptor_id").Eq(goqu.I("descriptor.id")))).
			LeftJoin(goqu.T(tblReference).As("external_subject_reference"),
				goqu.On(goqu.I("external_subject_reference.id").Eq(goqu.I("specific_asset_id.external_subject_ref")))).
			LeftJoin(goqu.T(tblReferenceKey).As("external_subject_reference_key"),
				goqu.On(goqu.I("external_subject_reference_key.reference_id").Eq(goqu.I("external_subject_reference.id")))).
			LeftJoin(goqu.T(tblAASDescriptorEndpoint).As("aas_descriptor_endpoint"),
				goqu.On(goqu.I("aas_descriptor_endpoint.descriptor_id").Eq(goqu.I("descriptor.id")))).
			LeftJoin(goqu.T(tblSubmodelDescriptor).As("submodel_descriptor"),
				goqu.On(goqu.I("submodel_descriptor.aas_descriptor_id").Eq(goqu.I("aas_descriptor.descriptor_id")))).
			LeftJoin(goqu.T(tblAASDescriptorEndpoint).As("submodel_descriptor_endpoint"),
				goqu.On(goqu.I("submodel_descriptor_endpoint.descriptor_id").Eq(goqu.I("submodel_descriptor.descriptor_id")))).
			LeftJoin(goqu.T(tblReference).As("aasdesc_submodel_descriptor_semantic_id_reference"),
				goqu.On(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference.id").Eq(goqu.I("submodel_descriptor.semantic_id")))).
			LeftJoin(goqu.T(tblReferenceKey).As("aasdesc_submodel_descriptor_semantic_id_reference_key"),
				goqu.On(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference_key.reference_id").
					Eq(goqu.I("aasdesc_submodel_descriptor_semantic_id_reference.id")))).
			Where(
				goqu.I("descriptor.id").Eq(tableCol.Col(colDescriptorID)),
				wc,
			)

		ds = ds.Where(
			goqu.L("EXISTS (?)", existsDataset),
		)
	}
	return ds, nil
}
