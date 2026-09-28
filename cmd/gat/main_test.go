package main

import (
	"bytes"
	"strings"
	"testing"
)

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
