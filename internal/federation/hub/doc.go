// Package hub is the immerle-hub instance-facing API client. The wire types in
// types.gen.go are generated from the hub's OpenAPI spec (openapi.json, a vendored
// copy of internal/api/docs/swagger.json from the immerle-hub repo); client.go
// wraps them with the handful of calls an instance makes (bootstrap, register,
// self-update, playlist sync, ingest, spotify import).
//
// Regenerate the types after updating openapi.json:
//
//	go generate ./internal/federation/hub
package hub

//go:generate go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -generate types -package hub -o types.gen.go openapi.json
