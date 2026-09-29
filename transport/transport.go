package transport

import "context"

const maxResponseBytes = 1 << 20

// Sender 发送已经编好的字节，并返回响应字节。status 只对 HTTP 有意义。
type Sender interface {
	Send(ctx context.Context, payload []byte) (body []byte, status int, err error)
}
