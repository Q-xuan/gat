package payload

import (
	"bytes"
	"encoding/json"
)

// Codec 把调用说明里的 JSON 正文编成线上字节，再把响应字节解回可脱敏的值。
type Codec interface {
	Encode(body []byte) ([]byte, error)
	Decode(wire []byte) (any, error)
}

// JSON 原样发送 JSON 字节。响应对应 JSON 时解成对象，否则保留为字符串。
type JSON struct{}

func (JSON) Encode(body []byte) ([]byte, error) {
	return body, nil
}

func (JSON) Decode(wire []byte) (any, error) {
	if len(bytes.TrimSpace(wire)) == 0 {
		return nil, nil
	}
	var parsed any
	if err := json.Unmarshal(wire, &parsed); err != nil {
		return string(wire), nil
	}
	return parsed, nil
}
