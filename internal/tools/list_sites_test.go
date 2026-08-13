package tools

import (
	"encoding/json"
	"testing"
)

func TestListSites_InputSchema(t *testing.T) {
	t.Parallel()
	var schema map[string]any
	if err := json.Unmarshal(listSitesInputSchema, &schema); err != nil {
		t.Fatalf("unmarshal listSitesInputSchema: %v", err)
	}

	properties, ok := schema["properties"]
	if !ok {
		t.Error("list_sites InputSchema is missing the 'properties' field")
	} else {
		if _, ok := properties.(map[string]any); !ok {
			t.Error("list_sites InputSchema 'properties' field must be a JSON object")
		}
	}

	required, ok := schema["required"]
	if !ok {
		t.Error("list_sites InputSchema is missing the 'required' field")
	} else {
		if _, ok := required.([]any); !ok {
			t.Error("list_sites InputSchema 'required' field must be a JSON array")
		}
	}

	additionalProperties, ok := schema["additionalProperties"]
	if !ok {
		t.Error("list_sites InputSchema is missing the 'additionalProperties' field")
	} else {
		additionalPropertiesBool, ok := additionalProperties.(bool)
		if !ok {
			t.Error("list_sites InputSchema 'additionalProperties' field must be a boolean")
		} else if additionalPropertiesBool {
			t.Error("list_sites InputSchema 'additionalProperties' must be false")
		}
	}
}
