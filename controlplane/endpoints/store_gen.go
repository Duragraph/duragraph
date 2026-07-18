// Code generated from controlplane/gen/endpoints.yaml — DO NOT EDIT.
// Source of truth: spec/models/d2/endpoint-queries.d2 (via endpoints.yaml).
// Regenerate: go run ./controlplane/gen
package endpoints

import (
	"github.com/labstack/echo/v4"
)

// RegisterStore mounts the store endpoints on g (the /api/v1 group).
func (s *Server) RegisterStore(g *echo.Group) {
	g.PUT("/store/items", s.StorePut)
	g.GET("/store/items", s.StoreGet)
	g.DELETE("/store/items", s.StoreDelete)
	g.POST("/store/items/search", s.StoreSearch)
	g.POST("/store/namespaces", s.StoreListNamespaces)
}

// StorePut — PUT /store/items  (kind: write) — hand-written in store.go

// StoreGet — GET /store/items  (kind: read) — hand-written in store.go

// StoreDelete — DELETE /store/items  (kind: delete) — hand-written in store.go

// StoreSearch — POST /store/items/search  (kind: read) — hand-written in store.go

// StoreListNamespaces — POST /store/namespaces  (kind: read) — hand-written in store.go
