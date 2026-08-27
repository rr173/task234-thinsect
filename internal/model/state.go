package model

import "fmt"

// 薄片批次状态机：importing → segmenting → review → published → archived。
const (
	BatchImporting  = "importing"  // 导入中：允许添加图像与分割区域
	BatchSegmenting = "segmenting" // 分割中：自动分割运行，不允许发布
	BatchReview     = "review"     // 待复核：研究者复核区域、关系与证据
	BatchPublished  = "published"  // 已发布：解释版本冻结
	BatchArchived   = "archived"   // 封存：只读终态
)

// 区域状态机：candidate → labeled / mismerged / open_boundary / excluded。
const (
	RegionCandidate   = "candidate"     // 候选：自动分割产出，等待复核
	RegionLabeled     = "labeled"       // 已标注：研究者确认矿物标签
	RegionMismerged   = "mismerged"     // 误合并：自动分割把两种矿物合并，待拆分
	RegionOpenBoundary = "open_boundary" // 缺边界：闭合环存在缺口，需修复
	RegionExcluded    = "excluded"      // 排除：非矿物（气泡、裂缝、玻屑），剔除
)

// 矿物关系类型与状态机：adjacent/intergrowth/conflict → confirmed。
const (
	RelationAdjacent   = "adjacent"   // 相邻：共享边界或距离低于阈值
	RelationIntergrowth = "intergrowth" // 交生：一个矿物晶体嵌生于另一矿物内部
	RelationConflict   = "conflict"   // 冲突：同一区域出现矛盾的标签证据
	RelationConfirmed  = "confirmed"  // 确认：研究者裁决通过
)

// 解释版本状态机：draft → shared → frozen → superseded。
const (
	VersionDraft     = "draft"     // 草稿：可修改
	VersionShared    = "shared"    // 共享：可被他人审阅，仍可回退
	VersionFrozen    = "frozen"    // 冻结：不可变快照，作为引用基线
	VersionSuperseded = "superseded" // 替代：被更新的冻结版本取代
)

// 图像采集模式。
const (
	ImageModePPL = "PPL" // 平面偏光（单偏光）
	ImageModeXPL = "XPL" // 正交偏光（消光观察）
)

// 批次允许的状态迁移表。
var batchTransitions = map[string][]string{
	BatchImporting:  {BatchSegmenting, BatchArchived},
	BatchSegmenting: {BatchReview, BatchArchived},
	BatchReview:     {BatchPublished, BatchArchived},
	BatchPublished:  {BatchArchived},
	BatchArchived:   {},
}

// 区域允许的状态迁移表。
var regionTransitions = map[string][]string{
	RegionCandidate:    {RegionLabeled, RegionMismerged, RegionOpenBoundary, RegionExcluded},
	RegionMismerged:    {RegionLabeled, RegionExcluded},
	RegionOpenBoundary: {RegionLabeled, RegionCandidate, RegionExcluded},
	RegionLabeled:      {RegionExcluded},
	RegionExcluded:     {},
}

// 版本允许的状态迁移表。
var versionTransitions = map[string][]string{
	VersionDraft:      {VersionShared, VersionSuperseded},
	VersionShared:     {VersionFrozen, VersionSuperseded},
	VersionFrozen:     {VersionSuperseded},
	VersionSuperseded: {},
}

// CanTransitionBatch 判断批次状态迁移是否合法。
func CanTransitionBatch(from, to string) bool { return can(batchTransitions, from, to) }

// CanTransitionRegion 判断区域状态迁移是否合法。
func CanTransitionRegion(from, to string) bool { return can(regionTransitions, from, to) }

// CanTransitionVersion 判断版本状态迁移是否合法。
func CanTransitionVersion(from, to string) bool { return can(versionTransitions, from, to) }

func can(table map[string][]string, from, to string) bool {
	for _, t := range table[from] {
		if t == to {
			return true
		}
	}
	return false
}

// ValidateBatchState 校验状态字面量合法。
func ValidateBatchState(s string) bool { return valid(batchTransitions, s) }

// ValidateRegionState 校验状态字面量合法。
func ValidateRegionState(s string) bool { return valid(regionTransitions, s) }

// ValidateVersionState 校验状态字面量合法。
func ValidateVersionState(s string) bool { return valid(versionTransitions, s) }

func valid(table map[string][]string, s string) bool {
	_, ok := table[s]
	return ok
}

// StateTransitionError 描述一次被拒绝的状态迁移。
type StateTransitionError struct {
	Entity string
	From   string
	To     string
}

func (e *StateTransitionError) Error() string {
	return fmt.Sprintf("%s cannot transition from %q to %q", e.Entity, e.From, e.To)
}
