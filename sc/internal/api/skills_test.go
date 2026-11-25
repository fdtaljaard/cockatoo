package api

import (
	"net/http"
	"strings"
	"testing"
)

// Skills: the starters, a new one, a new version, a removal; bad input is refused.
func TestSkillRoutes(t *testing.T) {
	h, _ := newApp(t)
	if code, b := do(t, h, "GET", "/api/skills", nil); code != 200 || !strings.Contains(b, `"What I keep"`) {
		t.Fatalf("starters: %d %s", code, b)
	}
	code, b := do(t, h, "POST", "/api/skills", map[string]any{"name": "  Shops  I buy from ", "applies": "shops", "instructions": "Invoices to Save."})
	if code != 200 || !strings.Contains(b, `"name":"Shops I buy from"`) || !strings.Contains(b, `"madeBy":"you"`) {
		t.Fatalf("new: %d %s", code, b)
	}
	if code, b := do(t, h, "POST", "/api/skills", map[string]any{"id": 3, "name": "Shops", "instructions": "Invoices to Save. Shipping notices need nothing.", "fromAgent": true}); code != 200 || !strings.Contains(b, `"madeBy":"agent"`) {
		t.Fatalf("approved proposal: %d %s", code, b)
	}
	if code, _ := do(t, h, "POST", "/api/skills", map[string]any{"name": "", "instructions": "x"}); code != http.StatusBadRequest {
		t.Fatalf("no name: %d", code)
	}
	if code, _ := do(t, h, "POST", "/api/skills", map[string]any{"id": 77, "name": "x", "instructions": "x"}); code != http.StatusNotFound {
		t.Fatalf("unknown skill: %d", code)
	}
	if code, _ := do(t, h, "DELETE", "/api/skills/1", nil); code != http.StatusNoContent {
		t.Fatalf("remove: %d", code)
	}
	if _, b := do(t, h, "GET", "/api/skills", nil); strings.Contains(b, "What I keep") || !strings.Contains(b, "Shipping notices") {
		t.Fatalf("after: %s", b)
	}
	if code, _ := do(t, h, "POST", "/api/teach", map[string]any{"from": "save", "to": "save"}); code != http.StatusBadRequest {
		t.Fatalf("teach nothing: %d", code)
	}
}
