package trengine

import "embed"

//go:embed web/*
var WebFiles embed.FS

//go:embed openapi.yaml
var OpenAPISpec []byte
