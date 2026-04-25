//go:generate swag init -g main.go -o ../../docs --parseInternal --parseDependency --outputTypes json,yaml
package main

// API documentation metadata for swag generation.
//
// @title LogMonitor API
// @version 1.0.0
// @description REST API for remote log monitoring, collection and integrity control.
// @BasePath /
// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key
