package server

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@v2.8.0 --config oapi-codegen.yaml openapi/openapi.yaml
//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.30.0 generate -f sqlc.yaml
