package handlers

import (
	"context"
	"fmt"
	aicore "guanlan/internal/ai"
	"guanlan/internal/database"
	"sync"
)

// Process-local limits match the single-binary deployment. They do not enqueue work.
var chatExecutions = struct {
	sync.Mutex
	active map[[2]uint]bool
}{active: make(map[[2]uint]bool)}

func acquireChatExecution(userID, conversationID uint) (func(), bool) {
	key := [2]uint{userID, conversationID}
	chatExecutions.Lock()
	defer chatExecutions.Unlock()
	if len(chatExecutions.active) >= 4 || chatExecutions.active[key] {
		return nil, false
	}
	chatExecutions.active[key] = true
	return func() { chatExecutions.Lock(); delete(chatExecutions.active, key); chatExecutions.Unlock() }, true
}

func authorizeAITool(ctx context.Context, userID uint) error {
	return authorizeAIPermission(ctx, userID, "datasource:list")
}

func authorizeAIPermission(ctx context.Context, userID uint, permission string) error {
	var count int64
	err := database.DB.WithContext(ctx).Table("permissions AS p").
		Joins("JOIN role_permissions AS rp ON rp.permission_id = p.id").
		Joins("JOIN roles AS r ON r.id = rp.role_id AND r.status = 1").
		Joins("JOIN user_roles AS ur ON ur.role_id = r.id").
		Joins("JOIN users AS u ON u.id = ur.user_id AND u.status = 1").
		Where("u.id = ? AND p.status = 1 AND p.code = ?", userID, permission).Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("没有调用该工具的权限")
	}
	return nil
}

func aiToolPolicy(userID uint) func(context.Context, aicore.ToolSpec, aicore.ToolCall) error {
	return func(ctx context.Context, spec aicore.ToolSpec, _ aicore.ToolCall) error {
		if tool, ok := findMCPTool(spec.Name); ok {
			if tool.Annotations["readOnlyHint"] != true && !tool.AssistantWritable {
				return fmt.Errorf("助手不支持写操作")
			}
			return authorizeAssistantMCPTool(ctx, userID, tool)
		}
		return authorizeAITool(ctx, userID)
	}
}
