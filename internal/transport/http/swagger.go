package httptransport

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>LogMonitor API Docs</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.onload = function() {
      window.ui = SwaggerUIBundle({
        url: '/swagger/openapi.json',
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis],
      });
    };
  </script>
</body>
</html>`

// registerSwagger exposes a lightweight OpenAPI document and Swagger UI.
func registerSwagger(engine *gin.Engine) {
	engine.GET("/swagger", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerUIHTML))
	})
	engine.GET("/swagger/", func(c *gin.Context) {
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(swaggerUIHTML))
	})
	engine.GET("/swagger/openapi.json", func(c *gin.Context) {
		if data, err := os.ReadFile("docs/swagger.json"); err == nil {
			c.Data(http.StatusOK, "application/json; charset=utf-8", data)
			return
		}
		c.JSON(http.StatusOK, openAPISpec())
	})
}

func openAPISpec() gin.H {
	serverSchema := gin.H{
		"type": "object",
		"properties": gin.H{
			"id":            gin.H{"type": "string"},
			"name":          gin.H{"type": "string"},
			"host":          gin.H{"type": "string"},
			"port":          gin.H{"type": "integer"},
			"username":      gin.H{"type": "string"},
			"auth_type":     gin.H{"type": "string"},
			"os_type":       gin.H{"type": "string"},
			"status":        gin.H{"type": "string", "enum": []string{"active", "degraded", "inactive", "error"}},
			"managed_by":    gin.H{"type": "string"},
			"success_count": gin.H{"type": "integer"},
			"failure_count": gin.H{"type": "integer"},
			"last_error":    gin.H{"type": "string"},
			"last_seen_at":  gin.H{"type": "string", "format": "date-time"},
			"backoff_until": gin.H{"type": "string", "format": "date-time"},
			"created_at":    gin.H{"type": "string", "format": "date-time"},
			"updated_at":    gin.H{"type": "string", "format": "date-time"},
		},
	}

	errorSchema := gin.H{
		"type": "object",
		"properties": gin.H{
			"error": gin.H{"type": "string"},
		},
	}

	return gin.H{
		"openapi": "3.0.3",
		"info": gin.H{
			"title":       "LogMonitor API",
			"version":     "1.0.0",
			"description": "REST API for remote log monitoring, collection and integrity control.",
		},
		"paths": gin.H{
			"/healthz": gin.H{
				"get": gin.H{
					"summary": "Liveness probe",
					"responses": gin.H{
						"200": gin.H{"description": "Process is alive"},
					},
				},
			},
			"/readyz": gin.H{
				"get": gin.H{
					"summary": "Readiness probe",
					"responses": gin.H{
						"200": gin.H{"description": "Process is ready"},
						"503": gin.H{"description": "Process is not ready"},
					},
				},
			},
			"/api/dashboard": gin.H{
				"get": authenticatedOperation("Dashboard summary for the future UI", gin.H{
					"200": jsonResponse("Dashboard summary"),
				}),
			},
			"/api/problems": gin.H{
				"get": authenticatedOperation("Aggregated operational problems", gin.H{
					"200": jsonResponseWithSchema(gin.H{
						"type":  "array",
						"items": gin.H{"$ref": "#/components/schemas/SystemProblem"},
					}),
				}),
			},
			"/api/runtime/validation": gin.H{
				"get": authenticatedOperation("Runtime validation and startup warnings", gin.H{
					"200": jsonResponse("Runtime validation snapshot"),
				}),
			},
			"/api/jobs": gin.H{
				"get": authenticatedOperation("List async jobs", gin.H{
					"parameters": []gin.H{
						queryParameter("type", "Optional job type filter"),
						queryParameter("status", "Optional job status filter"),
						queryParameter("server_id", "Optional server identifier"),
						queryParameter("log_file_id", "Optional log file identifier"),
						queryParameter("offset", "Pagination offset"),
						queryParameter("limit", "Pagination limit"),
					},
					"responses": gin.H{
						"200": jsonResponseWithSchema(gin.H{
							"type":  "array",
							"items": gin.H{"$ref": "#/components/schemas/Job"},
						}),
					},
				}),
			},
			"/api/jobs/{id}": gin.H{
				"get": authenticatedOperation("Get one async job", gin.H{
					"parameters": []gin.H{pathParameter("id", "Job identifier")},
					"responses": gin.H{
						"200": jsonResponseWithSchema(gin.H{"$ref": "#/components/schemas/Job"}),
						"404": jsonResponseWithSchema(errorSchema),
					},
				}),
			},
			"/api/servers": gin.H{
				"get": authenticatedOperation("List monitored servers", gin.H{
					"200": jsonResponseWithSchema(gin.H{
						"type":  "array",
						"items": gin.H{"$ref": "#/components/schemas/Server"},
					}),
				}),
				"post": authenticatedOperation("Create a monitored server", gin.H{
					"requestBody": jsonRequestBody(gin.H{"$ref": "#/components/schemas/CreateServerRequest"}),
					"responses": gin.H{
						"201": jsonResponseWithSchema(gin.H{"$ref": "#/components/schemas/Server"}),
						"400": jsonResponseWithSchema(errorSchema),
						"409": jsonResponseWithSchema(errorSchema),
					},
				}),
			},
			"/api/servers/{id}": gin.H{
				"get": authenticatedOperation("Get one monitored server", gin.H{
					"parameters": []gin.H{pathParameter("id", "Server identifier")},
					"responses": gin.H{
						"200": jsonResponseWithSchema(gin.H{"$ref": "#/components/schemas/Server"}),
						"404": jsonResponseWithSchema(errorSchema),
					},
				}),
				"put": authenticatedOperation("Update an API-managed server", gin.H{
					"parameters":  []gin.H{pathParameter("id", "Server identifier")},
					"requestBody": jsonRequestBody(gin.H{"$ref": "#/components/schemas/UpdateServerRequest"}),
					"responses": gin.H{
						"200": jsonResponseWithSchema(gin.H{"$ref": "#/components/schemas/Server"}),
						"400": jsonResponseWithSchema(errorSchema),
						"404": jsonResponseWithSchema(errorSchema),
						"409": jsonResponseWithSchema(errorSchema),
					},
				}),
				"delete": authenticatedOperation("Delete an API-managed server", gin.H{
					"parameters": []gin.H{pathParameter("id", "Server identifier")},
					"responses": gin.H{
						"204": gin.H{"description": "Server deleted"},
						"409": jsonResponseWithSchema(errorSchema),
					},
				}),
			},
			"/api/servers/{id}/retry": gin.H{
				"post": authenticatedOperation("Clear backoff and retry state for a server", gin.H{
					"parameters": []gin.H{pathParameter("id", "Server identifier")},
					"responses": gin.H{
						"200": jsonResponseWithSchema(gin.H{"$ref": "#/components/schemas/Server"}),
						"404": jsonResponseWithSchema(errorSchema),
					},
				}),
			},
			"/api/servers/discover": gin.H{
				"post": authenticatedOperation("Run log discovery for one server or for all servers", gin.H{
					"requestBody": jsonRequestBody(gin.H{"type": "object", "properties": gin.H{
						"server_id": gin.H{"type": "string"},
					}}),
					"responses": gin.H{
						"202": jsonResponseWithSchema(gin.H{"$ref": "#/components/schemas/Job"}),
					},
				}),
			},
			"/api/logfiles": gin.H{
				"get": authenticatedOperation("List active log files or log files of one server", gin.H{
					"parameters": []gin.H{queryParameter("server_id", "Optional server identifier")},
					"responses": gin.H{
						"200": jsonResponse("Log file list"),
					},
				}),
			},
			"/api/logfiles/collect": gin.H{
				"post": authenticatedOperation("Collect remote log entries", gin.H{
					"requestBody": jsonRequestBody(gin.H{"type": "object", "required": []string{"server_id"}, "properties": gin.H{
						"server_id":   gin.H{"type": "string"},
						"log_file_id": gin.H{"type": "string"},
					}}),
					"responses": gin.H{
						"202": jsonResponseWithSchema(gin.H{"$ref": "#/components/schemas/Job"}),
					},
				}),
			},
			"/api/entries": gin.H{
				"get": authenticatedOperation("List stored log entries", gin.H{
					"parameters": []gin.H{
						queryParameter("log_file_id", "Log file identifier"),
						queryParameter("offset", "Pagination offset"),
						queryParameter("limit", "Pagination limit"),
					},
					"responses": gin.H{
						"200": jsonResponse("Log entry list"),
					},
				}),
			},
			"/api/checks": gin.H{
				"get": authenticatedOperation("List integrity checks", gin.H{
					"parameters": []gin.H{
						queryParameter("log_file_id", "Log file identifier"),
						queryParameter("offset", "Pagination offset"),
						queryParameter("limit", "Pagination limit"),
					},
					"responses": gin.H{
						"200": jsonResponse("Integrity check list"),
					},
				}),
			},
			"/api/checks/run": gin.H{
				"post": authenticatedOperation("Run integrity checks", gin.H{
					"requestBody": jsonRequestBody(gin.H{"type": "object", "required": []string{"server_id"}, "properties": gin.H{
						"server_id":   gin.H{"type": "string"},
						"log_file_id": gin.H{"type": "string"},
					}}),
					"responses": gin.H{
						"202": jsonResponseWithSchema(gin.H{"$ref": "#/components/schemas/Job"}),
					},
				}),
			},
		},
		"components": gin.H{
			"securitySchemes": gin.H{
				"ApiKeyAuth": gin.H{
					"type": "apiKey",
					"in":   "header",
					"name": "X-API-Key",
				},
			},
			"schemas": gin.H{
				"Server": serverSchema,
				"CreateServerRequest": gin.H{
					"type":     "object",
					"required": []string{"name", "host", "username", "auth_type", "auth_value"},
					"properties": gin.H{
						"name":       gin.H{"type": "string"},
						"host":       gin.H{"type": "string"},
						"port":       gin.H{"type": "integer"},
						"username":   gin.H{"type": "string"},
						"auth_type":  gin.H{"type": "string", "enum": []string{"password", "key"}},
						"auth_value": gin.H{"type": "string"},
						"os_type":    gin.H{"type": "string", "enum": []string{"linux", "windows", "macos"}},
					},
				},
				"UpdateServerRequest": gin.H{
					"type":     "object",
					"required": []string{"name", "host", "username", "auth_type"},
					"properties": gin.H{
						"name":       gin.H{"type": "string"},
						"host":       gin.H{"type": "string"},
						"port":       gin.H{"type": "integer"},
						"username":   gin.H{"type": "string"},
						"auth_type":  gin.H{"type": "string", "enum": []string{"password", "key"}},
						"auth_value": gin.H{"type": "string"},
						"os_type":    gin.H{"type": "string", "enum": []string{"linux", "windows", "macos"}},
						"status":     gin.H{"type": "string", "enum": []string{"active", "inactive"}},
					},
				},
				"SystemProblem": gin.H{
					"type": "object",
					"properties": gin.H{
						"severity":    gin.H{"type": "string"},
						"type":        gin.H{"type": "string"},
						"server_id":   gin.H{"type": "string"},
						"server_name": gin.H{"type": "string"},
						"log_file_id": gin.H{"type": "string"},
						"log_path":    gin.H{"type": "string"},
						"message":     gin.H{"type": "string"},
						"detected_at": gin.H{"type": "string", "format": "date-time"},
					},
				},
				"Job": gin.H{
					"type": "object",
					"properties": gin.H{
						"id":              gin.H{"type": "string"},
						"type":            gin.H{"type": "string", "enum": []string{"discover", "collect", "integrity"}},
						"status":          gin.H{"type": "string", "enum": []string{"queued", "running", "succeeded", "failed", "canceled"}},
						"idempotency_key": gin.H{"type": "string"},
						"fingerprint":     gin.H{"type": "string"},
						"server_id":       gin.H{"type": "string"},
						"log_file_id":     gin.H{"type": "string"},
						"error":           gin.H{"type": "string"},
						"result":          gin.H{"type": "object"},
						"created_at":      gin.H{"type": "string", "format": "date-time"},
						"started_at":      gin.H{"type": "string", "format": "date-time"},
						"finished_at":     gin.H{"type": "string", "format": "date-time"},
					},
				},
			},
		},
	}
}

// authenticatedOperation adds API key security to one OpenAPI operation.
func authenticatedOperation(summary string, operation gin.H) gin.H {
	operation["summary"] = summary
	operation["security"] = []gin.H{{"ApiKeyAuth": []string{}}}
	return operation
}

// jsonRequestBody describes a JSON request schema in the OpenAPI document.
func jsonRequestBody(schema gin.H) gin.H {
	return gin.H{
		"required": true,
		"content": gin.H{
			"application/json": gin.H{
				"schema": schema,
			},
		},
	}
}

// jsonResponse describes a generic JSON response without a strict schema.
func jsonResponse(description string) gin.H {
	return gin.H{
		"description": description,
		"content": gin.H{
			"application/json": gin.H{},
		},
	}
}

// jsonResponseWithSchema describes a JSON response with an explicit schema.
func jsonResponseWithSchema(schema gin.H) gin.H {
	return gin.H{
		"description": "JSON response",
		"content": gin.H{
			"application/json": gin.H{
				"schema": schema,
			},
		},
	}
}

// pathParameter describes a required path parameter in the OpenAPI document.
func pathParameter(name, description string) gin.H {
	return gin.H{
		"in":          "path",
		"name":        name,
		"required":    true,
		"description": description,
		"schema": gin.H{
			"type": "string",
		},
	}
}

// queryParameter describes an optional query parameter in the OpenAPI document.
func queryParameter(name, description string) gin.H {
	return gin.H{
		"in":          "query",
		"name":        name,
		"required":    false,
		"description": description,
		"schema": gin.H{
			"type": "string",
		},
	}
}
