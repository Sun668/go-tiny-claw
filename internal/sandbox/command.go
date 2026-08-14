package sandbox

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
)

var errCommandEscapesWorkspace = errors.New("命令试图访问工作区外的路径")

// deniedCommandSubstrings 是启发式拒绝，不是完整 shell 解析。
// 只挡住常见破坏性命令；变量拼接、base64 等绕过下一步再收。
var deniedCommandSubstrings = []string{
	"rm -rf",
	"rm -fr",
	"rm -r -f",
	"rm -f -r",
	"mkfs",
	"dd of=/dev/",
	"shutdown",
	"reboot",
	":(){ :|:& };:",
}

func IsDestructiveCommand(command string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(command), " "))
	for _, pattern := range deniedCommandSubstrings {
		if strings.Contains(normalized, pattern) {
			return true
		}
	}
	return false
}

func CommandEscapesWorkspace(command string) error {
	tokens := strings.Fields(command)
	for i, token := range tokens {
		if filepath.IsAbs(token) {
			return errCommandEscapesWorkspace
		}
		tokenSplit := strings.Split(token, string(os.PathSeparator))
		for _, tokenPart := range tokenSplit {
			if tokenPart == ".." {
				return errCommandEscapesWorkspace
			}
		}
		if token == "cd" {
			if i+1 >= len(tokens) {
				return errCommandEscapesWorkspace
			}
			nextToken := tokens[i+1]
			if filepath.IsAbs(nextToken) {
				return errCommandEscapesWorkspace
			}
			if nextToken == ".." {
				return errCommandEscapesWorkspace
			}
		}
	}
	return nil
}
