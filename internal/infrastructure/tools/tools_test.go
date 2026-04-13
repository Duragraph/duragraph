package tools

import (
	"context"
	"testing"
)

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "test_tool", desc: "A test tool"}

	if err := r.Register(tool); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := r.Get("test_tool")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if got.Name() != "test_tool" {
		t.Errorf("expected test_tool, got %q", got.Name())
	}
}

func TestRegistry_Register_EmptyName(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: ""}

	err := r.Register(tool)
	if err == nil {
		t.Error("expected error for empty name")
	}
}

func TestRegistry_Register_Duplicate(t *testing.T) {
	r := NewRegistry()
	tool := &mockTool{name: "dup"}

	r.Register(tool)
	err := r.Register(tool)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistry_Unregister(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "removeme"})

	if err := r.Unregister("removeme"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	_, err := r.Get("removeme")
	if err == nil {
		t.Error("expected not found after unregister")
	}
}

func TestRegistry_Unregister_NotFound(t *testing.T) {
	r := NewRegistry()
	err := r.Unregister("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent tool")
	}
}

func TestRegistry_Get_NotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Get("missing")
	if err == nil {
		t.Error("expected error for missing tool")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{name: "a"})
	r.Register(&mockTool{name: "b"})
	r.Register(&mockTool{name: "c"})

	tools := r.List()
	if len(tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(tools))
	}
}

func TestRegistry_List_Empty(t *testing.T) {
	r := NewRegistry()
	tools := r.List()
	if len(tools) != 0 {
		t.Errorf("expected 0 tools, got %d", len(tools))
	}
}

func TestRegistry_Execute(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name: "echo",
		execFn: func(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"echoed": args["input"]}, nil
		},
	})

	result, err := r.Execute(context.Background(), "echo", map[string]interface{}{"input": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["echoed"] != "hello" {
		t.Errorf("expected echoed=hello, got %v", result)
	}
}

func TestRegistry_Execute_NotFound(t *testing.T) {
	r := NewRegistry()
	_, err := r.Execute(context.Background(), "missing", nil)
	if err == nil {
		t.Error("expected error for missing tool")
	}
}

func TestRegistry_GetSchemas(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockTool{
		name: "my_tool",
		desc: "Does something",
		schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"input": map[string]interface{}{"type": "string"},
			},
		},
	})

	schemas := r.GetSchemas()
	if len(schemas) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(schemas))
	}
	if schemas[0]["name"] != "my_tool" {
		t.Errorf("expected my_tool, got %v", schemas[0]["name"])
	}
	if schemas[0]["description"] != "Does something" {
		t.Errorf("expected description, got %v", schemas[0]["description"])
	}
}

func TestJSONProcessorTool_Parse(t *testing.T) {
	tool := &JSONProcessorTool{}

	if tool.Name() != "json_processor" {
		t.Errorf("name: got %q", tool.Name())
	}

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"data":      map[string]interface{}{"key": "value"},
		"operation": "parse",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, ok := result["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("result type: %T", result["result"])
	}
	if data["key"] != "value" {
		t.Errorf("expected key=value, got %v", data)
	}
}

func TestJSONProcessorTool_Stringify(t *testing.T) {
	tool := &JSONProcessorTool{}

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"data":      map[string]interface{}{"key": "value"},
		"operation": "stringify",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	str, ok := result["result"].(string)
	if !ok {
		t.Fatalf("expected string result, got %T", result["result"])
	}
	if str != `{"key":"value"}` {
		t.Errorf("got %q", str)
	}
}

func TestJSONProcessorTool_Extract(t *testing.T) {
	tool := &JSONProcessorTool{}

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"data": map[string]interface{}{
			"user": map[string]interface{}{"name": "Alice"},
		},
		"operation": "extract",
		"path":      "user.name",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["result"] != "Alice" {
		t.Errorf("expected Alice, got %v", result["result"])
	}
}

func TestJSONProcessorTool_Extract_MissingPath(t *testing.T) {
	tool := &JSONProcessorTool{}

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"data":      "hello",
		"operation": "extract",
	})
	if err == nil {
		t.Error("expected error for missing path")
	}
}

func TestJSONProcessorTool_MissingData(t *testing.T) {
	tool := &JSONProcessorTool{}

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "parse",
	})
	if err == nil {
		t.Error("expected error for missing data")
	}
}

func TestJSONProcessorTool_UnknownOperation(t *testing.T) {
	tool := &JSONProcessorTool{}

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"data":      "test",
		"operation": "unknown",
	})
	if err == nil {
		t.Error("expected error for unknown operation")
	}
}

func TestStringProcessorTool_Lowercase(t *testing.T) {
	tool := &StringProcessorTool{}

	if tool.Name() != "string_processor" {
		t.Errorf("name: got %q", tool.Name())
	}

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"text":      "HELLO WORLD",
		"operation": "lowercase",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["result"] != "hello world" {
		t.Errorf("expected 'hello world', got %v", result["result"])
	}
}

func TestStringProcessorTool_Uppercase(t *testing.T) {
	tool := &StringProcessorTool{}

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"text":      "hello",
		"operation": "uppercase",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["result"] != "HELLO" {
		t.Errorf("got %v", result["result"])
	}
}

func TestStringProcessorTool_Trim(t *testing.T) {
	tool := &StringProcessorTool{}

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"text":      "  spaced  ",
		"operation": "trim",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["result"] != "spaced" {
		t.Errorf("got %v", result["result"])
	}
}

func TestStringProcessorTool_Replace(t *testing.T) {
	tool := &StringProcessorTool{}

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"text":      "hello world",
		"operation": "replace",
		"old":       "world",
		"new":       "earth",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["result"] != "hello earth" {
		t.Errorf("got %v", result["result"])
	}
}

func TestStringProcessorTool_Split(t *testing.T) {
	tool := &StringProcessorTool{}

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"text":      "a,b,c",
		"operation": "split",
		"delimiter": ",",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parts, ok := result["result"].([]string)
	if !ok {
		t.Fatalf("expected []string, got %T", result["result"])
	}
	if len(parts) != 3 || parts[0] != "a" || parts[2] != "c" {
		t.Errorf("got %v", parts)
	}
}

func TestStringProcessorTool_Split_DefaultDelimiter(t *testing.T) {
	tool := &StringProcessorTool{}

	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"text":      "a,b",
		"operation": "split",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	parts := result["result"].([]string)
	if len(parts) != 2 {
		t.Errorf("expected 2 parts, got %d", len(parts))
	}
}

func TestStringProcessorTool_MissingText(t *testing.T) {
	tool := &StringProcessorTool{}

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"operation": "lowercase",
	})
	if err == nil {
		t.Error("expected error for missing text")
	}
}

func TestStringProcessorTool_UnknownOperation(t *testing.T) {
	tool := &StringProcessorTool{}

	_, err := tool.Execute(context.Background(), map[string]interface{}{
		"text":      "test",
		"operation": "unknown",
	})
	if err == nil {
		t.Error("expected error for unknown operation")
	}
}

func TestExtractJSONPath(t *testing.T) {
	data := map[string]interface{}{
		"a": map[string]interface{}{
			"b": map[string]interface{}{
				"c": "deep",
			},
		},
	}

	if v := extractJSONPath(data, "a.b.c"); v != "deep" {
		t.Errorf("expected 'deep', got %v", v)
	}

	if v := extractJSONPath(data, "a.x"); v != nil {
		t.Errorf("expected nil for missing path, got %v", v)
	}

	if v := extractJSONPath("not a map", "a"); v != nil {
		t.Errorf("expected nil for non-map, got %v", v)
	}
}

func TestRegisterBuiltinTools(t *testing.T) {
	r := NewRegistry()
	if err := RegisterBuiltinTools(r); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tools := r.List()
	if len(tools) != 3 {
		t.Errorf("expected 3 builtin tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name()] = true
	}

	for _, expected := range []string{"http_request", "json_processor", "string_processor"} {
		if !names[expected] {
			t.Errorf("missing builtin tool: %s", expected)
		}
	}
}

func TestRegisterBuiltinTools_DoubleRegister(t *testing.T) {
	r := NewRegistry()
	RegisterBuiltinTools(r)
	err := RegisterBuiltinTools(r)
	if err == nil {
		t.Error("expected error on double registration")
	}
}

func TestHTTPTool_Schema(t *testing.T) {
	tool := &HTTPTool{}
	schema := tool.Schema()
	if schema["type"] != "object" {
		t.Errorf("expected object type, got %v", schema["type"])
	}
	props := schema["properties"].(map[string]interface{})
	if _, ok := props["url"]; !ok {
		t.Error("schema should have url property")
	}
}

func TestHTTPTool_MissingURL(t *testing.T) {
	tool := &HTTPTool{}
	_, err := tool.Execute(context.Background(), map[string]interface{}{})
	if err == nil {
		t.Error("expected error for missing url")
	}
}

type mockTool struct {
	name   string
	desc   string
	schema map[string]interface{}
	execFn func(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error)
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string { return t.desc }
func (t *mockTool) Schema() map[string]interface{} {
	if t.schema != nil {
		return t.schema
	}
	return map[string]interface{}{}
}
func (t *mockTool) Execute(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
	if t.execFn != nil {
		return t.execFn(ctx, args)
	}
	return map[string]interface{}{}, nil
}
