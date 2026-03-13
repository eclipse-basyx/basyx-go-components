// Package main provides a local utility to inspect delegated operation payload serialization.
package main

import (
	"encoding/json"
	"fmt"

	"github.com/FriedJannik/aas-go-sdk/types"
	"github.com/eclipse-basyx/basyx-go-components/internal/common/model"
)

func main() {
	body := []byte(`{"inputArguments":[{"value":{"idShort":"numberA","modelType":"Property","value":"5","valueType":"xs:int"}},{"value":{"idShort":"numberB","modelType":"Property","value":"0","valueType":"xs:int"}}],"clientTimeoutDuration":"PT60S"}`)
	var req model.OperationRequest
	if err := json.Unmarshal(body, &req); err != nil {
		panic(err)
	}
	payload := make([]types.IOperationVariable, 0, len(req.InputArguments)+len(req.InoutputArguments))
	payload = append(payload, req.InputArguments...)
	payload = append(payload, req.InoutputArguments...)
	out, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		panic(err)
	}
	fmt.Println(string(out))
}
