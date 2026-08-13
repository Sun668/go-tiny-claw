package sandbox

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ResolveWithinWorkspace(workDir, userPath string) (string, error) {
	if strings.TrimSpace(userPath) == "" {
		return "", fmt.Errorf("路径不能为空")
	}

	if strings.TrimSpace(workDir) == "" {
		return "", fmt.Errorf("工作区路径不能为空")
	}

	if filepath.IsAbs(userPath) {
		return "", fmt.Errorf("不允许使用绝对路径")
	}

	root, err := filepath.Abs(workDir)

	if err != nil {
		return "", fmt.Errorf("解析工作区路径失败: %w", err)
	}

	root = filepath.Clean(root)

	resolved, err := filepath.Abs(filepath.Join(root, userPath))

	if err != nil {
		return "", fmt.Errorf("解析用户路径失败: %w", err)
	}

	resolved = filepath.Clean(resolved)

	rel, err := filepath.Rel(root, resolved)

	if err != nil {
		return "", fmt.Errorf("路径超出工作区")
	}

	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("路径超出工作区")
	}

	return resolved, nil
}
