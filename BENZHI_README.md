# BENZHI 评测说明

基于 Go 实现的火山岩薄片矿物边界复核后端服务，一款后端服务，完成偏光图像摘要导入、矿物区域边界几何校验与特征消光判定、邻接/交生关系检测，并发布不可变薄片解释版本。

## 启动

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go run ./cmd/thinsect --addr :8080 --db thinsect.db
```

## 自检（不启动长驻服务）

```bash
go run ./cmd/thinsect --smoke-test
```

`--smoke-test` 会真实创建批次、导入图像与区域、推进复核、检测关系、冻结解释版本，关闭并重新打开数据库验证持久化与重启恢复，最后以 0 退出码结束。

## 构建门禁

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...
go run ./cmd/thinsect --smoke-test
```

## HTTP API（前缀 /api）

批次：`POST/GET /api/batches`、`GET /api/batches/{id}`、`POST /api/batches/{id}/advance`
图像：`POST/GET /api/batches/{id}/images`、`GET /api/images/{id}`
区域：`POST/GET /api/images/{id}/regions`、`GET /api/regions/{id}`
关系：`POST /api/batches/{id}/relations/detect`、`GET /api/batches/{id}/relations`
版本：`POST/GET /api/batches/{id}/versions`、`POST /api/versions/{id}/share|freeze|supersede`
基础：`GET /api/minerals`、`GET /api/health`

## 持久化

SQLite（modernc.org/sqlite，CGO 无关）。建表：batches、images、regions、relationships、opinions、versions、minerals。同一图像哈希幂等；冻结版本为不可变快照。
