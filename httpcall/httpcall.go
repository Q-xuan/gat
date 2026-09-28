package httpcall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Q-xuan/gat/redact"
)

const maxResponseBytes = 1 << 20

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// Spec 描述一次 HTTP 调用。地址、头和正文由使用方提供。
type Spec struct {
	Method       string            `json:"method"`
	URL          string            `json:"url"`
	Headers      map[string]string `json:"headers,omitempty"`
	Body         json.RawMessage   `json:"body,omitempty"`
	ExpectStatus int               `json:"expect_status,omitempty"`
	Timeout      string            `json:"timeout,omitempty"`
}

// Report 是脱敏后的结果。不包含请求头。
type Report struct {
	OK     bool   `json:"ok"`
	Status int    `json:"status,omitempty"`
	Body   any    `json:"body,omitempty"`
	Error  string `json:"error,omitempty"`
}

func LoadSpec(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, fmt.Errorf("读取 HTTP 说明: %w", err)
	}
	var spec Spec
	if err := json.Unmarshal(data, &spec); err != nil {
		return Spec{}, fmt.Errorf("解析 HTTP 说明: %w", err)
	}
	return spec, nil
}

// Do 发出请求并返回脱敏后的报告。substituted 里的值会从正文里替换掉。
func Do(ctx context.Context, spec Spec, policy redact.Policy, client *http.Client) (Report, error) {
	method := strings.ToUpper(strings.TrimSpace(spec.Method))
	if method == "" {
		method = http.MethodGet
	}
	if !validMethod(method) {
		return Report{Error: "method 无法识别"}, fmt.Errorf("method 无法识别")
	}
	rawURL, secrets := expand(strings.TrimSpace(spec.URL))
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return Report{Error: "url 必须是 http 或 https"}, fmt.Errorf("url 必须是 http 或 https")
	}
	body, bodySecrets, err := requestBody(spec.Body)
	if err != nil {
		return Report{Error: err.Error()}, err
	}
	secrets = append(secrets, bodySecrets...)
	timeout := 15 * time.Second
	if strings.TrimSpace(spec.Timeout) != "" {
		timeout, err = time.ParseDuration(spec.Timeout)
		if err != nil || timeout <= 0 {
			return Report{Error: "timeout 无法识别"}, fmt.Errorf("timeout 无法识别")
		}
	}
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, rawURL, bytes.NewReader(body))
	if err != nil {
		return Report{Error: "创建请求失败"}, fmt.Errorf("创建请求失败")
	}
	for key, value := range spec.Headers {
		expanded, extra := expand(value)
		secrets = append(secrets, extra...)
		req.Header.Set(key, expanded)
	}
	resp, err := client.Do(req)
	if err != nil {
		return Report{Error: "请求失败"}, fmt.Errorf("请求失败")
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return Report{Status: resp.StatusCode, Error: "读取响应失败"}, fmt.Errorf("读取响应失败")
	}
	if len(payload) > maxResponseBytes {
		return Report{Status: resp.StatusCode, Error: "响应超过 1MiB"}, fmt.Errorf("响应超过 1MiB")
	}
	report := Report{OK: true, Status: resp.StatusCode, Body: redactBody(policy, payload, secrets)}
	if spec.ExpectStatus != 0 && resp.StatusCode != spec.ExpectStatus {
		report.OK = false
		report.Error = fmt.Sprintf("status=%d，期望 %d", resp.StatusCode, spec.ExpectStatus)
		return report, fmt.Errorf("%s", report.Error)
	}
	return report, nil
}

func requestBody(raw json.RawMessage) ([]byte, []string, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(raw) == "null" {
		return nil, nil, nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		expanded, secrets := expand(text)
		return []byte(expanded), secrets, nil
	}
	expanded, secrets := expand(string(raw))
	return []byte(expanded), secrets, nil
}

func redactBody(policy redact.Policy, payload []byte, secrets []string) any {
	var parsed any
	if json.Unmarshal(payload, &parsed) == nil {
		return redact.Apply(policy, parsed, secrets)
	}
	return redact.Apply(policy, string(payload), secrets)
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

func validMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead:
		return true
	default:
		return false
	}
}
