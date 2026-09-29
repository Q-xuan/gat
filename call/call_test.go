package call

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Q-xuan/gat/redact"
	"github.com/coder/websocket"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

func TestDoRedactsBodyAndHidesHeaderSecret(t *testing.T) {
	const secret = "secret-value"
	t.Setenv("DEMO_TOKEN", secret)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		if r.Method != http.MethodPost {
			t.Errorf("method = %s", r.Method)
		}
		_, _ = w.Write([]byte(`{"token":"secret-value","ok":true,"note":"sees secret-value"}`))
	}))
	defer server.Close()

	report, err := Do(context.Background(), Spec{
		Transport:    "http",
		Payload:      "json",
		Method:       "POST",
		URL:          server.URL + "/check",
		Headers:      map[string]string{"Authorization": "Bearer ${DEMO_TOKEN}"},
		Body:         json.RawMessage(`{"name":"sample"}`),
		ExpectStatus: http.StatusOK,
	}, redact.Policy{Presets: []string{"credentials"}, Fingerprint: "sha256-8"}, ".", server.Client())
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
	report, err := Do(context.Background(), Spec{
		Transport:    "http",
		Payload:      "json",
		URL:          server.URL,
		ExpectStatus: http.StatusOK,
	}, redact.Policy{Presets: []string{"credentials"}}, ".", server.Client())
	if err == nil || report.OK || report.Status != http.StatusNotFound {
		t.Fatalf("err=%v report=%+v", err, report)
	}
	if report.Body.(map[string]any)["password"] != "<redacted>" {
		t.Fatalf("%#v", report.Body)
	}
}

func TestRejectsNonHTTPURL(t *testing.T) {
	_, err := Do(context.Background(), Spec{Transport: "http", Payload: "json", URL: "file:///tmp/x"}, redact.Policy{}, ".", nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRejectsUnknownCombination(t *testing.T) {
	_, err := Do(context.Background(), Spec{Transport: "tcp", Payload: "json", URL: "http://127.0.0.1"}, redact.Policy{}, ".", nil)
	if err == nil || !strings.Contains(err.Error(), "transport") {
		t.Fatalf("err %v", err)
	}
	_, err = Do(context.Background(), Spec{Transport: "http", Payload: "xml", URL: "http://127.0.0.1"}, redact.Policy{}, ".", nil)
	if err == nil || !strings.Contains(err.Error(), "payload") {
		t.Fatalf("err %v", err)
	}
}

func TestPrepareHTTPDefaults(t *testing.T) {
	spec := Spec{}
	if err := spec.Prepare("http"); err != nil {
		t.Fatal(err)
	}
	if spec.Transport != "http" || spec.Payload != "json" {
		t.Fatalf("%+v", spec)
	}
	ws := Spec{Transport: "ws", Payload: "json"}
	if err := ws.Prepare("http"); err == nil {
		t.Fatal("expected error")
	}
	explicit := Spec{Transport: "http", Payload: "pb"}
	if err := explicit.Prepare("http"); err != nil {
		t.Fatal(err)
	}
	if explicit.Payload != "pb" {
		t.Fatalf("%+v", explicit)
	}
}

func TestHTTPProtobufRoundTrip(t *testing.T) {
	md, descPath := writeDescriptor(t)
	const secret = "secret-value"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/protobuf" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		msg := dynamicpb.NewMessage(md)
		if err := proto.Unmarshal(data, msg); err != nil {
			t.Errorf("unmarshal request: %v", err)
			return
		}
		if msg.Get(md.Fields().ByName("name")).String() != "ada" {
			t.Errorf("name = %s", msg.Get(md.Fields().ByName("name")).String())
		}
		reply := dynamicpb.NewMessage(md)
		reply.Set(md.Fields().ByName("name"), protoreflect.ValueOfString("ada"))
		reply.Set(md.Fields().ByName("token"), protoreflect.ValueOfString(secret))
		out, err := proto.Marshal(reply)
		if err != nil {
			t.Errorf("marshal reply: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/protobuf")
		_, _ = w.Write(out)
	}))
	defer server.Close()

	report, err := Do(context.Background(), Spec{
		Transport:    "http",
		Payload:      "pb",
		Method:       "POST",
		URL:          server.URL,
		Body:         json.RawMessage(`{"name":"ada","token":"secret-value"}`),
		ExpectStatus: http.StatusOK,
		PB:           &PBSpec{Descriptor: descPath, Request: "sample.Ping"},
	}, redact.Policy{Presets: []string{"credentials"}}, ".", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	body := report.Body.(map[string]any)
	if body["name"] != "ada" || body["token"] != "<redacted>" {
		t.Fatalf("%#v", body)
	}
	encoded, _ := json.Marshal(report)
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("report leaked secret: %s", encoded)
	}
}

func TestWebSocketJSON(t *testing.T) {
	url := newWS(t, func(t *testing.T, data []byte) []byte {
		if string(data) != `{"name":"sample"}` {
			t.Errorf("payload %s", data)
		}
		return []byte(`{"name":"sample","ok":true}`)
	})
	report, err := Do(context.Background(), Spec{
		Transport: "ws",
		Payload:   "json",
		URL:       url,
		Body:      json.RawMessage(`{"name":"sample"}`),
	}, redact.Policy{}, ".", nil)
	if err != nil {
		t.Fatal(err)
	}
	body := report.Body.(map[string]any)
	if body["name"] != "sample" || body["ok"] != true {
		t.Fatalf("%#v", body)
	}
	if report.Status != 0 {
		t.Fatalf("status %d", report.Status)
	}
}

func TestWebSocketProtobuf(t *testing.T) {
	md, descPath := writeDescriptor(t)
	url := newWS(t, func(t *testing.T, data []byte) []byte {
		msg := dynamicpb.NewMessage(md)
		if err := proto.Unmarshal(data, msg); err != nil {
			t.Errorf("unmarshal: %v", err)
			return data
		}
		if msg.Get(md.Fields().ByName("name")).String() != "ada" {
			t.Errorf("name = %s", msg.Get(md.Fields().ByName("name")).String())
		}
		reply := dynamicpb.NewMessage(md)
		reply.Set(md.Fields().ByName("name"), protoreflect.ValueOfString("ada"))
		out, err := proto.Marshal(reply)
		if err != nil {
			t.Errorf("marshal: %v", err)
			return data
		}
		return out
	})
	report, err := Do(context.Background(), Spec{
		Transport: "ws",
		Payload:   "pb",
		URL:       url,
		Body:      json.RawMessage(`{"name":"ada"}`),
		PB:        &PBSpec{Descriptor: descPath, Request: "sample.Ping", Response: "sample.Ping"},
	}, redact.Policy{}, ".", nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Body.(map[string]any)["name"] != "ada" {
		t.Fatalf("%#v", report.Body)
	}
}

func TestWebSocketJSONFrame(t *testing.T) {
	profilePath, err := filepath.Abs(filepath.Join("..", "examples", "profile.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	url := newWS(t, func(t *testing.T, data []byte) []byte {
		if len(data) < 12 {
			t.Errorf("frame length %d", len(data))
		}
		return data
	})
	report, err := Do(context.Background(), Spec{
		Transport: "ws",
		Payload:   "json",
		URL:       url,
		Body:      json.RawMessage(`{"name":"sample"}`),
		Frame: &FrameSpec{
			Profile: profilePath,
			Opcode:  7,
			Seq:     3,
		},
	}, redact.Policy{}, ".", nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Body.(map[string]any)["name"] != "sample" {
		t.Fatalf("%#v", report.Body)
	}
	if report.Frame == nil || report.Frame.Kind != "request" || report.Frame.Opcode != 7 || report.Frame.Seq != 3 {
		t.Fatalf("%+v", report.Frame)
	}
}

func TestPBRequiresDescriptor(t *testing.T) {
	_, err := Do(context.Background(), Spec{Transport: "http", Payload: "pb", URL: "http://127.0.0.1"}, redact.Policy{}, ".", nil)
	if err == nil || !strings.Contains(err.Error(), "descriptor") {
		t.Fatalf("err %v", err)
	}
}

func writeDescriptor(t *testing.T) (protoreflect.MessageDescriptor, string) {
	t.Helper()
	fd := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("sample.proto"),
		Package: proto.String("sample"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Ping"),
			Field: []*descriptorpb.FieldDescriptorProto{
				{
					Name:     proto.String("name"),
					JsonName: proto.String("name"),
					Number:   proto.Int32(1),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				},
				{
					Name:     proto.String("token"),
					JsonName: proto.String("token"),
					Number:   proto.Int32(2),
					Label:    descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(),
					Type:     descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
				},
			},
		}},
	}
	set := &descriptorpb.FileDescriptorSet{File: []*descriptorpb.FileDescriptorProto{fd}}
	files, err := protodesc.NewFiles(set)
	if err != nil {
		t.Fatal(err)
	}
	desc, err := files.FindDescriptorByName("sample.Ping")
	if err != nil {
		t.Fatal(err)
	}
	data, err := proto.Marshal(set)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "set.pb")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return desc.(protoreflect.MessageDescriptor), path
}

func newWS(t *testing.T, fn func(t *testing.T, data []byte) []byte) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "")
		conn.SetReadLimit(1 << 20)
		_, data, err := conn.Read(r.Context())
		if err != nil {
			return
		}
		reply := data
		if fn != nil {
			reply = fn(t, data)
		}
		_ = conn.Write(r.Context(), websocket.MessageBinary, reply)
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http")
}
