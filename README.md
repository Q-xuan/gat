# gat

给 Agent 用的游戏协议测试命令。名字是 game、agent、test 的简写。

一款游戏的帧格式和脱敏规则写在该项目自己的 JSON 里。gat 仓库不保存任何一款游戏的魔数、字段布局、密钥或业务场景。

## 接入

```text
go install github.com/Q-xuan/gat/cmd/gat@latest
```

在自己的仓库放两份不提交到公开位置的文件：

- `gat.profile.json`：二进制头
- `gat.redact.json`：报告里要删掉或遮住的键

然后：

```text
gat check --profile gat.profile.json
gat encode --profile gat.profile.json --opcode 1 --seq 1 --payload 0102
gat decode --profile gat.profile.json --hex <encode 的输出>
```

`check` 只打印头部字节数、字段数、端序、长度口径和是否启用签名，不打印常量。`encode` 的十六进制里含有 profile 常量，不要贴到公开渠道。

启用签名时，密钥放在 profile 的 `sign.key_env` 指定的环境变量里。gat 没有默认密钥。`decode` 加 `--verify` 才会校验签名。

Go 项目也可以直接引用：

```text
go get github.com/Q-xuan/gat
```

编解码用 `github.com/Q-xuan/gat/codec`，报告脱敏用 `github.com/Q-xuan/gat/redact`。

把自己的 AI 指到本仓库根目录的 `SKILL.md`。它会按项目现有协议写私有 profile，再跑 `check` 和一次往返。

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

## 当前范围

现在可以接入的是：校验 profile、组一帧、拆一帧、在 Go 里做报告脱敏。

按场景自动登录、发送、等待推送和断言还不在这条命令里。各项目仍用自己的测试流程调用 `encode` / `decode`。
