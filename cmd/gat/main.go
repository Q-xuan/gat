package main

import (
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Q-xuan/gat/codec"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(stdout, usage())
		return nil
	}
	switch args[0] {
	case "check":
		return check(args[1:], stdout)
	case "encode":
		return encode(args[1:], stdout)
	case "decode":
		return decode(args[1:], stdout)
	default:
		return fmt.Errorf("未知命令 %q\n%s", args[0], usage())
	}
}

func usage() string {
	return strings.TrimSpace(`
gat check --profile <file>
gat encode --profile <file> --opcode <n> [--seq n] [--ret n] [--payload hex] [--identity name=n] [--raw name=n]
gat decode --profile <file> --hex <frame> [--verify]
`)
}

func check(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	path := fs.String("profile", "", "profile json path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	profile, err := loadProfile(*path)
	if err != nil {
		return err
	}
	size, err := profile.HeaderBytes()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "header_bytes=%d fields=%d length_basis=%s endian=%s sign=%t\n",
		size, len(profile.Fields), profile.LengthBasis, profile.Endian, profile.HasSign())
	return nil
}

func encode(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("encode", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	path := fs.String("profile", "", "profile json path")
	opcode := fs.Uint("opcode", 0, "opcode")
	seq := fs.Uint("seq", 0, "seq")
	ret := fs.Uint("ret", 0, "ret code")
	payload := fs.String("payload", "", "payload hex")
	var identity, raw assignMap
	fs.Var(&identity, "identity", "name=integer")
	fs.Var(&raw, "raw", "name=integer")
	if err := fs.Parse(args); err != nil {
		return err
	}
	profile, err := loadProfile(*path)
	if err != nil {
		return err
	}
	body, err := parseHex(*payload)
	if err != nil {
		return fmt.Errorf("payload: %w", err)
	}
	key, err := signKey(profile, true)
	if err != nil {
		return err
	}
	frame, err := codec.Encode(profile, codec.Frame{
		Opcode:   uint32(*opcode),
		Seq:      uint32(*seq),
		Ret:      uint32(*ret),
		Identity: map[string]uint64(identity),
		Raw:      map[string]uint64(raw),
		Payload:  body,
	}, key)
	if err != nil {
		return err
	}
	fmt.Fprintln(stdout, hex.EncodeToString(frame))
	return nil
}

func decode(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("decode", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	path := fs.String("profile", "", "profile json path")
	rawHex := fs.String("hex", "", "frame hex")
	verify := fs.Bool("verify", false, "verify signature")
	if err := fs.Parse(args); err != nil {
		return err
	}
	profile, err := loadProfile(*path)
	if err != nil {
		return err
	}
	data, err := parseHex(*rawHex)
	if err != nil {
		return fmt.Errorf("hex: %w", err)
	}
	if len(data) == 0 {
		return fmt.Errorf("需要 --hex")
	}
	key, err := signKey(profile, *verify)
	if err != nil {
		return err
	}
	frame, err := codec.DecodeExact(profile, data, key, *verify)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "kind=%s opcode=%d seq=%d has_seq=%t ret=%d has_ret=%t payload_bytes=%d\n",
		frame.Kind, frame.Opcode, frame.Seq, frame.HasSeq, frame.Ret, frame.HasRet, len(frame.Payload))
	for name, value := range frame.Identity {
		fmt.Fprintf(stdout, "identity %s=%d\n", name, value)
	}
	for name, value := range frame.Raw {
		fmt.Fprintf(stdout, "raw %s=%d\n", name, value)
	}
	if len(frame.Payload) > 0 {
		fmt.Fprintf(stdout, "payload=%s\n", hex.EncodeToString(frame.Payload))
	}
	return nil
}

func loadProfile(path string) (codec.Profile, error) {
	if path == "" {
		return codec.Profile{}, fmt.Errorf("需要 --profile")
	}
	return codec.LoadProfile(path)
}

func signKey(profile codec.Profile, required bool) ([]byte, error) {
	if profile.Sign == nil || !required {
		return nil, nil
	}
	key := os.Getenv(profile.Sign.KeyEnv)
	if key == "" {
		return nil, fmt.Errorf("需要环境变量 %s", profile.Sign.KeyEnv)
	}
	return []byte(key), nil
}

func parseHex(text string) ([]byte, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "0x")
	text = strings.ReplaceAll(text, " ", "")
	if text == "" {
		return nil, nil
	}
	if len(text)%2 == 1 {
		return nil, fmt.Errorf("十六进制长度必须是偶数")
	}
	return hex.DecodeString(text)
}

type assignMap map[string]uint64

func (m *assignMap) String() string { return "" }

func (m *assignMap) Set(value string) error {
	name, raw, ok := strings.Cut(value, "=")
	name = strings.TrimSpace(name)
	if !ok || name == "" {
		return fmt.Errorf("需要 name=<integer>")
	}
	n, err := strconv.ParseUint(strings.TrimSpace(raw), 0, 64)
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if *m == nil {
		*m = assignMap{}
	}
	(*m)[name] = n
	return nil
}
