# Bug 复现说明

## Bug 是什么
团检订单交付的仓储方法用命名返回值 + `defer func(){ err = tx.Commit().Error }()` 无条件覆盖业务错误（吞错），批量交付在错误分支只记日志继续，非法订单（人数为 0 或已交付）也被放行并返回成功。

## 如何触发
1. 创建 `examinee_count=0` 的团检订单。
2. `POST /api/v1/enterprises/orders/:id/deliver` → 返回 200 成功，订单状态未变，无任何错误提示。
3. 对已交付订单再次调用交付 → 同样返回成功（应被拒绝）。

## 真实错误信息
- 交付接口返回 `{"code":0,"message":"ok"}`，但订单 `status` 仍为 `pending`。
- 日志只记录 `deliver_step_failed` 后继续，原始业务错误被吞。
