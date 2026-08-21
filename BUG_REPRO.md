# Bug 复现说明

## Bug 是什么
批量导入体检人时，服务里多个内存 map（去重 seen、身份证索引 dedupCache、summaryCache）未初始化即写入；同时仓储查无此人时丢失 not-found 哨兵。首次向 nil map 写入直接 panic。

## 如何触发
1. `POST /api/v1/examinees/batch-import`，CSV 含两行以上数据 → 服务 panic。
2. `POST /api/v1/examinees` 新建体检人 → panic。
3. `GET /api/v1/examinees/:id` → panic。

## 真实错误信息
```
panic: assignment to entry in nil map [recovered, repanicked]
goroutine 10 [running]:
github.com/blueship581/gbcheckup/internal/service.(*ExamineeService).BatchImport(...)
	.../internal/service/examinee_service.go:96 +0x35c
```
