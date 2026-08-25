// Package model 定义火山岩薄片矿物边界复核台的领域实体、状态机与几何判定算法。
package model

import "errors"

// 领域错误：httpapi 层据此映射 HTTP 状态码，service 层据此做边界校验。
var (
	// ErrNotFound 表示资源不存在。
	ErrNotFound = errors.New("not found")
	// ErrConflict 表示资源冲突（重复哈希、重复裁决、并发版本冲突）。
	ErrConflict = errors.New("conflict")
	// ErrBadState 表示非法的状态机流转。
	ErrBadState = errors.New("invalid state transition")
	// ErrInvalidGeometry 表示多边形几何非法（未闭合/顶点过少）。
	ErrInvalidGeometry = errors.New("invalid geometry")
	// ErrRegionOutOfBounds 表示区域顶点超出图像边界。
	ErrRegionOutOfBounds = errors.New("region out of image bounds")
	// ErrSelfIntersecting 表示多边形闭合环自交。
	ErrSelfIntersecting = errors.New("polygon self-intersects")
	// ErrNotClosed 表示多边形首尾未闭合。
	ErrNotClosed = errors.New("polygon not closed")
	// ErrUnknownMineral 表示矿物编码不在矿物库中。
	ErrUnknownMineral = errors.New("unknown mineral")
	// ErrFrozenVersion 表示解释版本已冻结，拒绝任何修改。
	ErrFrozenVersion = errors.New("interpretation version is frozen")
	// ErrClosedLoopRequired 表示操作要求薄片批次先进入待复核状态。
	ErrClosedLoopRequired = errors.New("batch not in review state")
	// ErrImageModeMismatch 表示消光特征计算需要成对的 PPL/XPL 图像。
	ErrImageModeMismatch = errors.New("require paired PPL and XPL images")
	// ErrValidation 表示普通参数校验失败。
	ErrValidation = errors.New("validation failed")
)
