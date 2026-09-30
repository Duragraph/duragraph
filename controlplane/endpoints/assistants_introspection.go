// Hand-written assistant introspection. Routes are generated from endpoints.yaml.
// The legacy handlers discover subgraph nodes, but their response types differ
// from the OpenAPI GraphSchema/Subgraphs wire contract.
package endpoints

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/labstack/echo/v4"
)

// introspectionGraph reads the latest graph bound to this assistant. A missing
// assistant is distinguished from an existing assistant with no graph.
func (s *Server) introspectionGraph(c echo.Context) (string, []byte, []byte, bool, error) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		return "", nil, nil, false, echo.NewHTTPError(http.StatusUnprocessableEntity, "invalid assistant_id")
	}
	ctx := c.Request().Context()
	var exists bool
	if err := s.Tenant.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assistants WHERE id = $1)`, id).Scan(&exists); err != nil {
		return "", nil, nil, false, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	if !exists {
		return "", nil, nil, false, echo.NewHTTPError(http.StatusNotFound, "assistant not found")
	}
	var name string
	var nodes, config []byte
	err = s.Tenant.QueryRow(ctx, `SELECT name, nodes, config FROM graphs WHERE assistant_id = $1 ORDER BY version DESC NULLS LAST, created_at DESC, id DESC LIMIT 1`, id).Scan(&name, &nodes, &config)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, nil, false, nil
	}
	if err != nil {
		return "", nil, nil, false, echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	return name, nodes, config, true, nil
}

func schemaObject(config map[string]interface{}, key string) map[string]interface{} {
	if schema, ok := config[key].(map[string]interface{}); ok {
		return schema
	}
	return nil
}

func decodeObject(data []byte) (map[string]interface{}, error) {
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		return nil, err
	}
	return obj, nil
}

// AssistantsGetSchemas returns the graph's actual JSON schemas from its config,
// not the nodes/edges graph definition used by GET /graph.
func (s *Server) AssistantsGetSchemas(c echo.Context) error {
	name, _, raw, found, err := s.introspectionGraph(c)
	if err != nil {
		return err
	}
	if !found {
		return echo.NewHTTPError(http.StatusNotFound, "graph not found")
	}
	config, err := decodeObject(raw)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	state := schemaObject(config, "state_schema")
	if state == nil {
		state = map[string]interface{}{}
	}
	return c.JSON(http.StatusOK, GraphSchema{
		GraphId: name, InputSchema: schemaPtr(config, "input_schema"),
		OutputSchema: schemaPtr(config, "output_schema"), StateSchema: state,
		ConfigSchema: schemaPtr(config, "config_schema"), ContextSchema: schemaPtr(config, "context_schema"),
	})
}

func schemaPtr(config map[string]interface{}, key string) *map[string]interface{} {
	obj := schemaObject(config, key)
	if obj == nil {
		return nil
	}
	return &obj
}

func subgraphSchema(config map[string]interface{}) GraphSchemaNoId {
	input, output, state := schemaObject(config, "input_schema"), schemaObject(config, "output_schema"), schemaObject(config, "state_schema")
	if input == nil {
		input = map[string]interface{}{}
	}
	if output == nil {
		output = map[string]interface{}{}
	}
	if state == nil {
		state = map[string]interface{}{}
	}
	return GraphSchemaNoId{
		InputSchema: input, OutputSchema: output, StateSchema: state,
		ConfigSchema: schemaPtr(config, "config_schema"), ContextSchema: schemaPtr(config, "context_schema"),
	}
}

// collectSubgraphs follows the legacy subgraph node convention: node.id is the
// default namespace, node.config.namespace overrides it, and nested nodes live
// in node.config.nodes. Malformed nodes are ignored rather than panicking.
func collectSubgraphs(nodes []interface{}, prefix string, recurse bool, out Subgraphs) {
	for _, item := range nodes {
		node, ok := item.(map[string]interface{})
		if !ok || node["type"] != "subgraph" {
			continue
		}
		config, _ := node["config"].(map[string]interface{})
		ns, _ := config["namespace"].(string)
		if ns == "" {
			ns, _ = node["id"].(string)
		}
		if ns == "" {
			continue
		}
		key := prefix + ns
		out[key] = subgraphSchema(config)
		if recurse {
			children, _ := config["nodes"].([]interface{})
			collectSubgraphs(children, key+":", true, out)
		}
	}
}

func (s *Server) assistantSubgraphs(c echo.Context, namespace string) error {
	recurse := false
	if raw := c.QueryParam("recurse"); raw != "" {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return echo.NewHTTPError(http.StatusUnprocessableEntity, "invalid recurse")
		}
		recurse = parsed
	}
	_, raw, _, found, err := s.introspectionGraph(c)
	if err != nil {
		return err
	}
	out := Subgraphs{}
	if found {
		var nodes []interface{}
		if err := json.Unmarshal(raw, &nodes); err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		collectSubgraphs(nodes, "", recurse, out)
	}
	if namespace != "" {
		filtered := Subgraphs{}
		for key, schema := range out {
			if key == namespace || (recurse && strings.HasPrefix(key, namespace+":")) {
				filtered[key] = schema
			}
		}
		if len(filtered) == 0 {
			return echo.NewHTTPError(http.StatusNotFound, "subgraph not found")
		}
		out = filtered
	}
	return c.JSON(http.StatusOK, out)
}

func (s *Server) AssistantsGetSubgraphs(c echo.Context) error {
	return s.assistantSubgraphs(c, "")
}

func (s *Server) AssistantsGetSubgraphsByNamespace(c echo.Context) error {
	return s.assistantSubgraphs(c, c.Param("namespace"))
}
