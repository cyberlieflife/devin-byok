//go:build linux

package payload

// LSWrapper 在 Linux 上未内置（官方发布物仅 Windows/macOS）。
// 保持字段存在以保证 internal/lsinstall 等调用方可编译；
// 运行时 MaterializeWrapper 会因空内容返回明确错误。
var LSWrapper []byte
