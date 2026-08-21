# Bug 复现说明

## Bug 是什么
登记状态机转换表漏掉 `in_progress → completed` 边且允许 `completed` 回退到 `registered`；进行中列表按单一状态过滤漏掉已登记人群；状态更新接口返回更新前的旧状态。

## 如何触发
1. 把登记置为 `in_progress` 后调 `PUT /api/v1/registrations/:id/status` 置 `completed` → 400 被拒。
2. 把 `completed` 登记改回 `registered` → 成功（应被拒绝）。
3. `GET /api/v1/registrations?status=in_progress` → 只返回 in_progress，不含 registered。

## 真实错误信息
- `登记状态（Registration.status）流转不合法: illegal transition`（进行中→完成被拒）。
- 更新接口返回旧状态 `registered` 而非 `in_progress`。
