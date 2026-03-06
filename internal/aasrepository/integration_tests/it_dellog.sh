#!/bin/bash
rm -r ./internal/aasrepository/integration_tests/logs
go test -v ./internal/aasrepository/integration_tests
