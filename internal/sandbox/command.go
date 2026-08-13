package sandbox

import "strings"

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
