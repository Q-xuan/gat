---
name: gat
description: 让 Agent 用项目自己的 profile 和调用说明接入 gat，组合 http 或 ws、json 或 pb，完成检查、组帧、拆帧、一次收发和报告脱敏。真实协议、密钥和业务场景留在使用方仓库。
---

# gat

gat 是 game、agent、test 的简写。用它检查一款游戏自己的二进制帧，并按 `http` 或 `ws`、`json` 或 `pb` 做一次收发。

## 接入

1. 只从使用方仓库读取帧布局。对照该项目已有的组包、解包代码，写一份私有 `gat.profile.json`。
2. 复制 `examples/profile.example.json` 的字段角色，再替换成该项目的端序、长度口径、字段和分类规则。示例里的名字和常量没有业务含义。
3. 运行 `gat check --profile gat.profile.json`。向用户只复述摘要行。不要复述常量、密钥或完整 profile。
4. 用一帧无业务内容的最小消息做 `gat encode`，再把输出交给 `gat decode`。两边的 opcode、seq 和 payload 一致才算接入完成。
5. 若 profile 含 `sign`，密钥只放进 `sign.key_env` 命名的环境变量。缺少变量时停止并说明变量名。
6. 按该项目报告里会出现的键，写 `gat.redact.json`。业务字段名放在这份文件里。输出报告前用 `github.com/Q-xuan/gat/redact` 处理。
7. 写一份私有 `gat.call.json`。`transport` 取 `http` 或 `ws`，`payload` 取 `json` 或 `pb`。地址和请求头只放在这份文件里。
8. `payload` 为 `pb` 时，向使用方要 FileDescriptorSet 和消息全名，写入 `pb.descriptor`、`pb.request`。不要把生成好的 `.pb.go` 或 descriptor 复制进 gat 仓库。
9. 线上如果先有一层二进制帧，再加 `frame.profile`，指向第 2 步的私有 profile，并填写该项目的 `opcode` 和 `seq`。
10. 运行 `gat call --spec gat.call.json --redact gat.redact.json`。只复述 `ok`、`status` 和 `frame.opcode`。不要复述 URL、请求头、descriptor 或未脱敏正文。
11. 只有 HTTP 且正文是 JSON 时，可以继续用 `gat http --spec`。缺省即 `http` + `json`。

私有 profile、脱敏策略、调用说明、路由、账号和场景放在使用方仓库，不写回 gat 仓库。

## 命令

```text
gat check --profile <file>
gat encode --profile <file> --opcode <n> [--seq n] [--ret n] [--payload hex] [--identity name=n] [--raw name=n]
gat decode --profile <file> --hex <frame> [--verify]
gat call --spec <file> [--redact <file>]
gat http --spec <file> [--redact <file>]
```

`encode` 的十六进制含有 profile 常量。不要把它贴进公开渠道或聊天记录。

## 还没有的能力

gat 不会登录，也不会把多次收发排成自动旅程。WebSocket 只读回一条，不会等待推送。需要整段旅程时，继续用该项目已有的测试流程，单次组帧、拆帧和收发交给 gat。
