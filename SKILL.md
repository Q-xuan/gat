---
name: gat
description: 让 Agent 用项目自己的 profile 接入 gat，完成游戏协议帧的检查、组帧、拆帧和报告脱敏。真实协议、密钥和业务场景留在使用方仓库。
---

# gat

gat 是 game、agent、test 的简写。用它检查并编解码一款游戏自己的二进制帧。

## 接入

1. 只从使用方仓库读取帧布局。对照该项目已有的组包、解包代码，写一份私有 `gat.profile.json`。
2. 复制 `examples/profile.example.json` 的字段角色，再替换成该项目的端序、长度口径、字段和分类规则。示例里的名字和常量没有业务含义。
3. 运行 `gat check --profile gat.profile.json`。向用户只复述摘要行。不要复述常量、密钥或完整 profile。
4. 用一帧无业务内容的最小消息做 `gat encode`，再把输出交给 `gat decode`。两边的 opcode、seq 和 payload 一致才算接入完成。
5. 若 profile 含 `sign`，密钥只放进 `sign.key_env` 命名的环境变量。缺少变量时停止并说明变量名。
6. 按该项目报告里会出现的键，写 `gat.redact.json`。业务字段名放在这份文件里。输出报告前用 `github.com/Q-xuan/gat/redact` 处理。
7. 该项目若有 HTTP 接口，另写 `gat.http.json`，用 `gat http --spec gat.http.json --redact gat.redact.json` 发一次请求。只复述 `ok` 和 `status`。不要复述 URL、请求头或未脱敏正文。

私有 profile、脱敏策略、路由、账号和场景放在使用方仓库，不写回 gat 仓库。

## 命令

```text
gat check --profile <file>
gat encode --profile <file> --opcode <n> [--seq n] [--ret n] [--payload hex] [--identity name=n] [--raw name=n]
gat decode --profile <file> --hex <frame> [--verify]
gat http --spec <file> [--redact <file>]
```

`encode` 的十六进制含有 profile 常量。不要把它贴进公开渠道或聊天记录。

## 还没有的能力

gat 不会登录，也不会把多次 HTTP 或二进制帧排成自动旅程。需要整段旅程时，继续用该项目已有的测试流程，单次组帧、拆帧和 HTTP 调用交给 gat。
