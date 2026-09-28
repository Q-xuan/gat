package httpcall

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Q-xuan/gat/redact"
)

func TestDoRedactsBodyAndHidesHeaderSecret(t *testing.T) {
	const secret = "secret-value"
	t.Setenv("DEMO_TOKEN", secret)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"token":"secret-value","ok":true,"note":"sees secret-value"}`))
	}))
	defer server.Close()

	report, err := Do(context.Background(), Spec{
		Method:       "POST",
		URL:          server.URL + "/check",
		Headers:      map[string]string{"Authorization": "Bearer ${DEMO_TOKEN}"},
		Body:         json.RawMessage(`{"name":"sample"}`),
		ExpectStatus: http.StatusOK,
	}, redact.Policy{Presets: []string{"credentials"}, Fingerprint: "sha256-8"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || report.Status != http.StatusOK {
		t.Fatalf("%+v", report)
	}
	body := report.Body.(map[string]any)
	if body["token"] != "<redacted>" || body["ok"] != true {
		t.Fatalf("body %#v", body)
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("report leaked secret: %s", encoded)
	}
	if body["note"] == "sees secret-value" {
		t.Fatal("secret remained in note")
	}
}

func TestStatusMismatchStillReturnsRedactedBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"password":"hidden-pass"}`))
	}))
	defer server.Close()
	report, err := Do(context.Background(), Spec{URL: server.URL, ExpectStatus: http.StatusOK}, redact.Policy{Presets: []string{"credentials"}}, server.Client())
	if err == nil || report.OK || report.Status != http.StatusNotFound {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	if report.Body.(map[string]any)["password"] != "<redacted>" {
		t.Fatalf("%#v", report.Body)
	}
}

func TestRejectsNonHTTPURL(t *testing.T) {
	_, err := Do(context.Background(), Spec{URL: "file:///tmp/x"}, redact.Policy{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
