package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// helpOrderSpecServer serves one operation with a body, a success response and
// error responses, so the help layout can be asserted without depending on the
// live spec (which CI machines do not have cached).
func helpOrderSpecServer(t *testing.T) *httptest.Server {
	t.Helper()
	spec := `{
  "openapi": "3.0.0",
  "info": {"title": "Surf Test API", "version": "1.0.0"},
  "paths": {
    "/gateway/v1/helpOrder": {
      "post": {
        "operationId": "help-order",
        "summary": "Help order fixture",
        "description": "Prose that belongs at the top.",
        "tags": ["test"],
        "parameters": [
          {"name": "address", "in": "query", "schema": {"type": "string"}}
        ],
        "requestBody": {
          "content": {
            "application/json": {
              "schema": {"type": "object", "properties": {"sql": {"type": "string"}}},
              "example": {"sql": "SELECT 1"}
            }
          }
        },
        "responses": {
          "200": {
            "description": "OK",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "data": {
                      "type": "array",
                      "items": {"type": "object", "properties": {"id": {"type": "string", "description": "Article ID"}}}
                    }
                  }
                }
              }
            }
          },
          "408": {
            "description": "Timed out",
            "content": {
              "application/json": {
                "schema": {"type": "object", "properties": {"recommended_action": {"type": "string"}}}
              }
            }
          },
          "default": {
            "description": "Error",
            "content": {
              "application/json": {
                "schema": {"type": "object", "properties": {"error": {"type": "object", "properties": {"code": {"type": "string"}}}}}
              }
            }
          }
        }
      }
    }
  }
}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gateway/openapi.json" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, spec)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestHelpPutsFlagsBeforeResponseSchema asserts the ordering users and agents
// actually see in the final --help output, using a stub spec so it runs in CI.
func TestHelpPutsFlagsBeforeResponseSchema(t *testing.T) {
	bin := buildBareSurfBin(t)
	home := t.TempDir()
	srv := helpOrderSpecServer(t)
	seedSurfSpecCache(t, bin, home, srv.URL+"/gateway", srv.URL+"/gateway/openapi.json")

	cmd := surfCmd(t, bin, home, nil, "help-order", "--help")
	stdout, stderr := splitOutput(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatalf("--help failed: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}
	out := stdout.String() + stderr.String()

	usage := strings.Index(out, "\nUsage:")
	flags := strings.Index(out, "\nFlags:")
	responseBlock := strings.Index(out, "Format of the response:")
	response200 := strings.Index(out, "## Response 200")
	response408 := strings.Index(out, "## Response 408")
	responseDefault := strings.Index(out, "## Response default")
	inputExample := strings.Index(out, "## Input Example")

	if usage < 0 || flags < 0 {
		t.Fatalf("--help has no Usage/Flags section:\n%s", out)
	}
	if response200 < 0 || responseBlock < 0 {
		t.Fatalf("--help has no success-response block:\n%s", out)
	}

	// Flags come before the success schema, which comes after them.
	if flags > response200 {
		t.Errorf("Flags rendered after the response schema (flags@%d, response@%d):\n%s", flags, response200, out)
	}
	if responseBlock < flags {
		t.Errorf("response block should follow Flags (block@%d, flags@%d):\n%s", responseBlock, flags, out)
	}

	// The flag needed to construct the call is visible before the schema.
	if pos := strings.Index(out, "--address"); pos < 0 || pos > responseBlock {
		t.Errorf("--address should appear before the response block (pos=%d, block=%d):\n%s", pos, responseBlock, out)
	}

	// Body-type input example stays above Usage.
	if inputExample < 0 || inputExample > usage {
		t.Errorf("## Input Example must stay above Usage (pos=%d, usage=%d):\n%s", inputExample, usage, out)
	}

	// Every response schema renders after Usage/Flags: success first, then
	// error codes, then the shared default envelope.
	for name, pos := range map[string]int{"default": responseDefault, "408": response408} {
		if pos < 0 {
			t.Errorf("## Response %s missing from --help:\n%s", name, out)
			continue
		}
		if pos < responseBlock {
			t.Errorf("## Response %s should follow the response block (pos=%d, block=%d)", name, pos, responseBlock)
		}
	}
	if response408 < response200 || responseDefault < response408 {
		t.Errorf("response order should be 200, 408, default (200@%d, 408@%d, default@%d)",
			response200, response408, responseDefault)
	}
}
