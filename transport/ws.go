package transport

import (
	"context"
	"errors"
	"net/http"

	"github.com/coder/websocket"
)

// WS 发送一条 WebSocket 消息并读回一条。
type WS struct {
	URL    string
	Header http.Header
	Binary bool
}

func (w WS) Send(ctx context.Context, payload []byte) ([]byte, int, error) {
	conn, resp, err := websocket.Dial(ctx, w.URL, &websocket.DialOptions{HTTPHeader: w.Header})
	if resp != nil && resp.Body != nil {
		resp.Body.Close()
	}
	if err != nil {
		return nil, 0, errors.New("连接失败")
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(maxResponseBytes + 1)
	kind := websocket.MessageText
	if w.Binary {
		kind = websocket.MessageBinary
	}
	if err := conn.Write(ctx, kind, payload); err != nil {
		return nil, 0, errors.New("发送失败")
	}
	_, data, err := conn.Read(ctx)
	if err != nil {
		return nil, 0, errors.New("读取失败")
	}
	if len(data) > maxResponseBytes {
		return nil, 0, errors.New("响应超过 1MiB")
	}
	return data, 0, nil
}
