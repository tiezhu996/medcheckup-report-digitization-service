# Bug 复现说明

## Bug 是什么
运营日报导出 worker 把 `wg.Add(1)` 写在 goroutine 内部且错误分支向无缓冲 channel 发送（无人读取永久阻塞）；close 后仍有 worker 发送触发 `send on closed channel` panic，导出接口偶发崩溃或卡住。

## 如何触发
1. 数据中含 `department=""` 的检查项目时调 `GET /api/v1/stats/export` → 服务卡住或返回 panic。
2. 正常数据并发多次导出 → 偶发 `panic: send on closed channel`。

## 真实错误信息
```
panic: send on closed channel
goroutine 11 [running]:
github.com/blueship581/gbcheckup/internal/service.(*StatsService).ExportDailyReport.func1(...)
	.../internal/service/stats_service.go:73 +0xe8
```
