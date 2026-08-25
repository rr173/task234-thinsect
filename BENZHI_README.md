# 基于 Go 实现的火山岩薄片矿物边界复核 Web 项目，一款后端服务，完成偏光图像摘要导入、矿物区域边界几何校验与特征消光判定、邻接/交生关系检测并发布不可变解释版本。

# BENZHI 评测说明

## 项目一句话简介

火山岩薄片矿物边界复核台：岩石学研究者导入偏光图像摘要与自动分割区域，系统校验边界闭合与自交、计算颜色/消光特征并生成候选矿物标签，研究者可拆分误合并区域、补充显微证据、裁决矿物关系，最终冻结不可变的薄片解释版本。

## 业务闭环

1. 导入薄片批次与成对 PPL/XPL 图像摘要（SHA-256 幂等）；
2. 导入分割区域（闭合环校验：未闭合/自交/越界一律拒绝）；
3. 批次进入待复核 → 检测相邻/交生/冲突关系 → 计算特征与候选标签；
4. 人工复核：拆分误合并区域（保留来源）、排除非矿物、补充消光证据、裁决关系；
5. 创建解释版本 → 共享 → 冻结（不可变快照，冻结后拒绝一切修改）→ 批次发布。

## 状态机

- 薄片批次：importing → segmenting → review → published → archived
- 区域：candidate → labeled / mismerged / open_boundary / excluded
- 矿物关系：adjacent / intergrowth / conflict → confirmed
- 解释版本：draft → shared → frozen → superseded

## 标准命令

```bash
# 构建与测试（固定环境）
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...

# 端到端自检（Docker 验收契约）
go run ./cmd/thinsect --db /tmp/thinsect-smoke.db --smoke-test

# 启动服务
go run ./cmd/thinsect --addr :8080 --db ./thinsect.db
```

## API 入口（统一 /api 前缀，共 30 个）

| 能力 | API |
|---|---|
| 批次 | POST/GET /api/batches，GET /api/batches/{id}，POST /api/batches/{id}/advance，GET /api/batches/{id}/stats |
| 图像 | POST/GET /api/batches/{id}/images，GET /api/images/{id} |
| 区域 | POST/GET /api/images/{id}/regions，GET /api/regions/{id}，POST /api/regions/{id}/label，POST /api/regions/{id}/mismerged，POST /api/regions/{id}/open-boundary，POST /api/regions/{id}/exclude，POST /api/regions/{id}/split |
| 特征 | POST/GET /api/regions/{id}/features |
| 关系 | POST /api/batches/{id}/relations/detect，GET /api/batches/{id}/relations，POST /api/relations/{id}/adjudicate |
| 意见 | POST/GET /api/regions/{id}/opinions |
| 版本 | POST/GET /api/batches/{id}/versions，POST /api/versions/{id}/share\|freeze\|supersede |
| 基础 | GET /api/minerals，GET /api/health |

## Docker 双架构

```bash
bash build_benzhi_docker.sh my-project linux/amd64
bash build_benzhi_docker.sh my-project linux/arm64
docker run --rm --platform linux/amd64 my-project:latest --smoke-test
```

两个平台构建 + `docker run --smoke-test` 均须 exit 0（一次性基线证明见 `.private/docker_baseline_validation.json`）。

## --smoke-test 契约

不启动长驻服务，而是真实执行：创建批次 → 导入 PPL/XPL 图像（重复导入验证幂等）→ 导入 3 个区域 → 推进状态机 → 关系检测（相邻+交生）→ 特征计算 → 人工标注 → 补充意见 → 版本冻结 → 验证冻结守卫 → 关闭并重开数据库验证持久化恢复，全部断言通过后以 0 退出码结束。

## 持久化与重启恢复

SQLite（modernc.org/sqlite 纯 Go 驱动，CGO_ENABLED=0 可离线构建），表：batches / images / regions / relationships / opinions / versions / minerals。同一图像哈希幂等；冻结版本为不可变快照，重启后原始区域几何完整保留。
