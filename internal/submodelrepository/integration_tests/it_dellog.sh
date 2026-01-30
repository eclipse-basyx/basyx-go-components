#!/bin/bash
rm -r ./internal/submodelrepository/integration_tests/logs
go test -v ./internal/submodelrepository/integration_tests    