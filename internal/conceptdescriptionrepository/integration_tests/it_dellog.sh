#!/bin/bash
rm -r ./internal/conceptdescriptionrepository/integration_tests/logs
go test -v ./internal/conceptdescriptionrepository/integration_tests
