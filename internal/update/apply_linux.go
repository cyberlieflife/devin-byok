//go:build linux

package update

import (
	"fmt"
)

// Linux 上无官方 GUI 发布物，在线更新脚本不适用；
// 返回明确错误让调用方（GUI 更新按钮）提示用户手动升级。

func scheduleApply(extractDir, installDir, guiName, tmp string) (string, error) {
	return "", fmt.Errorf("在线更新暂不支持 Linux，请手动从源码构建升级")
}

func scheduleApplyArtifact(artifactPath, installDir, guiName, tmp string) (string, error) {
	return "", fmt.Errorf("在线更新暂不支持 Linux，请手动从源码构建升级")
}