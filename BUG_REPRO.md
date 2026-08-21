# Bug 复现说明

## Bug 是什么
检查结果仓储把首个请求的 context 存进结构体字段复用（BindCtx），后续所有结果查询都沿用第一个请求的 ctx；handler 还建了带超时的 ctx 却向下传 `context.Background()`，取消/超时完全不向下游传播。

## 如何触发
1. 用 ctx1 调一次 `POST /api/v1/exam-results/:id/enter`（成功）。
2. 取消 ctx1 后，再用新上下文调 `GET /api/v1/exam-results?registration_id=...` 或再次录入，会直接失败。
3. 用已取消的请求 ctx 调录入接口，结果仍被写入（请求取消不生效）。

## 真实错误信息
- `context canceled`（复用已取消的 ctx 导致查询失败）。
- 业务现象：某个慢请求之后，后面所有结果录入/查询秒失败；请求被取消时数据仍写库。
