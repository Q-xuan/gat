package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPRequiresSpec(t *testing.T) {
	err := run([]string{"http"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--spec") {
		t.Fatalf("err %v", err)
	}
}

func TestCallRequiresSpec(t *testing.T) {
	err := run([]string{"call"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--spec") {
		t.Fatalf("err %v", err)
	}
}

func TestUsageListsCall(t *testing.T) {
	var stdout bytes.Buffer
	if err := run(nil, &stdout); err != nil {
		t.Fatal(err)
	}
	text := stdout.String()
	if !strings.Contains(text, "gat call --spec") || !strings.Contains(text, "gat http --spec") {
		t.Fatalf("usage %q", text)
	}
}

func TestHTTPDefaultsToJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	spec := filepath.Join(dir, "spec.json")
	if err := os.WriteFile(spec, []byte(fmt.Sprintf(`{"method":"GET","url":%q,"expect_status":200}`, server.URL)), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := run([]string{"http", "--spec", spec}, &stdout); err != nil {
		t.Fatalf("%v %s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), `"ok": true`) {
		t.Fatalf("stdout %s", stdout.String())
	}
}

func TestCheckPrintsSummaryWithoutConst(t *testing.T) {
	var stdout bytes.Buffer
	err := run([]string{"check", "--profile", "../../examples/profile.example.json"}, &stdout)
	if err != nil {
		t.Fatal(err)
	}
	text := stdout.String()
	if !strings.Contains(text, "header_bytes=12") || strings.Contains(text, "4660") {
		t.Fatalf("summary %q", text)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	var encoded bytes.Buffer
	err := run([]string{
		"encode", "--profile", "../../examples/profile.example.json",
		"--opcode", "7", "--seq", "3", "--payload", "010203",
	}, &encoded)
	if err != nil {
		t.Fatal(err)
	}
	hexText := strings.TrimSpace(encoded.String())
	var decoded bytes.Buffer
	err = run([]string{
		"decode", "--profile", "../../examples/profile.example.json",
		"--hex", hexText,
	}, &decoded)
	if err != nil {
		t.Fatal(err)
	}
	text := decoded.String()
	if !strings.Contains(text, "kind=request") || !strings.Contains(text, "opcode=7") || !strings.Contains(text, "payload=010203") {
		t.Fatalf("decoded %q", text)
	}
}
