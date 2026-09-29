package payload

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
)

type pbCodec struct {
	request  protoreflect.MessageDescriptor
	response protoreflect.MessageDescriptor
}

// LoadPB 从使用方提供的 FileDescriptorSet 加载请求和响应消息。
// response 为空时与 request 使用同一个消息名。
func LoadPB(descriptorPath, requestName, responseName string) (Codec, error) {
	data, err := os.ReadFile(descriptorPath)
	if err != nil {
		return nil, errors.New("读取 descriptor")
	}
	set := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(data, set); err != nil {
		return nil, errors.New("解析 descriptor")
	}
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, fmt.Errorf("descriptor 无法加载: %w", err)
	}
	request, err := messageByName(files, requestName)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(responseName) == "" {
		responseName = requestName
	}
	response, err := messageByName(files, responseName)
	if err != nil {
		return nil, err
	}
	return pbCodec{request: request, response: response}, nil
}

func (p pbCodec) Encode(body []byte) ([]byte, error) {
	msg := dynamicpb.NewMessage(p.request)
	if len(bytes.TrimSpace(body)) > 0 {
		if err := (protojson.UnmarshalOptions{}).Unmarshal(body, msg); err != nil {
			return nil, errors.New("正文无法编码")
		}
	}
	out, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	if err != nil {
		return nil, errors.New("正文无法编码")
	}
	return out, nil
}

func (p pbCodec) Decode(wire []byte) (any, error) {
	msg := dynamicpb.NewMessage(p.response)
	if err := proto.Unmarshal(wire, msg); err != nil {
		return nil, errors.New("正文无法解码")
	}
	encoded, err := (protojson.MarshalOptions{}).Marshal(msg)
	if err != nil {
		return nil, errors.New("正文无法解码")
	}
	var parsed any
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		return nil, errors.New("正文无法解码")
	}
	return parsed, nil
}

func messageByName(files *protoregistry.Files, name string) (protoreflect.MessageDescriptor, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("pb 需要消息名")
	}
	desc, err := files.FindDescriptorByName(protoreflect.FullName(name))
	if err != nil {
		return nil, fmt.Errorf("找不到消息 %s", name)
	}
	md, ok := desc.(protoreflect.MessageDescriptor)
	if !ok {
		return nil, fmt.Errorf("找不到消息 %s", name)
	}
	return md, nil
}
