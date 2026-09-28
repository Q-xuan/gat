package redact

import "testing"

func TestApplyDropsPathsAndMasksSecrets(t *testing.T) {
	policy := Policy{
		DropKeySubstrings:   []string{"bag"},
		RedactKeySubstrings: []string{"token"},
		OmitPaths:           []string{"meta.note"},
		Fingerprint:         "sha256-8",
		Presets:             []string{"credentials"},
	}
	in := map[string]any{
		"token":    "abcd-token",
		"password": "hidden",
		"bag":      map[string]any{"id": 1},
		"meta":     map[string]any{"note": "x", "keep": "session-abcd-token-value"},
		"text":     "prefix abcd-token suffix",
	}
	got := Apply(policy, in, []string{"abcd-token"}).(map[string]any)
	if _, ok := got["bag"]; ok {
		t.Fatal("bag key remained")
	}
	if _, ok := got["meta"].(map[string]any)["note"]; ok {
		t.Fatal("omit path remained")
	}
	if got["token"] != "<redacted>" || got["password"] != "<redacted>" {
		t.Fatalf("keys %+v", got)
	}
	text := got["text"].(string)
	if text == "prefix abcd-token suffix" || len(text) < 10 {
		t.Fatalf("text %q", text)
	}
	keep := got["meta"].(map[string]any)["keep"].(string)
	if keep == "session-abcd-token-value" {
		t.Fatal("secret remained in nested text")
	}
}
