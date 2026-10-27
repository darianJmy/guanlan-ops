package database

import (
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"guanlan/internal/models"
	"guanlan/pkg/logger"
)

// seedInitialData is called only inside the first-time database initialization transaction.
func seedInitialData(db *gorm.DB) error {
	logger.Info("initializing database seed data")
	password, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user := models.User{Username: "admin", Password: string(password), Nickname: "管理员", Email: "admin@aiops.local", Status: 1}
	if err := db.Create(&user).Error; err != nil {
		return err
	}
	permissions := []models.Permission{
		{ID: 67, Name: "SQL 查询", Code: "query:sql", Status: 1},
		{ID: 1, Name: "用户列表", Code: "user:list", Status: 1},
		{ID: 2, Name: "创建用户", Code: "user:create", Status: 1},
		{ID: 3, Name: "编辑用户", Code: "user:edit", Status: 1},
		{ID: 4, Name: "删除用户", Code: "user:delete", Status: 1},
		{ID: 5, Name: "角色列表", Code: "role:list", Status: 1},
		{ID: 6, Name: "创建角色", Code: "role:create", Status: 1},
		{ID: 7, Name: "编辑角色", Code: "role:edit", Status: 1},
		{ID: 8, Name: "删除角色", Code: "role:delete", Status: 1},
		{ID: 9, Name: "权限列表", Code: "perm:list", Status: 1},
		{ID: 10, Name: "菜单列表", Code: "menu:list", Status: 1},
		{ID: 11, Name: "分配角色", Code: "user:assign:role", Status: 1},
		{ID: 12, Name: "分配权限", Code: "role:assign:perm", Status: 1},
		{ID: 13, Name: "分配菜单", Code: "role:assign:menu", Status: 1},
		{ID: 14, Name: "接入列表", Code: "datasource:list", Status: 1},
		{ID: 15, Name: "创建接入", Code: "datasource:create", Status: 1},
		{ID: 16, Name: "编辑接入", Code: "datasource:edit", Status: 1},
		{ID: 17, Name: "删除接入", Code: "datasource:delete", Status: 1},
		{ID: 18, Name: "仪表盘列表", Code: "dashboard:list", Status: 1},
		{ID: 19, Name: "创建仪表盘", Code: "dashboard:create", Status: 1},
		{ID: 20, Name: "编辑仪表盘", Code: "dashboard:edit", Status: 1},
		{ID: 21, Name: "删除仪表盘", Code: "dashboard:delete", Status: 1},
		{ID: 22, Name: "拨测任务列表", Code: "inspection:list", Status: 1},
		{ID: 23, Name: "创建拨测任务", Code: "inspection:create", Status: 1},
		{ID: 24, Name: "编辑拨测任务", Code: "inspection:edit", Status: 1},
		{ID: 25, Name: "删除拨测任务", Code: "inspection:delete", Status: 1},
		{ID: 26, Name: "执行拨测", Code: "inspection:run", Status: 1},
		{ID: 27, Name: "脚本列表", Code: "script:list", Status: 1},
		{ID: 28, Name: "创建脚本", Code: "script:create", Status: 1},
		{ID: 29, Name: "编辑脚本", Code: "script:edit", Status: 1},
		{ID: 30, Name: "删除脚本", Code: "script:delete", Status: 1},
		{ID: 31, Name: "工作流列表", Code: "workflow:list", Status: 1},
		{ID: 32, Name: "创建工作流", Code: "workflow:create", Status: 1},
		{ID: 33, Name: "编辑工作流", Code: "workflow:edit", Status: 1},
		{ID: 34, Name: "删除工作流", Code: "workflow:delete", Status: 1},
		{ID: 35, Name: "执行工作流", Code: "workflow:run", Status: 1},
		{ID: 36, Name: "自动化任务列表", Code: "automation-task:list", Status: 1},
		{ID: 37, Name: "创建自动化任务", Code: "automation-task:create", Status: 1},
		{ID: 38, Name: "编辑自动化任务", Code: "automation-task:edit", Status: 1},
		{ID: 39, Name: "删除自动化任务", Code: "automation-task:delete", Status: 1},
		{ID: 40, Name: "执行自动化任务", Code: "automation-task:run", Status: 1},
		{ID: 41, Name: "Git 认证列表", Code: "git-credential:list", Status: 1},
		{ID: 42, Name: "维护 Git 认证", Code: "git-credential:edit", Status: 1},
		{ID: 43, Name: "API Token 列表", Code: "api-token:list", Status: 1},
		{ID: 44, Name: "维护 API Token", Code: "api-token:edit", Status: 1},
		{ID: 45, Name: "MCP 服务", Code: "mcp-service:list", Status: 1},
		{ID: 46, Name: "维护 MCP 服务", Code: "mcp-service:edit", Status: 1},
		{ID: 47, Name: "系统设置", Code: "system-setting:list", Status: 1},
		{ID: 48, Name: "维护系统设置", Code: "system-setting:edit", Status: 1},
		{ID: 49, Name: "节点列表", Code: "certificate:executor:list", Status: 1},
		{ID: 50, Name: "维护节点", Code: "certificate:executor:edit", Status: 1},
		{ID: 53, Name: "日志查询", Code: "observability:logs", Status: 1},
		{ID: 54, Name: "链路追踪", Code: "observability:traces", Status: 1},
		{ID: 55, Name: "查看故障告警", Code: "alert:event:list", Status: 1},
		{ID: 56, Name: "告警规则", Code: "alert:rule:list", Status: 1},
		{ID: 57, Name: "创建告警规则", Code: "alert:rule:create", Status: 1},
		{ID: 58, Name: "编辑告警规则", Code: "alert:rule:edit", Status: 1},
		{ID: 59, Name: "删除告警规则", Code: "alert:rule:delete", Status: 1},
		{ID: 60, Name: "执行告警规则", Code: "alert:rule:run", Status: 1},
		{ID: 61, Name: "通知对象", Code: "notification:channel:list", Status: 1},
		{ID: 62, Name: "维护通知对象", Code: "notification:channel:edit", Status: 1},
		{ID: 63, Name: "通知模板", Code: "notification:template:list", Status: 1},
		{ID: 64, Name: "维护通知模板", Code: "notification:template:edit", Status: 1},
		{ID: 65, Name: "通知记录", Code: "notification:record:list", Status: 1},
		{ID: 66, Name: "操作审计", Code: "security:audit:list", Status: 1},
	}
	if err := db.Create(&permissions).Error; err != nil {
		return err
	}
	menus := []models.Menu{
		{ID: 1, ParentID: 0, Name: "用户管理", Path: "/system", Component: "", Icon: "User", Sort: 98, Type: 1, Permission: "", Status: 1},
		{ID: 2, ParentID: 1, Name: "用户列表", Path: "/system/users", Component: "system/UserManage", Icon: "User", Sort: 1, Type: 2, Permission: "user:list", Status: 1},
		{ID: 3, ParentID: 1, Name: "角色列表", Path: "/system/roles", Component: "system/RoleManage", Icon: "Avatar", Sort: 2, Type: 2, Permission: "role:list", Status: 1},
		{ID: 4, ParentID: 1, Name: "权限列表", Path: "/system/permissions", Component: "system/PermissionManage", Icon: "Lock", Sort: 3, Type: 2, Permission: "perm:list", Status: 1},
		{ID: 5, ParentID: 1, Name: "菜单列表", Path: "/system/menus", Component: "system/MenuManage", Icon: "Menu", Sort: 4, Type: 2, Permission: "menu:list", Status: 1},
		{ID: 7, ParentID: 0, Name: "接入中心", Path: "/integrations/sources", Component: "monitor/IntegrationManage", Icon: "Connection", Sort: 10, Type: 2, Permission: "datasource:list", Status: 1},
		{ID: 8, ParentID: 0, Name: "仪表盘", Path: "/dashboards", Component: "monitor/DashboardHub", Icon: "DataAnalysis", Sort: 1, Type: 2, Permission: "dashboard:list", Status: 1},
		{ID: 11, ParentID: 38, Name: "拨测任务", Path: "/inspection/items", Component: "inspection/InspectionRuleGroupList", Icon: "Checked", Sort: 3, Type: 2, Permission: "inspection:list", Status: 1},
		{ID: 12, ParentID: 15, Name: "脚本库", Path: "/automation/scripts", Component: "automation/ScriptLibrary", Icon: "Document", Sort: 2, Type: 2, Permission: "", Status: 1},
		{ID: 13, ParentID: 15, Name: "自动化任务", Path: "/automation/tasks", Component: "automation/AutomationTaskManage", Icon: "SetUp", Sort: 3, Type: 2, Permission: "automation-task:list", Status: 1},
		{ID: 14, ParentID: 15, Name: "执行记录", Path: "/automation/records", Component: "inspection/ExecutionRecords", Icon: "Tickets", Sort: 5, Type: 2, Permission: "", Status: 1},
		{ID: 15, ParentID: 0, Name: "自动化中心", Path: "/automation", Component: "", Icon: "Operation", Sort: 7, Type: 1, Permission: "", Status: 1},
		{ID: 19, ParentID: 0, Name: "即时查询", Path: "/monitor/query", Component: "monitor/Explore", Icon: "Search", Sort: 2, Type: 2, Permission: "", Status: 1},
		{ID: 24, ParentID: 87, Name: "节点管理", Path: "/nodes", Component: "certificate/ExecutorManage", Icon: "Monitor", Sort: 5, Type: 2, Permission: "certificate:executor:list", Status: 1},
		{ID: 38, ParentID: 0, Name: "告警中心", Path: "/alert-center", Component: "", Icon: "Bell", Sort: 3, Type: 1, Permission: "", Status: 1},
		{ID: 39, ParentID: 38, Name: "告警管理", Path: "/alert-rules", Component: "alert/RuleGroupList", Icon: "Histogram", Sort: 1, Type: 2, Permission: "alert:rule:list", Status: 1},
		{ID: 40, ParentID: 0, Name: "故障中心", Path: "/alerts", Component: "alert/AlertCenter", Icon: "OfficeBuilding", Sort: 4, Type: 2, Permission: "alert:event:list", Status: 1},
		{ID: 70, ParentID: 0, Name: "通知管理", Path: "/notifications", Component: "", Icon: "Message", Sort: 11, Type: 1, Permission: "", Status: 1},
		{ID: 71, ParentID: 70, Name: "通知对象", Path: "/notifications/channels", Component: "notification/ChannelManage", Icon: "Promotion", Sort: 1, Type: 2, Permission: "notification:channel:list", Status: 1},
		{ID: 72, ParentID: 70, Name: "通知模板", Path: "/notifications/templates", Component: "notification/TemplateManage", Icon: "Tickets", Sort: 2, Type: 2, Permission: "notification:template:list", Status: 1},
		{ID: 73, ParentID: 70, Name: "通知记录", Path: "/notifications/records", Component: "notification/RecordManage", Icon: "DocumentChecked", Sort: 3, Type: 2, Permission: "notification:record:list", Status: 1},
		{ID: 81, ParentID: 0, Name: "操作审计", Path: "/security/audit", Component: "security/AuditLog", Icon: "DocumentChecked", Sort: 90, Type: 2, Permission: "security:audit:list", Status: 1},
		{ID: 86, ParentID: 87, Name: "Git 认证", Path: "/settings/git-credentials", Component: "system/GitCredentialManage", Icon: "Key", Sort: 3, Type: 2, Permission: "git-credential:list", Status: 1},
		{ID: 87, ParentID: 0, Name: "系统设置", Path: "/settings", Component: "", Icon: "Tools", Sort: 99, Type: 1, Permission: "", Status: 1},
		{ID: 89, ParentID: 87, Name: "登录认证", Path: "/settings/authentication", Component: "system/AuthSetting", Icon: "Lock", Sort: 2, Type: 2, Permission: "system-setting:list", Status: 1},
		{ID: 90, ParentID: 87, Name: "访问令牌", Path: "/settings/api-tokens", Component: "system/APITokenManage", Icon: "Key", Sort: 4, Type: 2, Permission: "api-token:list", Status: 1},
		{ID: 93, ParentID: 87, Name: "模型", Path: "/settings/ai-models", Component: "system/AIModelManage", Icon: "Cpu", Sort: 6, Type: 2, Permission: "system-setting:list", Status: 1},
	}
	if err := db.Create(&menus).Error; err != nil {
		return err
	}
	role := models.Role{Name: "管理员", Code: "admin", Description: "系统管理员, 拥有所有权限", Status: 1, Permissions: permissions, Menus: menus}
	if err := db.Create(&role).Error; err != nil {
		return err
	}
	if err := db.Model(&user).Association("Roles").Append(&role); err != nil {
		return err
	}
	return db.Create(&models.FaultCenter{Name: "默认故障中心", Status: 1, IsDefault: true}).Error
}
