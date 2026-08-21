# Bug 复现说明

## Bug 是什么
套餐仓库进程内缓存 map 无锁读写并直接返回内部指针：并发改套餐价格与读取套餐详情时发生 data race（`concurrent map read and map write`），调用方持有的套餐引用会被后续改价原地改写，改价接口返回的可能是旧快照。

## 如何触发
1. 启动服务，先 GET `/api/v1/packages/:id` 预热缓存。
2. 并发执行：PUT `/api/v1/packages/:id`（改价）与 GET `/api/v1/packages/:id`（读详情）多次。

## 真实错误信息
- `-race` 运行：`WARNING: DATA RACE`（package_repository.go 缓存 map 的并发读写）。
- 无 `-race` 偶发：`fatal error: concurrent map read and map write`。
- 业务现象：改价后详情页仍显示旧价格；更新接口返回旧快照（price 未变）。
