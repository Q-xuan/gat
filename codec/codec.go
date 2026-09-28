package codec

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	KindRequest  = "request"
	KindResponse = "response"
	KindPush     = "push"
	KindMessage  = "message"
)

var (
	ErrShort = errors.New("帧长度不足")
	ErrConst = errors.New("常量字段不匹配")
	ErrSign  = errors.New("签名不匹配")
	ErrKey   = errors.New("缺少签名密钥")
)

// Frame 是一帧解码结果。身份字段和未命名整数按 profile 里的字段名放进 map。
type Frame struct {
	Kind     string
	Opcode   uint32
	Seq      uint32
	Ret      uint32
	HasSeq   bool
	HasRet   bool
	Identity map[string]uint64
	Raw      map[string]uint64
	Payload  []byte
}

type placed struct {
	Field
	offset int
	width  int
}

// Encode 按 profile 组一帧。签名密钥由调用方传入，本包没有默认密钥。
func Encode(profile Profile, frame Frame, key []byte) ([]byte, error) {
	layout, err := layoutOf(profile)
	if err != nil {
		return nil, err
	}
	if len(frame.Payload) > profile.maxPayload() {
		return nil, fmt.Errorf("消息体超过 %d 字节", profile.maxPayload())
	}
	var length uint64
	if profile.LengthBasis == "frame" {
		length = uint64(layout.size + len(frame.Payload))
	} else {
		length = uint64(len(frame.Payload))
	}
	buf := make([]byte, layout.size+len(frame.Payload))
	copy(buf[layout.size:], frame.Payload)
	if err := writeHeader(profile, layout, buf, frame, length); err != nil {
		return nil, err
	}
	if profile.Sign != nil {
		if len(key) == 0 {
			return nil, ErrKey
		}
		sum := xor32(buf[layout.signFrom:], key)
		putUint(buf[layout.signAt:], uint64(sum), 4, profile.Endian == "big")
	}
	return buf, nil
}

// Decode 解出第一帧并返回消费的字节数。数据不够时返回 ErrShort。
// verify 为 true 时用 key 校验签名。
func Decode(profile Profile, data, key []byte, verify bool) (Frame, int, error) {
	layout, err := layoutOf(profile)
	if err != nil {
		return Frame{}, 0, err
	}
	if len(data) < layout.size {
		return Frame{}, 0, ErrShort
	}
	frame, err := readHeader(profile, layout, data[:layout.size])
	if err != nil {
		return Frame{}, 0, err
	}
	lengthField := readField(profile, layout, data, "length")
	var total int
	if profile.LengthBasis == "frame" {
		if lengthField < uint64(layout.size) {
			return Frame{}, 0, fmt.Errorf("length 小于头部")
		}
		total = int(lengthField)
	} else {
		total = layout.size + int(lengthField)
	}
	payloadLen := total - layout.size
	if payloadLen > profile.maxPayload() {
		return Frame{}, 0, fmt.Errorf("消息体超过 %d 字节", profile.maxPayload())
	}
	if len(data) < total {
		return Frame{}, 0, ErrShort
	}
	frame.Payload = append([]byte(nil), data[layout.size:total]...)
	if verify && profile.Sign != nil {
		if len(key) == 0 {
			return Frame{}, 0, ErrKey
		}
		got := uint32(readField(profile, layout, data, "sign"))
		tmp := append([]byte(nil), data[:total]...)
		putUint(tmp[layout.signAt:], 0, 4, profile.Endian == "big")
		if xor32(tmp[layout.signFrom:], key) != got {
			return Frame{}, 0, ErrSign
		}
	}
	frame.Kind = classify(profile, frame)
	return frame, total, nil
}

// DecodeExact 要求 data 恰好是一帧。
func DecodeExact(profile Profile, data, key []byte, verify bool) (Frame, error) {
	frame, n, err := Decode(profile, data, key, verify)
	if err != nil {
		return Frame{}, err
	}
	if n != len(data) {
		return Frame{}, fmt.Errorf("帧后还有 %d 字节", len(data)-n)
	}
	return frame, nil
}

type layout struct {
	fields   []placed
	size     int
	signAt   int
	signFrom int
}

func layoutOf(profile Profile) (layout, error) {
	if err := profile.Validate(); err != nil {
		return layout{}, err
	}
	out := layout{}
	for _, field := range profile.Fields {
		n, _ := widthOf(field.Type)
		item := placed{Field: field, offset: out.size, width: n}
		if field.Role == "sign" {
			out.signAt = out.size
		}
		if profile.Sign != nil && field.Name == profile.Sign.From {
			out.signFrom = out.size
		}
		out.fields = append(out.fields, item)
		out.size += n
	}
	return out, nil
}

func writeHeader(profile Profile, layout layout, buf []byte, frame Frame, length uint64) error {
	big := profile.Endian == "big"
	for _, field := range layout.fields {
		var value uint64
		switch field.Role {
		case "const":
			value = *field.Const
		case "opcode":
			value = uint64(frame.Opcode)
		case "seq":
			value = uint64(frame.Seq)
		case "ret":
			value = uint64(frame.Ret)
		case "length":
			value = length
		case "sign":
			value = 0
		case "identity":
			value = frame.Identity[field.Name]
		case "raw":
			value = frame.Raw[field.Name]
		}
		if err := fits(value, field.Type); err != nil {
			return fmt.Errorf("字段 %s: %w", field.Name, err)
		}
		putUint(buf[field.offset:], value, field.width, big)
	}
	return nil
}

func readHeader(profile Profile, layout layout, header []byte) (Frame, error) {
	frame := Frame{}
	big := profile.Endian == "big"
	for _, field := range layout.fields {
		value := getUint(header[field.offset:field.offset+field.width], field.width, big)
		switch field.Role {
		case "const":
			if value != *field.Const {
				return Frame{}, fmt.Errorf("%w: %s", ErrConst, field.Name)
			}
		case "opcode":
			if err := fits(value, "uint32"); err != nil {
				return Frame{}, fmt.Errorf("opcode: %w", err)
			}
			frame.Opcode = uint32(value)
		case "seq":
			if err := fits(value, "uint32"); err != nil {
				return Frame{}, fmt.Errorf("seq: %w", err)
			}
			frame.Seq = uint32(value)
			frame.HasSeq = true
		case "ret":
			if err := fits(value, "uint32"); err != nil {
				return Frame{}, fmt.Errorf("ret: %w", err)
			}
			frame.Ret = uint32(value)
			frame.HasRet = true
		case "identity":
			if frame.Identity == nil {
				frame.Identity = map[string]uint64{}
			}
			frame.Identity[field.Name] = value
		case "raw":
			if frame.Raw == nil {
				frame.Raw = map[string]uint64{}
			}
			frame.Raw[field.Name] = value
		}
	}
	return frame, nil
}

func readField(profile Profile, layout layout, data []byte, role string) uint64 {
	big := profile.Endian == "big"
	for _, field := range layout.fields {
		if field.Role == role {
			return getUint(data[field.offset:field.offset+field.width], field.width, big)
		}
	}
	return 0
}

func classify(profile Profile, frame Frame) string {
	if profile.Classify != nil && profile.Classify.Push.hit(frame.HasSeq, frame.Seq, frame.Opcode) {
		return KindPush
	}
	if profile.Classify != nil && profile.Classify.Response.hit(frame.HasSeq, frame.Seq, frame.Opcode) {
		return KindResponse
	}
	if frame.HasSeq {
		return KindRequest
	}
	return KindMessage
}

func putUint(dst []byte, value uint64, width int, big bool) {
	switch width {
	case 1:
		dst[0] = byte(value)
	case 2:
		if big {
			binary.BigEndian.PutUint16(dst, uint16(value))
			return
		}
		binary.LittleEndian.PutUint16(dst, uint16(value))
	case 4:
		if big {
			binary.BigEndian.PutUint32(dst, uint32(value))
			return
		}
		binary.LittleEndian.PutUint32(dst, uint32(value))
	case 8:
		if big {
			binary.BigEndian.PutUint64(dst, value)
			return
		}
		binary.LittleEndian.PutUint64(dst, value)
	}
}

func getUint(src []byte, width int, big bool) uint64 {
	switch width {
	case 1:
		return uint64(src[0])
	case 2:
		if big {
			return uint64(binary.BigEndian.Uint16(src))
		}
		return uint64(binary.LittleEndian.Uint16(src))
	case 4:
		if big {
			return uint64(binary.BigEndian.Uint32(src))
		}
		return uint64(binary.LittleEndian.Uint32(src))
	case 8:
		if big {
			return binary.BigEndian.Uint64(src)
		}
		return binary.LittleEndian.Uint64(src)
	default:
		return 0
	}
}

// xor32 从 data 起点到末尾计算 32 位循环异或。data 应由调用方先把签名字段置零。
func xor32(data, key []byte) uint32 {
	if len(key) == 0 {
		return 0
	}
	var sign uint32
	for i := 0; i < len(data); i++ {
		val := data[i] ^ key[i%len(key)]
		shift := uint(24 - (i%4)*8)
		sign ^= uint32(val) << shift
	}
	return sign
}
