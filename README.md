# gat

给 Agent 用的游戏协议测试命令。名字是 game、agent、test 的简写。

一款游戏的帧格式和脱敏规则写在该项目自己的 JSON 里。gat 仓库不保存任何一款游戏的魔数、字段布局、密钥或业务场景。

## 接入

```text
go install github.com/Q-xuan/gat/cmd/gat@latest
```

在自己的仓库放不提交到公开位置的文件：

- `gat.profile.json`：二进制头，只有帧协议才需要
- `gat.redact.json`：报告里要删掉或遮住的键
- `gat.call.json`：一次收发，写明 `transport` 和 `payload`

然后：

```text
gat check --profile gat.profile.json
gat encode --profile gat.profile.json --opcode 1 --seq 1 --payload 0102
gat decode --profile gat.profile.json --hex <encode 的输出>
gat call --spec gat.call.json --redact gat.redact.json
```

`check` 只打印头部字节数、字段数、端序、长度口径和是否启用签名，不打印常量。`encode` 的十六进制里含有 profile 常量，不要贴到公开渠道。

启用签名时，密钥放在 profile 的 `sign.key_env` 指定的环境变量里。gat 没有默认密钥。`decode` 加 `--verify` 才会校验签名。

Go 项目也可以直接引用：

```text
go get github.com/Q-xuan/gat
```

编解码用 `github.com/Q-xuan/gat/codec`，报告脱敏用 `github.com/Q-xuan/gat/redact`。一次收发用 `github.com/Q-xuan/gat/call`，传输在 `transport`，正文编码在 `payload`。

把自己的 AI 指到本仓库根目录的 `SKILL.md`。它会按项目现有协议写私有 profile，再跑 `check` 和一次往返。要改 gat 本身时读 `AGENTS.md`。

## Profile

`endian` 取 `big` 或 `little`。`length_basis` 取 `payload`（长度只含消息体）或 `frame`（长度含头部）。

| role | 含义 |
| --- | --- |
| `const` | 固定值，解码时必须一致 |
| `opcode` | 协议号 |
| `seq` | 序号，可选 |
| `ret` | 返回码，可选 |
| `sign` | 签名槽，配合 `sign.algo=xor32` |
| `length` | 长度 |
| `identity` | 项目自己的身份整数，名称自定 |
| `raw` | 原样读写、不参与分类的整数 |

`classify.push` 和 `classify.response` 可以用 `seq_eq`、`seq_gt`、`opcode_ge`、`opcode_lt`。未命中且有 `seq` 时视为请求，否则视为普通消息。

`examples/profile.example.json` 是临时示例，字段名和常量没有业务含义。接入时复制出来再改。

## 脱敏

`redact` 包按项目自己的策略处理报告：

- `drop_key_substrings`：删掉匹配的键和值
- `redact_key_substrings` / `redact_key_exact`：保留键，值换成替换文案
- `omit_paths`：按点路径删除
- `fingerprint` 为 `sha256-8` 时，把运行中出现的秘密字符串换成短指纹
- `presets` 可包含 `credentials`，只覆盖通用的 token、password、secret

业务字段名写在项目自己的策略里。临时示例是 `examples/redact.example.json`。

## 一次收发

`transport` 和 `payload` 分开选：

| transport | payload | 结果 |
| --- | --- | --- |
| `http` | `json` | HTTP，正文是 JSON |
| `http` | `pb` | HTTP，正文是 protobuf |
| `ws` | `json` | WebSocket 文本消息，正文是 JSON |
| `ws` | `pb` | WebSocket 二进制消息，正文是 protobuf |

`gat call --spec` 要求这两项都写明。`gat http --spec` 仍可用：缺省就是 `http` + `json`，旧的 HTTP 说明不用改。`gat http` 遇到 `transport: ws` 会失败。

HTTP 只接受 `http` 和 `https`。WebSocket 只接受 `ws` 和 `wss`，发一条并读回一条。响应正文按 `--redact` 脱敏后再打印，请求头不会打印。`${变量名}` 从环境变量替换，替换出来的值也会从正文里抹掉。

`expect_status` 只用于 HTTP。与实际状态码不一致时，命令以失败退出，标准输出仍是脱敏后的结果。

`payload` 为 `pb` 时，`pb.descriptor` 是使用方用 `protoc --descriptor_set_out` 生成的 FileDescriptorSet，`pb.request` 是请求消息全名，`pb.response` 省略时与请求相同。路径相对 spec 文件所在目录。调用说明里的 `body` 仍写 JSON，发出去是 protobuf，响应再解回 JSON 后脱敏。

需要在正文外包一层二进制帧时，加上 `frame`。`frame.profile` 指向项目自己的 profile，`opcode` 和 `seq` 由使用方填写。帧格式仍由 profile 决定，不内置在命令里。

```json
{
  "transport": "ws",
  "payload": "pb",
  "url": "ws://127.0.0.1:9/ws",
  "body": {},
  "pb": {"descriptor": "messages.pb", "request": "sample.Ping"},
  "frame": {"profile": "gat.profile.json", "opcode": 1, "seq": 1}
}
```

`binary` 只影响 WebSocket 的消息类型。省略时，`pb` 和带 `frame` 的请求走二进制，其余走文本。带 `frame` 时不能改成文本。

临时示例是 `examples/http.example.json` 和 `examples/ws-pb.example.json`。地址、消息名和 descriptor 文件名都没有业务含义。

## 当前范围

现在可以接入的是：校验 profile、组一帧、拆一帧、按上面四种组合发一次请求，以及在 Go 里做报告脱敏。二进制帧可以包在任意一种正文外面。

按场景自动登录、连续多步、等待推送和断言还不在这条命令里。WebSocket 不会挂着等下一条推送。各项目仍用自己的测试流程调用 `encode`、`decode` 和 `call`。
