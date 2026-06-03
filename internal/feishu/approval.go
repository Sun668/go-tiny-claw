package feishu

import (
	"fmt"
	"log"
	"regexp"
	"sync"
)

type ApprovalResult struct {
	Allowed bool
	Reason  string
}

type ApprovalManager struct {
	mu           sync.RWMutex
	pendingTasks map[string]chan ApprovalResult
}

var GlobalApprovalMgr = &ApprovalManager{
	pendingTasks: make(map[string]chan ApprovalResult),
}

func (m *ApprovalManager) WaitForApproval(taskID string, toolName string, args string, reporter *FeishuReporter) (bool, string) {
	ch := make(chan ApprovalResult, 1)

	m.mu.Lock()
	m.pendingTasks[taskID] = ch
	m.mu.Unlock()

	noticeMsg := fmt.Sprintf(`⚠️ **高危操作审批请求**
	Agent 试图执行以下动作:
	- 工具: %s
	- 参数: %s
	
	任务 ID: **%s**
	👉 请在此消息下方回复 "approve %s" 或 "reject %s" 来决定是否放行。`, toolName, args, taskID, taskID, taskID)

	if reporter != nil {
		reporter.sendMsg(noticeMsg)
	} else {
		fmt.Printf("\n\033[31m[需要审批 TaskID: %s]\033[0m %s\n", taskID, noticeMsg)
	}

	log.Printf("[Approval] 已发送审批请求 (TaskID: %s)，协程挂起等待...\n", taskID)

	result := <-ch

	m.mu.Lock()
	delete(m.pendingTasks, taskID)
	m.mu.Unlock()

	return result.Allowed, result.Reason
}

func (m *ApprovalManager) ResolveApproval(taskID string, allowed bool, reason string) {
	m.mu.Lock()
	ch, exists := m.pendingTasks[taskID]
	m.mu.Unlock()

	if exists {
		log.Printf("[Approval] 收到来自飞书的审批结果 (TaskID: %s, Allowed: %v)\n", taskID, allowed)
		ch <- ApprovalResult{Allowed: allowed, Reason: reason}
	} else {
		log.Printf("[Approval] 找不到对应的 TaskID: %s，可能已超时或处理完毕\n", taskID)
	}
}

func IsDangerousCommand(toolName string, args string) bool {
	if toolName != "bash" && toolName != "write_file" && toolName != "edit_file" {
		return false
	}

	if toolName == "bash" {
		dangerousPatterns := []string{
			`rm\s+-r`, // 级联删除
			`sudo\s+`, // 提权
			`drop\s+`, // 数据库删除
			`>.*\.go`, // 恶意覆盖源代码
		}

		for _, p := range dangerousPatterns {
			matched, _ := regexp.MatchString(p, args)
			if matched {
				return true
			}
		}
	}
	return false
}
