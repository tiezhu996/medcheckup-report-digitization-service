# Bug 复现说明

## Bug 是什么
异常指标仓储用 `listBuffer[:0]` 复用底层数组，服务又把 `lastPage` 直接指向该切片；下一页查询会覆盖共享底层数组，导致上一页数据被下一页内容串场改写。

## 如何触发
1. 造 25 条异常指标。
2. 连续请求 `GET /api/v1/abnormal-metrics?page=1&page_size=10` 和 `?page=2&page_size=10`。
3. 第一次返回的列表快照被第二次查询改写（内容变成第二页）；接口返回的 last_page 与当前页相同。

## 真实错误信息
- 页面快照断言失败：`page1 snapshot corrupted: item[0] id=15 want 25`。
- 接口 last_page 与 list 的 id 相同（数据串场）。
