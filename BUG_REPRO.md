# Bug 复现说明

## Bug 是什么
套餐新增"归档(archived)"状态后，handler 仍用本地硬编码的状态白名单 `["active","inactive"]` 校验，创建/更新归档套餐被 400 拦截；状态常量与消费方校验集合不一致（契约破坏）。

## 如何触发
1. `POST /api/v1/packages`，body 里 `status:"archived"` → 400 "套餐状态（Package.status）不合法"。
2. `PUT /api/v1/packages/:id` 同样失败。
3. 列表按 `?status=archived` 过滤无法查到任何套餐。

## 真实错误信息
- `{"code":1000,"message":"套餐状态（Package.status）不合法"}`。
- 状态常量已定义 `PackageArchived`，但 handler 校验列表未同步。
