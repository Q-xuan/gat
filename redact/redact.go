package redact

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Policy 决定报告里哪些键被抹掉、哪些值被替换。字段名列表由使用方提供。
type Policy struct {
	DropKeySubstrings   []string `json:"drop_key_substrings,omitempty"`
	RedactKeySubstrings []string `json:"redact_key_substrings,omitempty"`
	RedactKeyExact      []string `json:"redact_key_exact,omitempty"`
	OmitPaths           []string `json:"omit_paths,omitempty"`
	MinSecretLen        int      `json:"min_secret_len,omitempty"`
	Replacement         string   `json:"replacement,omitempty"`
	Fingerprint         string   `json:"fingerprint,omitempty"`
	Presets             []string `json:"presets,omitempty"`
}

func (p Policy) normalized() Policy {
	if p.MinSecretLen <= 0 {
		p.MinSecretLen = 4
	}
	if p.Replacement == "" {
		p.Replacement = "<redacted>"
	}
	for _, preset := range p.Presets {
		if preset == "credentials" {
			p.RedactKeySubstrings = append(p.RedactKeySubstrings, "token", "password", "secret")
		}
	}
	return p
}

// Load 读取使用方自己的脱敏策略。文件不存在时由调用方决定是否继续。
func Load(path string) (Policy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("读取脱敏策略: %w", err)
	}
	var policy Policy
	if err := json.Unmarshal(data, &policy); err != nil {
		return Policy{}, fmt.Errorf("解析脱敏策略: %w", err)
	}
	return policy, nil
}

// Apply 返回新的值。secrets 里足够长的字符串会从文本中替换掉。
func Apply(policy Policy, value any, secrets []string) any {
	policy = policy.normalized()
	return walk(policy, value, "", secrets)
}

func walk(policy Policy, value any, path string, secrets []string) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if omitPath(policy, childPath) || dropKey(policy, key) {
				continue
			}
			if redactKey(policy, key) {
				out[key] = policy.Replacement
				continue
			}
			out[key] = walk(policy, child, childPath, secrets)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			out[i] = walk(policy, child, path, secrets)
		}
		return out
	case string:
		return maskText(policy, typed, secrets)
	default:
		return value
	}
}

func omitPath(policy Policy, path string) bool {
	for _, item := range policy.OmitPaths {
		if item == path {
			return true
		}
	}
	return false
}

func dropKey(policy Policy, key string) bool {
	lower := strings.ToLower(key)
	for _, item := range policy.DropKeySubstrings {
		if item != "" && strings.Contains(lower, strings.ToLower(item)) {
			return true
		}
	}
	return false
}

func redactKey(policy Policy, key string) bool {
	lower := strings.ToLower(key)
	for _, item := range policy.RedactKeyExact {
		if strings.EqualFold(key, item) {
			return true
		}
	}
	for _, item := range policy.RedactKeySubstrings {
		if item != "" && strings.Contains(lower, strings.ToLower(item)) {
			return true
		}
	}
	return false
}

func maskText(policy Policy, text string, secrets []string) string {
	out := text
	for _, secret := range secrets {
		if len(secret) < policy.MinSecretLen || !strings.Contains(out, secret) {
			continue
		}
		out = strings.ReplaceAll(out, secret, fingerprint(policy, secret))
	}
	return out
}

func fingerprint(policy Policy, secret string) string {
	if policy.Fingerprint == "sha256-8" {
		sum := sha256.Sum256([]byte(secret))
		return "sha256:" + hex.EncodeToString(sum[:8])
	}
	return policy.Replacement
}
