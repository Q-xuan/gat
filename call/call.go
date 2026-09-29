package call

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/Q-xuan/gat/codec"
	"github.com/Q-xuan/gat/payload"
	"github.com/Q-xuan/gat/redact"
	"github.com/Q-xuan/gat/transport"
)

const defaultTimeout = 15 * time.Second

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Spec 描述一次收发。transport 取 http 或 ws，payload 取 json 或 pb。
// 地址、请求头、descriptor 和帧 profile 都由使用方提供。
type Spec struct {
	Transport    string            `json:"transport"`
	Payload      string            `json:"payload"`
	Method       string            `json:"method,omitempty"`
	URL          string            `json:"url"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         json.RawMessage   `json:"body,omitempty"`
	ExpectStatus int               `json:"expect_status,omitempty"`
	Timeout      string            `json:"timeout,omitempty"`
	Binary       *bool             `json:"binary,omitempty"`
	PB           *PBSpec           `json:"pb,omitempty"`
	Frame        *FrameSpec        `json:"frame,omitempty"`
}

// PBSpec 指向使用方自己的 FileDescriptorSet 和消息名。
type PBSpec struct {
	Descriptor string `json:"descriptor"`
	Request    string `json:"request"`
	Response   string `json:"response,omitempty"`
}

// FrameSpec 用已有 profile 在正文外包一层二进制帧。
type FrameSpec struct {
	Profile  string            `json:"profile"`
	Opcode   uint32            `json:"opcode"`
	Seq      uint32            `json:"seq"`
	Ret      uint32            `json:"ret,omitempty"`
	Identity map[string]uint64 `json:"identity,omitempty"`
	Raw      map[string]uint64 `json:"raw,omitempty"`
}

// Report 是脱敏后的结果。不包含请求头和身份字段。
type Report struct {
	OK     bool       `json:"ok"`
	Status int        `json:"status,omitempty"`
	Body   any        `json:"body,omitempty"`
	Frame  *FrameView `json:"frame,omitempty"`
	Error  string     `json:"error,omitempty"`
}

// FrameView 只保留分类所需的帧字段。
type FrameView struct {
	Kind   string `json:"kind"`
	Opcode uint32 `json:"opcode"`
	Seq    uint32 `json:"seq"`
	HasSeq bool   `json:"has_seq"`
	Ret    uint32 `json:"ret"`
	HasRet bool   `json:"has_ret"`
}

// Prepare 给 gat http 补上缺省的 http + json。gat call 保持调用方写明的组合。
func (s *Spec) Prepare(command string) error {
	if command != "http" {
		return nil
	}
	if s.Transport == "" {
		s.Transport = "http"
	}
	if s.Transport != "http" {
		return errors.New("gat http 只发送 HTTP")
	}
	if s.Payload == "" {
		s.Payload = "json"
	}
	return nil
}

func Load(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, fmt.Errorf("读取调用说明: %w", err)
	}
	var spec Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return Spec{}, fmt.Errorf("解析调用说明: %w", err)
	}
	return spec, nil
}

// Do 按 transport 和 payload 完成一次收发。baseDir 用来解析相对的 descriptor 和 profile。
func Do(ctx context.Context, spec Spec, policy redact.Policy, baseDir string, client *http.Client) (Report, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var secrets []string
	timeout := defaultTimeout
	if strings.TrimSpace(spec.Timeout) != "" {
		parsed, err := time.ParseDuration(spec.Timeout)
		if err != nil || parsed <= 0 {
			return fail(Report{}, nil, "timeout 无法识别")
		}
		timeout = parsed
	}
	if spec.Transport != "http" && spec.Transport != "ws" {
		return fail(Report{}, nil, "transport 取 http 或 ws")
	}
	if spec.Payload != "json" && spec.Payload != "pb" {
		return fail(Report{}, nil, "payload 取 json 或 pb")
	}
	if spec.Transport == "ws" && spec.ExpectStatus != 0 {
		return fail(Report{}, nil, "ws 没有 status")
	}
	rawURL, extra := expand(strings.TrimSpace(spec.URL))
	secrets = append(secrets, extra...)
	if err := checkURL(spec.Transport, rawURL); err != nil {
		return fail(Report{}, secrets, err.Error())
	}
	method := ""
	if spec.Transport == "http" {
		var err error
		method, err = normalizeMethod(spec.Method)
		if err != nil {
			return fail(Report{}, secrets, err.Error())
		}
	}
	codecImpl, err := newCodec(spec, baseDir)
	if err != nil {
		return fail(Report{}, secrets, err.Error())
	}
	body, extra, err := prepareBody(spec.Body)
	secrets = append(secrets, extra...)
	if err != nil {
		return fail(Report{}, secrets, err.Error())
	}
	encoded, err := codecImpl.Encode(body)
	if err != nil {
		return fail(Report{}, secrets, err.Error())
	}
	wire, framed, extra, err := wrapFrame(spec, baseDir, encoded)
	secrets = append(secrets, extra...)
	if err != nil {
		return fail(Report{}, secrets, err.Error())
	}
	binary, err := binaryMode(spec)
	if err != nil {
		return fail(Report{}, secrets, err.Error())
	}
	header, extra, err := expandHeaders(spec.Headers)
	secrets = append(secrets, extra...)
	if err != nil {
		return fail(Report{}, secrets, err.Error())
	}
	applyContentType(spec, header, wire)
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	sender := senderFor(spec, method, rawURL, header, client, binary)
	raw, status, err := sender.Send(reqCtx, wire)
	report := Report{}
	if spec.Transport == "http" {
		report.Status = status
	}
	if err != nil {
		return fail(report, secrets, err.Error())
	}
	decodedWire, view, err := unwrap(framed, raw)
	if err != nil {
		return fail(report, secrets, err.Error())
	}
	report.Frame = view
	parsed, err := codecImpl.Decode(decodedWire)
	if err != nil {
		return fail(report, secrets, err.Error())
	}
	if parsed != nil {
		report.Body = redact.Apply(policy, parsed, secrets)
	}
	report.OK = true
	if spec.Transport == "http" && spec.ExpectStatus != 0 && status != spec.ExpectStatus {
		report.OK = false
		report.Error = scrub(fmt.Sprintf("status=%d，期望 %d", status, spec.ExpectStatus), secrets)
		return report, errors.New(report.Error)
	}
	return report, nil
}

func senderFor(spec Spec, method, rawURL string, header http.Header, client *http.Client, binary bool) transport.Sender {
	if spec.Transport == "ws" {
		return transport.WS{URL: rawURL, Header: header, Binary: binary}
	}
	return transport.HTTP{Method: method, URL: rawURL, Header: header, Client: client}
}

func newCodec(spec Spec, baseDir string) (payload.Codec, error) {
	if spec.Payload == "json" {
		return payload.JSON{}, nil
	}
	if spec.PB == nil || strings.TrimSpace(spec.PB.Descriptor) == "" || strings.TrimSpace(spec.PB.Request) == "" {
		return nil, errors.New("pb 需要 descriptor 和 request")
	}
	return payload.LoadPB(resolve(baseDir, spec.PB.Descriptor), spec.PB.Request, spec.PB.Response)
}

type frameState struct {
	on      bool
	profile codec.Profile
	key     []byte
}

func wrapFrame(spec Spec, baseDir string, body []byte) ([]byte, frameState, []string, error) {
	if spec.Frame == nil {
		return body, frameState{}, nil, nil
	}
	if strings.TrimSpace(spec.Frame.Profile) == "" {
		return nil, frameState{}, nil, errors.New("frame 需要 profile")
	}
	profile, err := codec.LoadProfile(resolve(baseDir, spec.Frame.Profile))
	if err != nil {
		return nil, frameState{}, nil, err
	}
	key, err := signKey(profile)
	if err != nil {
		return nil, frameState{}, nil, err
	}
	var secrets []string
	if len(key) > 0 {
		secrets = append(secrets, string(key))
	}
	wire, err := codec.Encode(profile, codec.Frame{
		Opcode:   spec.Frame.Opcode,
		Seq:      spec.Frame.Seq,
		Ret:      spec.Frame.Ret,
		Identity: spec.Frame.Identity,
		Raw:      spec.Frame.Raw,
		Payload:  body,
	}, key)
	if err != nil {
		return nil, frameState{}, secrets, fmt.Errorf("组帧: %w", err)
	}
	return wire, frameState{on: true, profile: profile, key: key}, secrets, nil
}

func unwrap(state frameState, data []byte) ([]byte, *FrameView, error) {
	if !state.on {
		return data, nil, nil
	}
	frame, err := codec.DecodeExact(state.profile, data, state.key, state.profile.HasSign())
	if err != nil {
		return nil, nil, fmt.Errorf("拆帧: %w", err)
	}
	return frame.Payload, &FrameView{
		Kind:   frame.Kind,
		Opcode: frame.Opcode,
		Seq:    frame.Seq,
		HasSeq: frame.HasSeq,
		Ret:    frame.Ret,
		HasRet: frame.HasRet,
	}, nil
}

func binaryMode(spec Spec) (bool, error) {
	if spec.Transport != "ws" {
		return false, nil
	}
	if spec.Frame != nil {
		if spec.Binary != nil && !*spec.Binary {
			return false, errors.New("带 frame 时必须使用二进制")
		}
		return true, nil
	}
	if spec.Binary != nil {
		return *spec.Binary, nil
	}
	return spec.Payload == "pb", nil
}

func applyContentType(spec Spec, header http.Header, wire []byte) {
	if spec.Transport != "http" || len(wire) == 0 || header.Get("Content-Type") != "" {
		return
	}
	switch {
	case spec.Frame != nil:
		header.Set("Content-Type", "application/octet-stream")
	case spec.Payload == "pb":
		header.Set("Content-Type", "application/protobuf")
	default:
		header.Set("Content-Type", "application/json")
	}
}

func signKey(profile codec.Profile) ([]byte, error) {
	if profile.Sign == nil {
		return nil, nil
	}
	key := os.Getenv(profile.Sign.KeyEnv)
	if key == "" {
		return nil, fmt.Errorf("需要环境变量 %s", profile.Sign.KeyEnv)
	}
	return []byte(key), nil
}

func prepareBody(raw json.RawMessage) ([]byte, []string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil, nil
	}
	var text string
	if err := json.Unmarshal(trimmed, &text); err == nil {
		expanded, secrets := expand(text)
		return []byte(expanded), secrets, nil
	}
	expanded, secrets := expand(string(trimmed))
	if !json.Valid([]byte(expanded)) {
		return nil, secrets, errors.New("body 必须是 JSON")
	}
	return []byte(expanded), secrets, nil
}

func expandHeaders(in map[string]string) (http.Header, []string, error) {
	out := make(http.Header, len(in))
	var secrets []string
	for key, value := range in {
		if strings.TrimSpace(key) == "" {
			return nil, secrets, errors.New("header 名不能为空")
		}
		expanded, extra := expand(value)
		secrets = append(secrets, extra...)
		out.Set(key, expanded)
	}
	return out, secrets, nil
}

func expand(text string) (string, []string) {
	var secrets []string
	out := envPattern.ReplaceAllStringFunc(text, func(match string) string {
		name := envPattern.FindStringSubmatch(match)[1]
		value := os.Getenv(name)
		if value != "" {
			secrets = append(secrets, value)
		}
		return value
	})
	return out, secrets
}

func checkURL(transportName, raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		if transportName == "ws" {
			return errors.New("url 必须是 ws 或 wss")
		}
		return errors.New("url 必须是 http 或 https")
	}
	switch transportName {
	case "http":
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return errors.New("url 必须是 http 或 https")
		}
	case "ws":
		if parsed.Scheme != "ws" && parsed.Scheme != "wss" {
			return errors.New("url 必须是 ws 或 wss")
		}
	}
	return nil
}

func normalizeMethod(method string) (string, error) {
	method = strings.ToUpper(strings.TrimSpace(method))
	if method == "" {
		method = http.MethodGet
	}
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
		return method, nil
	default:
		return "", errors.New("method 无法识别")
	}
}

func resolve(base, path string) string {
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		return path
	}
	if strings.TrimSpace(base) == "" {
		base = "."
	}
	return filepath.Clean(filepath.Join(base, path))
}

func fail(report Report, secrets []string, text string) (Report, error) {
	report.OK = false
	report.Error = scrub(text, secrets)
	return report, errors.New(report.Error)
}

func scrub(text string, secrets []string) string {
	for _, secret := range secrets {
		if len(secret) < 4 {
			continue
		}
		text = strings.ReplaceAll(text, secret, "<redacted>")
	}
	return text
}
