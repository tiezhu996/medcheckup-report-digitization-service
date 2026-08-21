# Bug 复现说明

## Bug 是什么
报告仓储用 `%v` 丢失 `util.ErrNotFound` 哨兵错误，`errors.Is` 失效：首次生成草稿时把"不存在草稿"误判为系统错误返回 500；查不存在的报告也返回 500 而非 404。

## 如何触发
1. `POST /api/v1/reports/draft?registration_id=1`（该登记还没有草稿）→ 返回 500，草稿未创建。
2. `GET /api/v1/reports/999999` → 返回 500 而非 404。

## 真实错误信息
- `find report by registration 1: not found`（errors.Is 无法识别 sentinel）。
- 响应体：`{"code":1007,"message":"服务内部错误"}`。
