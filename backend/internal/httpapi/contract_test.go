package httpapi

import (
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/The-Ultimate-Developer/card-dispute-portal/backend/api"
)

// TestEveryEndpointIsDocumented keeps the OpenAPI contract and the router in
// lockstep in both directions: no undocumented endpoints, no documented ghosts.
func TestEveryEndpointIsDocumented(t *testing.T) {
	var spec struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(api.OpenAPI, &spec); err != nil {
		t.Fatalf("openapi.yaml is not valid YAML: %v", err)
	}

	routed := map[string]bool{}
	for _, e := range endpoints {
		key := strings.ToLower(e.method) + " " + e.path
		routed[key] = true
		if _, ok := spec.Paths[e.path][strings.ToLower(e.method)]; !ok {
			t.Errorf("%s %s is routed but not documented", e.method, e.path)
		}
	}

	methods := []string{"get", "post", "put", "patch", "delete"}
	for path, ops := range spec.Paths {
		if !strings.HasPrefix(path, "/api/") {
			continue
		}
		for method := range ops {
			if slices.Contains(methods, method) && !routed[method+" "+path] {
				t.Errorf("%s %s is documented but not routed", strings.ToUpper(method), path)
			}
		}
	}
}
