package codec

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"
)

func TestPayloadLengthRoundTrip(t *testing.T) {
	profile := exampleProfile(t)
	frame := Frame{Opcode: 7, Seq: 3, Payload: []byte{1, 2, 3}}
	data, err := Encode(profile, frame, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeExact(profile, data, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Opcode != 7 || got.Seq != 3 || got.Kind != KindRequest || !bytes.Equal(got.Payload, frame.Payload) {
		t.Fatalf("got %+v", got)
	}
}

func TestPushAndResponseClass(t *testing.T) {
	profile := exampleProfile(t)
	push, err := Encode(profile, Frame{Opcode: 7, Seq: 0}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeExact(profile, push, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindPush {
		t.Fatalf("kind %s", got.Kind)
	}
	resp, err := Encode(profile, Frame{Opcode: 150, Seq: 4}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err = DecodeExact(profile, resp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindResponse {
		t.Fatalf("kind %s", got.Kind)
	}
}

func TestFrameLengthAndIdentity(t *testing.T) {
	profile := Profile{
		Endian:      "big",
		LengthBasis: "frame",
		Fields: []Field{
			{Name: "size", Type: "uint32", Role: "length"},
			{Name: "mark", Type: "uint16", Role: "const", Const: u(0x1234)},
			{Name: "op", Type: "uint16", Role: "opcode"},
			{Name: "who", Type: "uint64", Role: "identity"},
		},
	}
	data, err := Encode(profile, Frame{
		Opcode:   9,
		Identity: map[string]uint64{"who": 99},
		Payload:  []byte("abc"),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 19 {
		t.Fatalf("len %d", len(data))
	}
	got, n, err := Decode(profile, append(data, 0xFF), nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(data) || got.Kind != KindMessage || got.Identity["who"] != 99 || string(got.Payload) != "abc" {
		t.Fatalf("n=%d frame=%+v", n, got)
	}
}

func TestLittleEndianAndStream(t *testing.T) {
	profile := Profile{
		Endian:      "little",
		LengthBasis: "payload",
		Fields: []Field{
			{Name: "op", Type: "uint16", Role: "opcode"},
			{Name: "size", Type: "uint16", Role: "length"},
		},
	}
	first, err := Encode(profile, Frame{Opcode: 0x0201, Payload: []byte{9}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first[0] != 0x01 || first[1] != 0x02 {
		t.Fatalf("endian bytes %x", first[:2])
	}
	second, err := Encode(profile, Frame{Opcode: 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	buf := append(append([]byte{}, first...), second...)
	got, n, err := Decode(profile, buf, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Opcode != 0x0201 || n != len(first) {
		t.Fatalf("first %+v n=%d", got, n)
	}
	got, err = DecodeExact(profile, buf[n:], nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if got.Opcode != 3 {
		t.Fatalf("second %+v", got)
	}
}

func TestSignRoundTrip(t *testing.T) {
	profile := Profile{
		Endian:      "big",
		LengthBasis: "payload",
		Fields: []Field{
			{Name: "mark", Type: "uint16", Role: "const", Const: u(1)},
			{Name: "sig", Type: "uint32", Role: "sign"},
			{Name: "n", Type: "uint32", Role: "seq"},
			{Name: "size", Type: "uint16", Role: "length"},
			{Name: "op", Type: "uint16", Role: "opcode"},
		},
		Sign: &Sign{Algo: "xor32", From: "n", KeyEnv: "SIGN_KEY"},
	}
	key := []byte("k")
	data, err := Encode(profile, Frame{Opcode: 4, Seq: 8, Payload: []byte{1, 2}}, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeExact(profile, data, []byte("other"), true); !errors.Is(err, ErrSign) {
		t.Fatalf("err %v", err)
	}
	got, err := DecodeExact(profile, data, key, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.Opcode != 4 || got.Seq != 8 || !bytes.Equal(got.Payload, []byte{1, 2}) {
		t.Fatalf("%+v", got)
	}
	if _, err := Encode(profile, Frame{Opcode: 4}, nil); !errors.Is(err, ErrKey) {
		t.Fatalf("err %v", err)
	}
}

func TestConstMismatchAndShort(t *testing.T) {
	profile := exampleProfile(t)
	data, err := Encode(profile, Frame{Opcode: 1, Seq: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	data[0], data[1] = 0, 0
	if _, err := DecodeExact(profile, data, nil, false); !errors.Is(err, ErrConst) {
		t.Fatalf("err %v", err)
	}
	if _, _, err := Decode(profile, data[:3], nil, false); !errors.Is(err, ErrShort) {
		t.Fatalf("err %v", err)
	}
}

func TestRejectsEmptySignKeyName(t *testing.T) {
	profile := Profile{
		Endian:      "big",
		LengthBasis: "payload",
		Fields: []Field{
			{Name: "op", Type: "uint16", Role: "opcode"},
			{Name: "sig", Type: "uint32", Role: "sign"},
			{Name: "size", Type: "uint16", Role: "length"},
		},
		Sign: &Sign{Algo: "xor32", From: "op"},
	}
	if err := profile.Validate(); err == nil {
		t.Fatal("expected missing key_env")
	}
}

func exampleProfile(t *testing.T) Profile {
	t.Helper()
	var profile Profile
	if err := json.Unmarshal([]byte(`{
	  "endian": "big",
	  "length_basis": "payload",
	  "fields": [
	    {"name": "mark", "type": "uint16", "role": "const", "const": 4660},
	    {"name": "op", "type": "uint16", "role": "opcode"},
	    {"name": "n", "type": "uint32", "role": "seq"},
	    {"name": "size", "type": "uint32", "role": "length"}
	  ],
	  "classify": {
	    "push": {"seq_eq": 0},
	    "response": {"seq_gt": 0, "opcode_ge": 100, "opcode_lt": 200}
	  }
	}`), &profile); err != nil {
		t.Fatal(err)
	}
	return profile
}

func u(v uint64) *uint64 { return &v }
