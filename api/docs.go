// Package api GENERATED BY SWAG; DO NOT EDIT
// This file was generated by swaggo/swag
package api

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "license": {
            "name": "Apache 2.0",
            "url": "https://www.apache.org/licenses/LICENSE-2.0"
        },
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/repositories/": {
            "get": {
                "description": "list repositories",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "repositories"
                ],
                "summary": "List Repositories",
                "operationId": "listRepositories",
                "parameters": [
                    {
                        "type": "integer",
                        "description": "Offset into the list of results to return in the response",
                        "name": "offset",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "description": "Limit the number of items returned",
                        "name": "limit",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Comma separated list of architecture to optionally filter-on (e.g. 'x86_64,s390x' would return Repositories with x86_64 or s390x only)",
                        "name": "version",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Comma separated list of versions to optionally filter-on  (e.g. '7,8' would return Repositories with versions 7 or 8 only)",
                        "name": "arch",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Filter by compatible arch (e.g. 'x86_64' would return Repositories with the 'x86_64' arch and Repositories where arch is not set)",
                        "name": "available_for_version",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Filter by compatible version (e.g. 7 would return Repositories with the version 7 or where version is not set)",
                        "name": "available_for_arch",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Search term for name and url.",
                        "name": "search",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Filter repositories by name using an exact match",
                        "name": "name",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Filter repositories by name using an exact match",
                        "name": "url",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/api.RepositoryCollectionResponse"
                        }
                    }
                }
            },
            "post": {
                "description": "create a repository",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "repositories"
                ],
                "summary": "Create Repository",
                "operationId": "createRepository",
                "parameters": [
                    {
                        "description": "request body",
                        "name": "body",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/api.RepositoryRequest"
                        }
                    }
                ],
                "responses": {
                    "201": {
                        "description": "Created",
                        "schema": {
                            "$ref": "#/definitions/api.RepositoryResponse"
                        },
                        "headers": {
                            "Location": {
                                "type": "string",
                                "description": "resource URL"
                            }
                        }
                    }
                }
            }
        },
        "/repositories/bulk_create/": {
            "post": {
                "description": "bulk create repositories",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "repositories"
                ],
                "summary": "Bulk create repositories",
                "operationId": "bulkCreateRepositories",
                "parameters": [
                    {
                        "description": "request body",
                        "name": "body",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/api.RepositoryRequest"
                            }
                        }
                    }
                ],
                "responses": {
                    "201": {
                        "description": "Created",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/api.RepositoryBulkCreateResponse"
                            }
                        },
                        "headers": {
                            "Location": {
                                "type": "string",
                                "description": "resource URL"
                            }
                        }
                    }
                }
            }
        },
        "/repositories/{uuid}": {
            "get": {
                "description": "Get information about a Repository",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "repositories"
                ],
                "summary": "Get Repository",
                "operationId": "getRepository",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Identifier of the Repository",
                        "name": "uuid",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/api.RepositoryResponse"
                        }
                    }
                }
            },
            "put": {
                "description": "Fully update a repository",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "repositories"
                ],
                "summary": "Update Repository",
                "operationId": "fullUpdateRepository",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Identifier of the Repository",
                        "name": "uuid",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "request body",
                        "name": "body",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/api.RepositoryRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            },
            "delete": {
                "tags": [
                    "repositories"
                ],
                "summary": "Delete a repository",
                "operationId": "deleteRepository",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Identifier of the Repository",
                        "name": "uuid",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "204": {
                        "description": "Repository was successfully deleted"
                    }
                }
            },
            "patch": {
                "description": "Partially Update a repository",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "repositories"
                ],
                "summary": "Partial Update Repository",
                "operationId": "partialUpdateRepository",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Identifier of the Repository",
                        "name": "uuid",
                        "in": "path",
                        "required": true
                    },
                    {
                        "description": "request body",
                        "name": "body",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/api.RepositoryRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/repositories/{uuid}/rpms": {
            "get": {
                "description": "list repositories RPMs",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "repositories",
                    "rpms"
                ],
                "summary": "List Repositories RPMs",
                "operationId": "listRepositoriesRpms",
                "parameters": [
                    {
                        "type": "string",
                        "description": "Identifier of the Repository",
                        "name": "uuid",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/api.RepositoryRpmCollectionResponse"
                        }
                    }
                }
            }
        },
        "/repository_parameters/": {
            "get": {
                "description": "get repository parameters (Versions and Architectures)",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "repositories"
                ],
                "summary": "List Repository Parameters",
                "operationId": "listRepositoryParameters",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/api.RepositoryParameterResponse"
                        }
                    }
                }
            }
        },
        "/repository_parameters/external_gpg_key": {
            "post": {
                "description": "Fetch gpgkey from URL",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "gpgKey"
                ],
                "summary": "Fetch gpgkey from URL",
                "operationId": "fetchGpgKey",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/api.FetchGPGKeyResponse"
                        }
                    }
                }
            }
        },
        "/repository_parameters/validate/": {
            "post": {
                "description": "Validate parameters prior to creating a repository, including checking if remote yum metadata is present",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "repositories"
                ],
                "summary": "Validate parameters prior to creating a repository",
                "operationId": "validateRepositoryParameters",
                "parameters": [
                    {
                        "description": "request body",
                        "name": "body",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/api.RepositoryValidationRequest"
                            }
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/api.RepositoryValidationResponse"
                            }
                        }
                    }
                }
            }
        },
        "/rpms/names": {
            "post": {
                "description": "Search RPMs for a given list of repository URLs",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "repositories",
                    "rpms"
                ],
                "summary": "Search RPMs",
                "operationId": "searchRpm",
                "parameters": [
                    {
                        "description": "request body",
                        "name": "body",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/api.SearchRpmRequest"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/api.SearchRpmResponse"
                            }
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "api.FetchGPGKeyResponse": {
            "type": "object",
            "properties": {
                "gpg_key": {
                    "description": "The downloaded GPG Keys from the provided url.",
                    "type": "string"
                }
            }
        },
        "api.GenericAttributeValidationResponse": {
            "type": "object",
            "properties": {
                "error": {
                    "description": "Error message if the attribute is not valid",
                    "type": "string"
                },
                "skipped": {
                    "description": "Skipped if the attribute is not passed in for validation",
                    "type": "boolean"
                },
                "valid": {
                    "description": "Valid if not skipped and the provided attribute is valid",
                    "type": "boolean"
                }
            }
        },
        "api.Links": {
            "type": "object",
            "properties": {
                "first": {
                    "description": "Path to first page of results",
                    "type": "string"
                },
                "last": {
                    "description": "Path to last page of results",
                    "type": "string"
                },
                "next": {
                    "description": "Path to next page of results",
                    "type": "string"
                },
                "prev": {
                    "description": "Path to previous page of results",
                    "type": "string"
                }
            }
        },
        "api.RepositoryBulkCreateResponse": {
            "type": "object",
            "properties": {
                "error": {
                    "description": "Error during creation",
                    "type": "string"
                },
                "repository": {
                    "description": "Repository object information",
                    "$ref": "#/definitions/api.RepositoryResponse"
                }
            }
        },
        "api.RepositoryCollectionResponse": {
            "type": "object",
            "properties": {
                "data": {
                    "description": "Requested Data",
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/api.RepositoryResponse"
                    }
                },
                "links": {
                    "description": "Links to other pages of results",
                    "$ref": "#/definitions/api.Links"
                },
                "meta": {
                    "description": "Metadata about the request",
                    "$ref": "#/definitions/api.ResponseMetadata"
                }
            }
        },
        "api.RepositoryParameterResponse": {
            "type": "object",
            "properties": {
                "distribution_arches": {
                    "description": "Architectures available for repository creation",
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/config.DistributionArch"
                    }
                },
                "distribution_versions": {
                    "description": "Versions available for repository creation",
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/config.DistributionVersion"
                    }
                }
            }
        },
        "api.RepositoryRequest": {
            "type": "object",
            "properties": {
                "distribution_arch": {
                    "description": "Architecture to restrict client usage to",
                    "type": "string",
                    "example": "x86_64"
                },
                "distribution_versions": {
                    "description": "Versions to restrict client usage to",
                    "type": "array",
                    "items": {
                        "type": "string"
                    },
                    "example": [
                        "7",
                        "8"
                    ]
                },
                "gpg_key": {
                    "description": "GPG key for repository",
                    "type": "string"
                },
                "metadata_verification": {
                    "description": "Verify packages",
                    "type": "boolean"
                },
                "name": {
                    "description": "Name of the remote yum repository",
                    "type": "string"
                },
                "url": {
                    "description": "URL of the remote yum repository",
                    "type": "string"
                }
            }
        },
        "api.RepositoryResponse": {
            "type": "object",
            "properties": {
                "account_id": {
                    "description": "Account ID of the owner",
                    "type": "string",
                    "readOnly": true
                },
                "distribution_arch": {
                    "description": "Architecture to restrict client usage to",
                    "type": "string",
                    "example": "x86_64"
                },
                "distribution_versions": {
                    "description": "Versions to restrict client usage to",
                    "type": "array",
                    "items": {
                        "type": "string"
                    },
                    "example": [
                        "7",
                        "8"
                    ]
                },
                "gpg_key": {
                    "description": "GPG key for repository",
                    "type": "string"
                },
                "last_introspection_error": {
                    "description": "Error of last attempted introspection",
                    "type": "string"
                },
                "last_introspection_time": {
                    "description": "Timestamp of last attempted introspection",
                    "type": "string"
                },
                "last_success_introspection_time": {
                    "description": "Timestamp of last successful introspection",
                    "type": "string"
                },
                "last_update_introspection_time": {
                    "description": "Timestamp of last introspection that had updates",
                    "type": "string"
                },
                "metadata_verification": {
                    "description": "Verify packages",
                    "type": "boolean"
                },
                "name": {
                    "description": "Name of the remote yum repository",
                    "type": "string"
                },
                "org_id": {
                    "description": "Organization ID of the owner",
                    "type": "string",
                    "readOnly": true
                },
                "package_count": {
                    "description": "Number of packages last read in the repository",
                    "type": "integer"
                },
                "status": {
                    "description": "Status of repository introspection (Valid, Invalid, Unavailable, Pending)",
                    "type": "string"
                },
                "url": {
                    "description": "URL of the remote yum repository",
                    "type": "string"
                },
                "uuid": {
                    "description": "UUID of the object",
                    "type": "string",
                    "readOnly": true
                }
            }
        },
        "api.RepositoryRpm": {
            "type": "object",
            "properties": {
                "arch": {
                    "description": "The Architecture of the rpm",
                    "type": "string"
                },
                "checksum": {
                    "description": "The checksum of the rpm",
                    "type": "string"
                },
                "epoch": {
                    "description": "The epoch of the rpm",
                    "type": "integer"
                },
                "name": {
                    "description": "The rpm package name",
                    "type": "string"
                },
                "release": {
                    "description": "The release of the rpm",
                    "type": "string"
                },
                "summary": {
                    "description": "The summary of the rpm",
                    "type": "string"
                },
                "uuid": {
                    "description": "Identifier of the rpm",
                    "type": "string"
                },
                "version": {
                    "description": "The version of the  rpm",
                    "type": "string"
                }
            }
        },
        "api.RepositoryRpmCollectionResponse": {
            "type": "object",
            "properties": {
                "data": {
                    "description": "List of rpms",
                    "type": "array",
                    "items": {
                        "$ref": "#/definitions/api.RepositoryRpm"
                    }
                },
                "links": {
                    "description": "Links to other pages of results",
                    "$ref": "#/definitions/api.Links"
                },
                "meta": {
                    "description": "Metadata about the request",
                    "$ref": "#/definitions/api.ResponseMetadata"
                }
            }
        },
        "api.RepositoryValidationRequest": {
            "type": "object",
            "properties": {
                "gpg_key": {
                    "description": "GPGKey of the remote yum repository",
                    "type": "string"
                },
                "metadata_verification": {
                    "description": "If set, attempt to validate the yum metadata with the specified GPG Key",
                    "type": "boolean"
                },
                "name": {
                    "description": "Name of the remote yum repository",
                    "type": "string"
                },
                "url": {
                    "description": "URL of the remote yum repository",
                    "type": "string"
                },
                "uuid": {
                    "description": "If set, this is an \"Update\" validation",
                    "type": "string"
                }
            }
        },
        "api.RepositoryValidationResponse": {
            "type": "object",
            "properties": {
                "gpg_key": {
                    "description": "Validation response for the GPG Key",
                    "$ref": "#/definitions/api.GenericAttributeValidationResponse"
                },
                "name": {
                    "description": "Validation response for repository name",
                    "$ref": "#/definitions/api.GenericAttributeValidationResponse"
                },
                "url": {
                    "description": "Validation response for repository url",
                    "$ref": "#/definitions/api.UrlValidationResponse"
                }
            }
        },
        "api.ResponseMetadata": {
            "type": "object",
            "properties": {
                "count": {
                    "description": "Total count of results",
                    "type": "integer"
                },
                "limit": {
                    "description": "Limit of results used for the request",
                    "type": "integer"
                },
                "offset": {
                    "description": "Offset into results used for the request",
                    "type": "integer"
                }
            }
        },
        "api.SearchRpmRequest": {
            "type": "object",
            "properties": {
                "search": {
                    "description": "Search string to search rpm names",
                    "type": "string"
                },
                "urls": {
                    "description": "URLs of repositories to search",
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "uuids": {
                    "description": "List of RepositoryConfig UUIDs to search",
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                }
            }
        },
        "api.SearchRpmResponse": {
            "type": "object",
            "properties": {
                "package_name": {
                    "description": "Package name found",
                    "type": "string"
                },
                "summary": {
                    "description": "Summary of the package found",
                    "type": "string"
                }
            }
        },
        "api.UrlValidationResponse": {
            "type": "object",
            "properties": {
                "error": {
                    "description": "Error message if the attribute is not valid",
                    "type": "string"
                },
                "http_code": {
                    "description": "If the metadata cannot be fetched successfully, the http code that is returned if the http request was completed",
                    "type": "integer"
                },
                "metadata_present": {
                    "description": "True if the metadata can be fetched successfully",
                    "type": "boolean"
                },
                "metadata_signature_present": {
                    "description": "True if a repomd.xml.sig file was found in the repository",
                    "type": "boolean"
                },
                "skipped": {
                    "description": "Skipped if the URL is not passed in for validation",
                    "type": "boolean"
                },
                "valid": {
                    "description": "Valid if not skipped and the provided attribute is valid",
                    "type": "boolean"
                }
            }
        },
        "config.DistributionArch": {
            "type": "object",
            "properties": {
                "label": {
                    "description": "Static label of the arch",
                    "type": "string"
                },
                "name": {
                    "description": "Human-readable form of the arch",
                    "type": "string"
                }
            }
        },
        "config.DistributionVersion": {
            "type": "object",
            "properties": {
                "label": {
                    "description": "Static label of the version",
                    "type": "string"
                },
                "name": {
                    "description": "Human-readable form of the version",
                    "type": "string"
                }
            }
        }
    },
    "securityDefinitions": {
        "RhIdentity": {
            "type": "apiKey",
            "name": "x-rh-identity",
            "in": "header"
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "v1.0.0",
	Host:             "api.example.com",
	BasePath:         "/api/content-sources/v1.0/",
	Schemes:          []string{},
	Title:            "ContentSourcesBackend",
	Description:      "API of the Content Sources application on [console.redhat.com](https://console.redhat.com)\n",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
