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

	if escapesWorkspace(root, resolved) {
		return "", fmt.Errorf("路径超出工作区")
	}
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("解析符号链接失败: %w", err)
		}
		rootReal = root
	}
	followed, err := followExisting(resolved)
	if err != nil {
		return "", fmt.Errorf("解析符号链接失败: %w", err)
	}
	if escapesWorkspace(rootReal, followed) {
		return "", fmt.Errorf("路径超出工作区")
	}
	return resolved, nil
}

func escapesWorkspace(root, candidate string) bool {
	rel, err := filepath.Rel(root, candidate)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func followExisting(path string) (string, error) {
	realPath, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Clean(realPath), nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}

	current := filepath.Dir(path)
	suffix := filepath.Base(path)
	for {
		realParent, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(filepath.Clean(realParent), suffix), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		if current == filepath.Dir(current) {
			return "", err
		}
		suffix = filepath.Join(filepath.Base(current), suffix)
		current = filepath.Dir(current)
	}
}
