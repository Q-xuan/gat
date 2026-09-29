package transport

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
)

// HTTP 发送一次 HTTP 请求。
type HTTP struct {
	Method string
	URL    string
	Header http.Header
	Client *http.Client
}

func (h HTTP) Send(ctx context.Context, payload []byte) ([]byte, int, error) {
	var body io.Reader
	if len(payload) > 0 {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, h.Method, h.URL, body)
	if err != nil {
		return nil, 0, errors.New("创建请求失败")
	}
	for key, values := range h.Header {
		req.Header[key] = append([]string(nil), values...)
	}
	client := h.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, errors.New("请求失败")
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, errors.New("读取响应失败")
	}
	if len(data) > maxResponseBytes {
		return nil, resp.StatusCode, errors.New("响应超过 1MiB")
	}
	return data, resp.StatusCode, nil
}
