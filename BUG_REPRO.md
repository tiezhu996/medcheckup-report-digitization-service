# Bug 复现说明

## Bug 是什么
请求 ID 中间件用包级变量复用上一次生成的 ID，导致所有请求的 `X-Request-ID` 相同；请求日志在 `c.Next()` 之后读请求头拿不到当前 ID；panic 恢复日志也不带 request id，无法按请求排查。

## 如何触发
1. 连续发起多个请求，观察响应头 `X-Request-ID` → 全部相同。
2. 查看日志 → request_id 为空或全部相同。
3. 触发一次 panic → 恢复日志里没有 request id。

## 真实错误信息
- 多个请求响应头：`X-Request-ID: 7af9403d9c58754c15466499acfdecf6`（完全相同）。
- 日志：`msg="Request finish" request_id="" ...`。
