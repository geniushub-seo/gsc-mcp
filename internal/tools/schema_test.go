package tools

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// explicitSchema is a tool whose InputSchema is handwritten rather than
// inferred by the SDK. Keeping the schema in sync with the Go struct prevents
// silent drift when fields are added or renamed.
var explicitSchemas = []struct {
	name   string
	schema json.RawMessage
	input  any
}{
	{"list_sites", listSitesInputSchema, listSitesInput{}},
	{"query_search_analytics", querySearchAnalyticsInputSchema, querySearchAnalyticsInput{}},
	{"compare_periods", comparePeriodsInputSchema, comparePeriodsInput{}},
	{"inspect_url", inspectURLInputSchema, inspectURLInput{}},
}

func TestExplicitSchemas_MatchStructFields(t *testing.T) {
	t.Parallel()

	for _, tc := range explicitSchemas {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var schema map[string]any
			if err := json.Unmarshal(tc.schema, &schema); err != nil {
				t.Fatalf("unmarshal schema: %v", err)
			}
			assertSchemaMatchesStruct(t, schema, reflect.TypeOf(tc.input), "")
		})
	}
}

func TestExplicitSchemas_NoUnionTypes(t *testing.T) {
	t.Parallel()

	for _, tc := range explicitSchemas {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var schema map[string]any
			if err := json.Unmarshal(tc.schema, &schema); err != nil {
				t.Fatalf("unmarshal schema: %v", err)
			}
			findUnionTypes(t, "", schema)
		})
	}
}

func assertSchemaMatchesStruct(t *testing.T, schema map[string]any, typ reflect.Type, path string) {
	t.Helper()

	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ.Kind() != reflect.Struct {
		t.Fatalf("%s: expected struct, got %v", path, typ.Kind())
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s: schema missing properties", path)
	}

	wantKeys := structJSONKeys(typ)
	gotKeys := make([]string, 0, len(props))
	for k := range props {
		gotKeys = append(gotKeys, k)
	}
	sort.Strings(wantKeys)
	sort.Strings(gotKeys)
	if !stringSlicesEqual(wantKeys, gotKeys) {
		t.Errorf("%s: schema keys %v do not match struct json tags %v", path, gotKeys, wantKeys)
	}

	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := jsonTagName(field.Tag.Get("json"))
		if name == "-" || name == "" {
			continue
		}
		prop, ok := props[name].(map[string]any)
		if !ok {
			continue
		}
		fieldPath := name
		if path != "" {
			fieldPath = path + "." + name
		}
		fieldType := field.Type

		switch fieldType.Kind() {
		case reflect.Slice:
			if items, ok := prop["items"].(map[string]any); ok {
				elem := fieldType.Elem()
				if elem.Kind() == reflect.Struct || (elem.Kind() == reflect.Pointer && elem.Elem().Kind() == reflect.Struct) {
					assertSchemaMatchesStruct(t, items, elem, fieldPath+"[]")
				}
			}
		case reflect.Struct:
			assertSchemaMatchesStruct(t, prop, fieldType, fieldPath)
		default:
			// scalar or other non-nested type: nothing to recurse
		}
	}
}

func findUnionTypes(t *testing.T, path string, v any) {
	t.Helper()
	switch x := v.(type) {
	case map[string]any:
		if typ, ok := x["type"]; ok {
			if arr, ok := typ.([]any); ok {
				if sliceContains(arr, "null") {
					t.Errorf("schema path %q has nullable/union type: %v", path, arr)
				}
			}
		}
		if props, ok := x["properties"].(map[string]any); ok {
			for k, child := range props {
				childPath := k
				if path != "" {
					childPath = path + "." + k
				}
				findUnionTypes(t, childPath, child)
			}
		}
		if items, ok := x["items"]; ok {
			findUnionTypes(t, path+"[]", items)
		}
	case []any:
		for i, child := range x {
			findUnionTypes(t, path+"["+string(rune('0'+i))+"]", child)
		}
	}
}

func structJSONKeys(typ reflect.Type) []string {
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	var keys []string
	for i := 0; i < typ.NumField(); i++ {
		name := jsonTagName(typ.Field(i).Tag.Get("json"))
		if name == "-" || name == "" {
			continue
		}
		keys = append(keys, name)
	}
	return keys
}

func jsonTagName(tag string) string {
	if idx := strings.Index(tag, ","); idx != -1 {
		return tag[:idx]
	}
	return tag
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sliceContains(arr []any, s string) bool {
	for _, e := range arr {
		if str, ok := e.(string); ok && str == s {
			return true
		}
	}
	return false
}
