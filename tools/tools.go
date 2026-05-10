//go:build tools
// +build tools

// Package tools pins development-only tools to module dependencies so
// `go mod tidy` keeps them in go.sum and `go run` can invoke them
// without a global install.
package tools

import (
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/playwright-community/playwright-go/cmd/playwright"
)
