# 火山岩薄片矿物边界复核台

基于 Go 实现的火山岩薄片（偏光显微镜薄片）矿物边界复核 Web 项目：导入偏光图像摘要与自动分割区域，校验边界几何（闭合/自交/越界），计算颜色与消光特征并生成候选矿物标签，检测相邻/交生关系，研究者可拆分误合并区域、补充显微证据、裁决关系并冻结不可变的薄片解释版本。

## 业务闭环

1. 创建薄片批次，导入成对 PPL（平面偏光）/XPL（正交偏光）图像摘要（SHA-256 幂等去重）；
2. 导入自动分割区域：多边形必须闭合、不自交、不越出图像范围；
3. 批次推进到待复核后：检测区域间相邻/交生/冲突关系，计算颜色与消光特征；
4. 人工复核：拆分误合并区域（子区域保留来源 parent）、排除非矿物、补充消光证据、裁决关系；
5. 创建解释版本 → 共享 → 冻结（不可变快照）→ 批次发布；新冻结版本自动替代旧冻结版本。

## 关键设计

- **状态机**：批次 importing→segmenting→review→published→archived；区域 candidate→labeled/mismerged/open_boundary/excluded；版本 draft→shared→frozen→superseded。
- **几何判定**：鞋带公式面积、跨立实验自交检测、射线法点在多边形内、主轴方向角。
- **特征判定**：XPL/PPL 亮度比 → 消光比；颜色统计 + 消光比匹配矿物库 → 候选标签与置信度。
- **关系判定**：顶点距离 ≤ 图像短边 2% → 相邻；小面积质心落入大区域 → 交生；同标签消光差异显著 → 冲突。
- **并发与错误边界**：不同区域可并行复核；版本冻结用 BEGIN IMMEDIATE 事务串行化同批次发布；冻结后一切区域/关系修改被拒绝（ErrFrozenVersion）。

## 标准命令

```bash
CGO_ENABLED=0 GOTOOLCHAIN=local go build ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go vet   ./...
CGO_ENABLED=0 GOTOOLCHAIN=local go test  ./...
go run ./cmd/thinsect --db /tmp/thinsect.db --smoke-test   # 端到端自检
go run ./cmd/thinsect --addr :8080 --db ./thinsect.db      # 启动服务
```

## 主要 API

| 能力 | API |
|---|---|
| 批次 | `POST /api/batches`、`GET /api/batches`、`GET /api/batches/{id}`、`POST /api/batches/{id}/advance`、`GET /api/batches/{id}/stats` |
| 图像 | `POST /api/batches/{id}/images`、`GET /api/batches/{id}/images`、`GET /api/images/{id}` |
| 区域 | `POST /api/images/{id}/regions`、`POST /api/regions/{id}/label`、`POST /api/regions/{id}/split`、`POST /api/regions/{id}/exclude` 等 |
| 特征 | `POST /api/regions/{id}/features` |
| 关系 | `POST /api/batches/{id}/relations/detect`、`POST /api/relations/{id}/adjudicate` |
| 意见 | `POST /api/regions/{id}/opinions` |
| 版本 | `POST /api/batches/{id}/versions`、`POST /api/versions/{id}/freeze` 等 |

完整清单见 `BENZHI_README.md`。

## 持久化

SQLite（modernc.org/sqlite v1.52.0，纯 Go 驱动）：batches、images、regions、relationships、opinions、versions、minerals 七张表。WAL 模式 + 5s busy timeout；同一批次同哈希图像幂等；冻结版本保留原始区域几何（不可变快照），重启后可完整恢复。
