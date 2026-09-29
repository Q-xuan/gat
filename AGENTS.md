# gat

这个仓库是给 agent 用来对网络服务拼一次测试的工具。

要测服务，按四步做。最小例子是 `examples/profile.example.json`。可复制的命令在 `SKILL.md` 开头。

要改这个工具，再读下面已有的约束。

# 修改 gat

这份说明给要改本仓库的 Agent。用 gat 去接一款游戏时，读 `SKILL.md`，不要改这里的代码。

## 保持通用

帧格式、签名密钥、调用地址和请求头、protobuf descriptor、消息名、脱敏字段名、登录和业务场景都属于使用方。它们只出现在使用方自己的 profile、调用说明和测试里。

本仓库的 Go 源码和 `examples/` 只保留无业务含义的临时样例。新增一种帧差异时，先加 profile 字段或 role，并补测试。不要把某一款游戏的魔数、字段顺序、默认密钥或 descriptor 写进代码。

传输和正文分开。`transport` 只负责 http 或 ws，`payload` 只负责 json 或 pb，二进制帧继续走 `codec`。新增组合时复用这三边，不要再写一套发送，也不要内置某一款游戏的消息。descriptor 由使用方用 FileDescriptorSet 提供。

密钥只从 profile 的 `sign.key_env` 读取。`check` 的输出不打印常量。

## 改完要验证

```text
go test ./...
```

改了命令行参数时，同步 `README.md`、`SKILL.md` 和 `cmd/gat` 的测试。README 里的「当前范围」要和命令实际能做的事一致。

## 提交

提交说明只写改动原因。不要把工具署名、协作者行或某一款游戏的 profile 写进提交。
