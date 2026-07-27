package mcp

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestToolSchemasDoNotEmitNullRequired(t *testing.T) {
	encoded, err := json.Marshal(Tools())
	if err != nil {
		t.Fatalf("marshal tools: %v", err)
	}

	if bytes.Contains(encoded, []byte(`"required":null`)) {
		t.Fatalf("tool schemas contain required:null: %s", encoded)
	}
}

func TestRequiredIsStringArrayWhenPresent(t *testing.T) {
	for _, tool := range Tools() {
		required, present := tool.InputSchema["required"]
		if !present {
			continue
		}

		if _, ok := required.([]string); !ok {
			t.Errorf("%s: required must be []string, got %T", tool.Name, required)
		}
	}
}
