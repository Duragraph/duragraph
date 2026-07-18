// Hand-written store handlers. store is the LangGraph cross-thread KV store
// (store_items table, per-tenant DB) — NOT event-sourced, so these run plain
// s.Tenant pool queries with no events/outbox/writeTx. Routes are generated
// into store_gen.go (endpoints marked custom: true in endpoints.yaml); the
// bodies live here. namespace is TEXT[] — array ops, never LIKE.
package endpoints

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// StorePut upserts an item by (namespace, key). PUT /store/items -> 204.
func (s *Server) StorePut(c echo.Context) error {
	ctx := c.Request().Context()
	var req StorePutRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	value := mustJSON(req.Value)
	if _, err := s.Tenant.Exec(ctx, `
		INSERT INTO store_items (namespace, key, value)
		VALUES ($1, $2, $3)
		ON CONFLICT (namespace, key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		req.Namespace, req.Key, value); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// StoreGet reads one item by namespace (repeated query param) + key.
// GET /store/items?namespace=a&namespace=b&key=k -> 200 Item / 404.
func (s *Server) StoreGet(c echo.Context) error {
	ctx := c.Request().Context()
	namespace := c.QueryParams()["namespace"]
	key := c.QueryParam("key")
	if key == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "key is required")
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, namespace, key, value, created_at, updated_at
		FROM store_items WHERE namespace = $1 AND key = $2`, namespace, key)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	row, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[storeItemRow])
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.JSON(http.StatusOK, row.toAPI())
}

// StoreDelete removes an item by (namespace, key). DELETE /store/items -> 204
// (idempotent; 204 even when nothing matched, per the OpenAPI contract).
func (s *Server) StoreDelete(c echo.Context) error {
	ctx := c.Request().Context()
	var req StoreDeleteRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var namespace []string
	if req.Namespace != nil {
		namespace = *req.Namespace
	}
	if _, err := s.Tenant.Exec(ctx,
		`DELETE FROM store_items WHERE namespace = $1 AND key = $2`,
		namespace, req.Key); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return c.NoContent(http.StatusNoContent)
}

// StoreSearch lists items under a namespace prefix, optionally filtered by a
// jsonb subset match. POST /store/items/search -> 200 SearchItemsResponse.
// namespace_prefix matches when the item's leading namespace elements equal the
// prefix array. filter uses `value @> $filter` (GIN jsonb_path_ops index).
func (s *Server) StoreSearch(c echo.Context) error {
	ctx := c.Request().Context()
	var req StoreSearchRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var prefix []string
	if req.NamespacePrefix != nil {
		prefix = *req.NamespacePrefix
	}
	var filter any
	if req.Filter != nil {
		filter = mustJSON(*req.Filter)
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT id, namespace, key, value, created_at, updated_at
		FROM store_items
		WHERE ($1::text[] IS NULL OR namespace[1:cardinality($1)] = $1)
		  AND ($2::jsonb IS NULL OR value @> $2)
		ORDER BY namespace, key
		LIMIT $3 OFFSET $4`,
		nilIfEmpty(prefix), filter, intOr(req.Limit, 10), intOr(req.Offset, 0))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	list, err := pgx.CollectRows(rows, pgx.RowToStructByName[storeItemRow])
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	out := SearchItemsResponse{Items: make([]Item, len(list))}
	for i := range list {
		out.Items[i] = list[i].toAPI()
	}
	return c.JSON(http.StatusOK, out)
}

// StoreListNamespaces returns the distinct namespaces under a prefix.
// POST /store/namespaces -> 200 ListNamespaceResponse ([][]string).
func (s *Server) StoreListNamespaces(c echo.Context) error {
	ctx := c.Request().Context()
	var req StoreListNamespacesRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	var prefix []string
	if req.Prefix != nil {
		prefix = *req.Prefix
	}
	rows, err := s.Tenant.Query(ctx, `
		SELECT DISTINCT namespace
		FROM store_items
		WHERE ($1::text[] IS NULL OR namespace[1:cardinality($1)] = $1)
		ORDER BY namespace
		LIMIT $2 OFFSET $3`,
		nilIfEmpty(prefix), intOr(req.Limit, 100), intOr(req.Offset, 0))
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	out := ListNamespaceResponse{}
	for rows.Next() {
		var ns []string
		if err := rows.Scan(&ns); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		out = append(out, ns)
	}
	if rows.Err() != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, rows.Err().Error())
	}
	return c.JSON(http.StatusOK, out)
}
