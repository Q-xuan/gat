package codec

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const (
	defaultMaxPayload = 1 << 20
	maxHeaderBytes    = 256
	maxFields         = 32
)

// Profile 用字段角色描述一帧二进制头，不内置任何项目的魔数或字段布局。
type Profile struct {
	Endian      string    `json:"endian"`
	LengthBasis string    `json:"length_basis"`
	MaxPayload  int       `json:"max_payload,omitempty"`
	Fields      []Field   `json:"fields"`
	Sign        *Sign     `json:"sign,omitempty"`
	Classify    *Classify `json:"classify,omitempty"`
}

type Field struct {
	Name  string  `json:"name"`
	Type  string  `json:"type"`
	Role  string  `json:"role"`
	Const *uint64 `json:"const,omitempty"`
}

type Sign struct {
	Algo   string `json:"algo"`
	From   string `json:"from"`
	KeyEnv string `json:"key_env,omitempty"`
}

type Classify struct {
	Response *Match `json:"response,omitempty"`
	Push     *Match `json:"push,omitempty"`
}

// Match 里已填写的条件同时成立才命中。
type Match struct {
	SeqEQ    *uint32 `json:"seq_eq,omitempty"`
	SeqGT    *uint32 `json:"seq_gt,omitempty"`
	OpcodeGE *uint32 `json:"opcode_ge,omitempty"`
	OpcodeLT *uint32 `json:"opcode_lt,omitempty"`
}

func LoadProfile(path string) (Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("读取 profile: %w", err)
	}
	var profile Profile
	if err := json.Unmarshal(data, &profile); err != nil {
		return Profile{}, fmt.Errorf("解析 profile: %w", err)
	}
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (p Profile) Validate() error {
	switch p.Endian {
	case "big", "little":
	default:
		return fmt.Errorf("endian 只能是 big 或 little")
	}
	switch p.LengthBasis {
	case "payload", "frame":
	default:
		return fmt.Errorf("length_basis 只能是 payload 或 frame")
	}
	if p.MaxPayload < 0 {
		return fmt.Errorf("max_payload 不能为负")
	}
	if len(p.Fields) == 0 || len(p.Fields) > maxFields {
		return fmt.Errorf("fields 数量必须在 1 到 %d", maxFields)
	}
	seen := map[string]bool{}
	roles := map[string]int{}
	width := 0
	for i, field := range p.Fields {
		name := strings.TrimSpace(field.Name)
		if name == "" {
			return fmt.Errorf("fields[%d] 缺少 name", i)
		}
		if seen[name] {
			return fmt.Errorf("字段名 %s 重复", name)
		}
		seen[name] = true
		n, err := widthOf(field.Type)
		if err != nil {
			return fmt.Errorf("fields[%d]: %w", i, err)
		}
		width += n
		switch field.Role {
		case "const":
			if field.Const == nil {
				return fmt.Errorf("字段 %s 需要 const", name)
			}
			if err := fits(*field.Const, field.Type); err != nil {
				return fmt.Errorf("字段 %s: %w", name, err)
			}
		case "opcode", "seq", "ret", "sign", "length":
			if field.Const != nil {
				return fmt.Errorf("字段 %s 不能同时写 const", name)
			}
			roles[field.Role]++
		case "identity", "raw":
			if field.Const != nil {
				return fmt.Errorf("字段 %s 不能同时写 const", name)
			}
		default:
			return fmt.Errorf("字段 %s 的 role %q 无法识别", name, field.Role)
		}
	}
	if width > maxHeaderBytes {
		return fmt.Errorf("头部超过 %d 字节", maxHeaderBytes)
	}
	for _, role := range []string{"opcode", "seq", "ret", "sign", "length"} {
		if roles[role] > 1 {
			return fmt.Errorf("role %s 只能出现一次", role)
		}
	}
	if roles["opcode"] != 1 {
		return fmt.Errorf("需要一个 opcode 字段")
	}
	if roles["length"] != 1 {
		return fmt.Errorf("需要一个 length 字段")
	}
	if p.Sign != nil {
		if p.Sign.Algo != "xor32" {
			return fmt.Errorf("sign.algo 目前只支持 xor32")
		}
		if roles["sign"] != 1 {
			return fmt.Errorf("配置了 sign 时需要一个 sign 字段")
		}
		signWidth, _ := widthOf(fieldByRole(p.Fields, "sign").Type)
		if signWidth != 4 {
			return fmt.Errorf("xor32 的 sign 字段必须是 uint32")
		}
		if !seen[strings.TrimSpace(p.Sign.From)] {
			return fmt.Errorf("sign.from 字段不存在")
		}
		if strings.TrimSpace(p.Sign.KeyEnv) == "" {
			return fmt.Errorf("sign.key_env 必须填写环境变量名")
		}
	}
	if p.Classify != nil {
		if err := p.Classify.validate(roles["seq"] == 1); err != nil {
			return err
		}
	}
	return nil
}

func (c *Classify) validate(hasSeq bool) error {
	if c.Response == nil && c.Push == nil {
		return fmt.Errorf("classify 至少要有 response 或 push")
	}
	if err := c.Response.validate("response", hasSeq); err != nil {
		return err
	}
	return c.Push.validate("push", hasSeq)
}

func (m *Match) validate(name string, hasSeq bool) error {
	if m == nil {
		return nil
	}
	if m.SeqEQ == nil && m.SeqGT == nil && m.OpcodeGE == nil && m.OpcodeLT == nil {
		return fmt.Errorf("classify.%s 没有条件", name)
	}
	if (m.SeqEQ != nil || m.SeqGT != nil) && !hasSeq {
		return fmt.Errorf("classify.%s 使用了 seq，但头部没有 seq 字段", name)
	}
	if m.OpcodeGE != nil && m.OpcodeLT != nil && *m.OpcodeGE >= *m.OpcodeLT {
		return fmt.Errorf("classify.%s 的 opcode 区间为空", name)
	}
	return nil
}

func (m *Match) hit(hasSeq bool, seq, opcode uint32) bool {
	if m == nil {
		return false
	}
	if m.SeqEQ != nil && (!hasSeq || seq != *m.SeqEQ) {
		return false
	}
	if m.SeqGT != nil && (!hasSeq || seq <= *m.SeqGT) {
		return false
	}
	if m.OpcodeGE != nil && opcode < *m.OpcodeGE {
		return false
	}
	if m.OpcodeLT != nil && opcode >= *m.OpcodeLT {
		return false
	}
	return true
}

func widthOf(kind string) (int, error) {
	switch kind {
	case "uint8":
		return 1, nil
	case "uint16":
		return 2, nil
	case "uint32":
		return 4, nil
	case "uint64":
		return 8, nil
	default:
		return 0, fmt.Errorf("type %q 无法识别", kind)
	}
}

func fits(value uint64, kind string) error {
	n, err := widthOf(kind)
	if err != nil {
		return err
	}
	if n == 8 {
		return nil
	}
	if value >= 1<<(uint(n)*8) {
		return fmt.Errorf("值 %d 超出 %s", value, kind)
	}
	return nil
}

func fieldByRole(fields []Field, role string) Field {
	for _, field := range fields {
		if field.Role == role {
			return field
		}
	}
	return Field{}
}

func (p Profile) maxPayload() int {
	if p.MaxPayload == 0 {
		return defaultMaxPayload
	}
	return p.MaxPayload
}

func (p Profile) HeaderBytes() (int, error) {
	if err := p.Validate(); err != nil {
		return 0, err
	}
	size := 0
	for _, field := range p.Fields {
		n, _ := widthOf(field.Type)
		size += n
	}
	return size, nil
}

func (p Profile) HasSign() bool {
	return p.Sign != nil
}
