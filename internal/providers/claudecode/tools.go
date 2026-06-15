package claudecode

import (
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolmapper"
	"github.com/Shaik-Sirajuddin/graft/internal/providers/internal/toolset"
)

// knownTools is the set of native tool names this provider understands on disk.
// Implements contract.ToolSupporter. Native names are PascalCase for claude-code.
// Source: internal/catalog/data/claude-code/tools.json
var knownTools = toolset.New(
	"Read", "Write", "Edit", "Bash", "BashOutput", "Glob", "Grep", "WebFetch", "WebSearch",
	"Agent", "AskUserQuestion", "CronCreate", "CronDelete", "CronList",
	"EnterPlanMode", "EnterWorktree", "ExitPlanMode", "ExitWorktree",
	"KillShell", "ListMcpResourcesTool", "LSP", "Monitor", "NotebookEdit", "PowerShell",
	"PushNotification", "ReadMcpResourceTool", "RemoteTrigger", "ScheduleWakeup",
	"SendMessage", "ShareOnboardingGuide", "Skill", "TaskCreate", "TaskGet",
	"TaskList", "TaskOutput", "TaskStop", "TaskUpdate", "TeamCreate", "TeamDelete",
	"TodoWrite", "ToolSearch", "WaitForMcpServers", "Workflow",
)

// SupportsTool reports whether the provider understands the given native tool name.
// Implements contract.ToolSupporter.
func (Provider) SupportsTool(tool string) bool { return knownTools.Contains(tool) }

// toolMap is the bidirectional native↔canonical mapping for claude-code.
// Source: internal/catalog/data/claude-code/tools.json
var toolMap = toolmapper.New([]toolmapper.Entry{
	{Native: "Agent", Canonical: "agent"},
	{Native: "AskUserQuestion", Canonical: "ask_user_question"},
	{Native: "Bash", Canonical: "bash"},
	{Native: "BashOutput", Canonical: "bash_output"},
	{Native: "CronCreate", Canonical: "cron_create"},
	{Native: "CronDelete", Canonical: "cron_delete"},
	{Native: "CronList", Canonical: "cron_list"},
	{Native: "Edit", Canonical: "file_edit"},
	{Native: "EnterPlanMode", Canonical: "enter_plan_mode"},
	{Native: "EnterWorktree", Canonical: "enter_worktree"},
	{Native: "ExitPlanMode", Canonical: "exit_plan_mode"},
	{Native: "ExitWorktree", Canonical: "exit_worktree"},
	{Native: "Glob", Canonical: "glob"},
	{Native: "Grep", Canonical: "grep"},
	{Native: "KillShell", Canonical: "kill_shell"},
	{Native: "ListMcpResourcesTool", Canonical: "list_mcp_resources"},
	{Native: "LSP", Canonical: "lsp"},
	{Native: "Monitor", Canonical: "monitor"},
	{Native: "NotebookEdit", Canonical: "notebook_edit"},
	{Native: "PowerShell", Canonical: "powershell"},
	{Native: "PushNotification", Canonical: "push_notification"},
	{Native: "Read", Canonical: "read_file"},
	{Native: "ReadMcpResourceTool", Canonical: "read_mcp_resource"},
	{Native: "RemoteTrigger", Canonical: "remote_trigger"},
	{Native: "ScheduleWakeup", Canonical: "schedule_wakeup"},
	{Native: "SendMessage", Canonical: "send_message"},
	{Native: "ShareOnboardingGuide", Canonical: "share_onboarding_guide"},
	{Native: "Skill", Canonical: "skill"},
	{Native: "TaskCreate", Canonical: "task_create"},
	{Native: "TaskGet", Canonical: "task_get"},
	{Native: "TaskList", Canonical: "task_list"},
	{Native: "TaskOutput", Canonical: "task_output"},
	{Native: "TaskStop", Canonical: "task_stop"},
	{Native: "TaskUpdate", Canonical: "task_update"},
	{Native: "TeamCreate", Canonical: "team_create"},
	{Native: "TeamDelete", Canonical: "team_delete"},
	{Native: "TodoWrite", Canonical: "todo_write"},
	{Native: "ToolSearch", Canonical: "tool_search"},
	{Native: "WaitForMcpServers", Canonical: "wait_for_mcp_servers"},
	{Native: "WebFetch", Canonical: "web_fetch"},
	{Native: "WebSearch", Canonical: "web_search"},
	{Native: "Workflow", Canonical: "workflow"},
	{Native: "Write", Canonical: "file_write"},
})

// CanonicalTool translates a native tool name to its canonical equivalent.
// Implements contract.ToolMapper. Lookup is case-insensitive.
func (Provider) CanonicalTool(native string) (string, bool) { return toolMap.CanonicalTool(native) }

// NativeTool translates a canonical tool name to this provider's native name.
// Implements contract.ToolMapper.
func (Provider) NativeTool(canonical string) (string, bool) { return toolMap.NativeTool(canonical) }

// Tools returns the sorted canonical names of all tools this provider supports.
// Implements contract.ToolMapper.
func (Provider) Tools() []string { return toolMap.Tools() }
