package auth

import "github.com/eclipse-basyx/basyx-go-components/internal/common/model/grammar"

type mapMethodAndPatternToRights struct {
	Method  string
	Pattern string
	Rights  []grammar.RightsEnum
}

var mapMethodAndPatternToRightsData = []mapMethodAndPatternToRights{
	// aas registry
	{"GET", "/shell-descriptors/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/shell-descriptors/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"DELETE", "/shell-descriptors/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/shell-descriptors", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/shell-descriptors", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"GET", "/shell-descriptors/{aasIdentifier}/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"PUT", "/shell-descriptors/{aasIdentifier}/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumUPDATE}},
	{"DELETE", "/shell-descriptors/{aasIdentifier}/submodel-descriptors/{submodelIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
	{"GET", "/shell-descriptors/{aasIdentifier}/submodel-descriptors/", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/shell-descriptors/{aasIdentifier}/submodel-descriptors/", []grammar.RightsEnum{grammar.RightsEnumCREATE}},

	// aas discovery
	{"GET", "/lookup/shells", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/lookup/shellsByAssetLink", []grammar.RightsEnum{grammar.RightsEnumREAD}}, // this is one of the reasons why we need this complex mapping
	{"GET", "/lookup/shells/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumREAD}},
	{"POST", "/lookup/shells/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumCREATE}},
	{"DELETE", "/lookup/shells/{aasIdentifier}", []grammar.RightsEnum{grammar.RightsEnumDELETE}},
}
