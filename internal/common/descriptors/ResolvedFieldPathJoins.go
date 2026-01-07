package descriptors

import (
	"fmt"
	"sort"
	"strings"

	"github.com/doug-martin/goqu/v9"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"
)

type joinRule struct {
	Alias string
	Deps  []string
	Apply func(ds *goqu.SelectDataset) *goqu.SelectDataset
}

// GetJoinTablesForResolvedFieldPath returns a FROM+JOIN dataset containing only the
// tables required to reference the column and array position bindings in the
// provided ResolvedFieldPath.
//
// The join rules and aliases match the conventions used throughout the
// descriptors package (see AASFilterQuery.go).
//
// NOTE:
// - This helper targets descriptor-based query roots (descriptor + aas_descriptor).
// - It always includes the base inner join to aas_descriptor.
func GetJoinTablesForResolvedFieldPath(d goqu.DialectWrapper, resolved grammar.ResolvedFieldPath) (*goqu.SelectDataset, error) {
	// Base join set for AAS descriptor queries.
	ds := d.From(tDescriptor).
		InnerJoin(
			tAASDescriptor,
			goqu.On(tAASDescriptor.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
		)

	requiredAliases, err := requiredAliasesFromResolvedFieldPath(resolved)
	if err != nil {
		return nil, err
	}

	// If nothing is required (e.g. empty fragment), base joins are still useful.
	if len(requiredAliases) == 0 {
		return ds, nil
	}

	rules := joinRulesForAASDescriptors()
	applied := map[string]struct{}{}
	visiting := map[string]struct{}{}

	var ensure func(alias string) error
	ensure = func(alias string) error {
		if alias == "" {
			return nil
		}
		// Base tables are always available.
		if alias == tblDescriptor || alias == tblAASDescriptor {
			return nil
		}
		if _, ok := applied[alias]; ok {
			return nil
		}
		if _, ok := visiting[alias]; ok {
			return fmt.Errorf("cyclic join dependency for alias %q", alias)
		}

		rule, ok := rules[alias]
		if !ok {
			return fmt.Errorf("no join rule registered for alias %q", alias)
		}

		visiting[alias] = struct{}{}
		for _, dep := range rule.Deps {
			if err := ensure(dep); err != nil {
				return err
			}
		}
		delete(visiting, alias)

		ds = rule.Apply(ds)
		applied[alias] = struct{}{}
		return nil
	}

	// Apply required aliases in a deterministic order (helps testing and debugging).
	aliases := make([]string, 0, len(requiredAliases))
	for a := range requiredAliases {
		aliases = append(aliases, a)
	}
	sort.Strings(aliases)

	for _, alias := range aliases {
		if err := ensure(alias); err != nil {
			return nil, err
		}
	}

	return ds, nil
}

func joinRulesForAASDescriptors() map[string]joinRule {
	return map[string]joinRule{
		// specific_asset_id
		aliasSpecificAssetID: {
			Alias: aliasSpecificAssetID,
			Deps:  nil,
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.LeftJoin(
					specificAssetIDAlias,
					goqu.On(specificAssetIDAlias.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
				)
			},
		},

		// external_subject_reference (reference)
		aliasExternalSubjectReference: {
			Alias: aliasExternalSubjectReference,
			Deps:  []string{aliasSpecificAssetID},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.LeftJoin(
					externalSubjectReferenceAlias,
					goqu.On(externalSubjectReferenceAlias.Col(colID).Eq(specificAssetIDAlias.Col(colExternalSubjectRef))),
				)
			},
		},

		// external_subject_reference_key (reference_key)
		aliasExternalSubjectReferenceKey: {
			Alias: aliasExternalSubjectReferenceKey,
			Deps:  []string{aliasExternalSubjectReference},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.LeftJoin(
					externalSubjectReferenceKeyAlias,
					goqu.On(externalSubjectReferenceKeyAlias.Col(colReferenceID).Eq(externalSubjectReferenceAlias.Col(colID))),
				)
			},
		},

		// aas_descriptor_endpoint
		aliasAASDescriptorEndpoint: {
			Alias: aliasAASDescriptorEndpoint,
			Deps:  nil,
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.LeftJoin(
					aasDescriptorEndpointAlias,
					goqu.On(aasDescriptorEndpointAlias.Col(colDescriptorID).Eq(tDescriptor.Col(colID))),
				)
			},
		},

		// submodel_descriptor
		aliasSubmodelDescriptor: {
			Alias: aliasSubmodelDescriptor,
			Deps:  nil,
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.LeftJoin(
					submodelDescriptorAlias,
					goqu.On(submodelDescriptorAlias.Col(colAASDescriptorID).Eq(tAASDescriptor.Col(colDescriptorID))),
				)
			},
		},

		// submodel_descriptor_endpoint (same underlying table as aas_descriptor_endpoint)
		aliasSubmodelDescriptorEndpoint: {
			Alias: aliasSubmodelDescriptorEndpoint,
			Deps:  []string{aliasSubmodelDescriptor},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.LeftJoin(
					submodelDescriptorEndpointAlias,
					goqu.On(submodelDescriptorEndpointAlias.Col(colDescriptorID).Eq(submodelDescriptorAlias.Col(colDescriptorID))),
				)
			},
		},

		// aasdesc_submodel_descriptor_semantic_id_reference
		aliasSubmodelDescriptorSemanticIDReference: {
			Alias: aliasSubmodelDescriptorSemanticIDReference,
			Deps:  []string{aliasSubmodelDescriptor},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.LeftJoin(
					submodelDescriptorSemanticIDReferenceAlias,
					goqu.On(submodelDescriptorSemanticIDReferenceAlias.Col(colID).Eq(submodelDescriptorAlias.Col(colSemanticID))),
				)
			},
		},

		// aasdesc_submodel_descriptor_semantic_id_reference_key
		aliasSubmodelDescriptorSemanticIDReferenceKey: {
			Alias: aliasSubmodelDescriptorSemanticIDReferenceKey,
			Deps:  []string{aliasSubmodelDescriptorSemanticIDReference},
			Apply: func(ds *goqu.SelectDataset) *goqu.SelectDataset {
				return ds.LeftJoin(
					submodelDescriptorSemanticIDReferenceKeyAlias,
					goqu.On(submodelDescriptorSemanticIDReferenceKeyAlias.Col(colReferenceID).Eq(submodelDescriptorSemanticIDReferenceAlias.Col(colID))),
				)
			},
		},
	}
}

func requiredAliasesFromResolvedFieldPath(resolved grammar.ResolvedFieldPath) (map[string]struct{}, error) {
	required := map[string]struct{}{}

	// From column expression.
	if strings.TrimSpace(resolved.Column) != "" {
		a, ok := leadingAlias(resolved.Column)
		if !ok {
			return nil, fmt.Errorf("cannot extract table alias from column expression %q", resolved.Column)
		}
		required[a] = struct{}{}
	}

	// From array binding position aliases (alias.position).
	for _, b := range resolved.ArrayBindings {
		a, ok := leadingAlias(b.Alias)
		if !ok {
			return nil, fmt.Errorf("cannot extract table alias from array binding alias %q", b.Alias)
		}
		required[a] = struct{}{}
	}

	// The descriptor root queries always have aas_descriptor available via the base join.
	delete(required, tblDescriptor)
	delete(required, tblAASDescriptor)

	return required, nil
}

// leadingAlias returns the identifier prefix before the first '.' character.
//
// Examples:
//   - "specific_asset_id.value" -> "specific_asset_id", true
//   - "specific_asset_id.position" -> "specific_asset_id", true
func leadingAlias(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", false
	}
	idx := strings.Index(expr, ".")
	if idx <= 0 {
		return "", false
	}
	return expr[:idx], true
}
