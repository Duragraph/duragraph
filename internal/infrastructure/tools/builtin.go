package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// HTTPTool makes HTTP requests
type HTTPTool struct{}

func (t *HTTPTool) Name() string {
	return "http_request"
}

func (t *HTTPTool) Description() string {
	return "Makes HTTP requests to external APIs"
}

func (t *HTTPTool) Execute(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
	url, ok := args["url"].(string)
	if !ok {
		return nil, fmt.Errorf("url is required")
	}

	method := "GET"
	if m, ok := args["method"].(string); ok {
		method = strings.ToUpper(m)
	}

	var body io.Reader
	if bodyData, ok := args["body"]; ok {
		bodyJSON, _ := json.Marshal(bodyData)
		body = strings.NewReader(string(bodyJSON))
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	// Add headers
	if headers, ok := args["headers"].(map[string]interface{}); ok {
		for k, v := range headers {
			req.Header.Set(k, fmt.Sprintf("%v", v))
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Try to parse as JSON
	var jsonData interface{}
	if err := json.Unmarshal(respBody, &jsonData); err == nil {
		return map[string]interface{}{
			"status_code": resp.StatusCode,
			"body":        jsonData,
			"headers":     resp.Header,
		}, nil
	}

	// Return as string if not JSON
	return map[string]interface{}{
		"status_code": resp.StatusCode,
		"body":        string(respBody),
		"headers":     resp.Header,
	}, nil
}

func (t *HTTPTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"url": map[string]interface{}{
				"type":        "string",
				"description": "The URL to request",
			},
			"method": map[string]interface{}{
				"type":        "string",
				"description": "HTTP method (GET, POST, etc.)",
				"enum":        []string{"GET", "POST", "PUT", "PATCH", "DELETE"},
			},
			"headers": map[string]interface{}{
				"type":        "object",
				"description": "HTTP headers",
			},
			"body": map[string]interface{}{
				"type":        "object",
				"description": "Request body (for POST/PUT/PATCH)",
			},
		},
		"required": []string{"url"},
	}
}

// JSONProcessorTool processes JSON data
type JSONProcessorTool struct{}

func (t *JSONProcessorTool) Name() string {
	return "json_processor"
}

func (t *JSONProcessorTool) Description() string {
	return "Processes and transforms JSON data"
}

func (t *JSONProcessorTool) Execute(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
	data, ok := args["data"]
	if !ok {
		return nil, fmt.Errorf("data is required")
	}

	operation, ok := args["operation"].(string)
	if !ok {
		operation = "parse"
	}

	switch operation {
	case "parse":
		// Data is already parsed as map
		return map[string]interface{}{
			"result": data,
		}, nil

	case "stringify":
		jsonStr, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"result": string(jsonStr),
		}, nil

	case "extract":
		path, ok := args["path"].(string)
		if !ok {
			return nil, fmt.Errorf("path is required for extract operation")
		}

		value := extractJSONPath(data, path)
		return map[string]interface{}{
			"result": value,
		}, nil

	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}
}

func (t *JSONProcessorTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"data": map[string]interface{}{
				"description": "The JSON data to process",
			},
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "Operation to perform",
				"enum":        []string{"parse", "stringify", "extract"},
			},
			"path": map[string]interface{}{
				"type":        "string",
				"description": "JSON path for extract operation (e.g., 'user.name')",
			},
		},
		"required": []string{"data", "operation"},
	}
}

// StringProcessorTool processes strings
type StringProcessorTool struct{}

func (t *StringProcessorTool) Name() string {
	return "string_processor"
}

func (t *StringProcessorTool) Description() string {
	return "Processes and transforms strings"
}

func (t *StringProcessorTool) Execute(ctx context.Context, args map[string]interface{}) (map[string]interface{}, error) {
	text, ok := args["text"].(string)
	if !ok {
		return nil, fmt.Errorf("text is required")
	}

	operation, ok := args["operation"].(string)
	if !ok {
		operation = "lowercase"
	}

	var result string
	switch operation {
	case "lowercase":
		result = strings.ToLower(text)
	case "uppercase":
		result = strings.ToUpper(text)
	case "trim":
		result = strings.TrimSpace(text)
	case "replace":
		old, _ := args["old"].(string)
		new, _ := args["new"].(string)
		result = strings.ReplaceAll(text, old, new)
	case "split":
		delimiter, _ := args["delimiter"].(string)
		if delimiter == "" {
			delimiter = ","
		}
		parts := strings.Split(text, delimiter)
		return map[string]interface{}{
			"result": parts,
		}, nil
	default:
		return nil, fmt.Errorf("unknown operation: %s", operation)
	}

	return map[string]interface{}{
		"result": result,
	}, nil
}

func (t *StringProcessorTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"text": map[string]interface{}{
				"type":        "string",
				"description": "The text to process",
			},
			"operation": map[string]interface{}{
				"type":        "string",
				"description": "Operation to perform",
				"enum":        []string{"lowercase", "uppercase", "trim", "replace", "split"},
			},
			"old": map[string]interface{}{
				"type":        "string",
				"description": "Text to replace (for replace operation)",
			},
			"new": map[string]interface{}{
				"type":        "string",
				"description": "Replacement text (for replace operation)",
			},
			"delimiter": map[string]interface{}{
				"type":        "string",
				"description": "Delimiter for split operation",
			},
		},
		"required": []string{"text", "operation"},
	}
}

// extractJSONPath extracts a value from nested JSON using dot notation
func extractJSONPath(data interface{}, path string) interface{} {
	parts := strings.Split(path, ".")

	current := data
	for _, part := range parts {
		if m, ok := current.(map[string]interface{}); ok {
			current = m[part]
		} else {
			return nil
		}
	}

	return current
}

// RegisterBuiltinTools registers all built-in tools to a registry
func RegisterBuiltinTools(registry *Registry) error {
	tools := []Tool{
		&HTTPTool{},
		&JSONProcessorTool{},
		&StringProcessorTool{},
	}

	for _, tool := range tools {
		if err := registry.Register(tool); err != nil {
			return err
		}
	}

	return nil
}
