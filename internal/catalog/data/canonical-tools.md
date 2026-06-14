# Canonical Tool Taxonomy

Canonical names are `lowercase_snake_case`. Every provider's native name maps
to exactly one canonical name. Provider-unique tools keep their own name as the
canonical. Lookup is case-insensitive on the native side.

## Core File Operations

| Canonical       | Providers → native names                                                                                                              |
|-----------------|---------------------------------------------------------------------------------------------------------------------------------------|
| `read_file`     | claude-code→`Read`, codex→_(none)_, cursor→`read_file`, gemini-cli→`read_file`, github-copilot→`read`+`view`, goose→_(text_editor)_, opencode→`read`, roo-code→`read` |
| `file_edit`     | claude-code→`Edit`, cursor→`edit_file`, gemini-cli→`edit`+`replace`, github-copilot→_(none mapped)_, goose→`text_editor`, opencode→`edit`, roo-code→`edit` |
| `file_write`    | claude-code→`Write`, gemini-cli→`write_file`                                                                                         |
| `apply_patch`   | codex→`apply_patch`, github-copilot→`apply_patch`, opencode→`apply_patch`                                                           |
| `delete_file`   | cursor→`delete_file`                                                                                                                  |
| `read_many_files` | gemini-cli→`read_many_files`                                                                                                        |

## Shell / Execution

| Canonical | Providers → native names                                                                                                                                   |
|-----------|------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `bash`    | claude-code→`Bash`, codex→`shell`, cursor→`run_terminal_command`, gemini-cli→`run_shell_command`, github-copilot→`bash`, goose→`shell`, grok-cli→_(none)_, opencode→`bash`, roo-code→`command`, antigravity→_(none)_ |
| `powershell` | claude-code→`PowerShell`                                                                                                                                |

## Search & Discovery

| Canonical          | Providers → native names                                                                                                          |
|--------------------|-----------------------------------------------------------------------------------------------------------------------------------|
| `glob`             | claude-code→`Glob`, gemini-cli→`glob`, opencode→`glob`                                                                           |
| `grep`             | claude-code→`Grep`, cursor→`grep_search`, gemini-cli→`search_file_content`, github-copilot→`grep`+`rg`, opencode→`grep`          |
| `list_directory`   | cursor→`list_dir`, gemini-cli→`list_directory`, opencode→`list`                                                                   |
| `file_search`      | cursor→`file_search`                                                                                                              |
| `semantic_search`  | cursor→`codebase_search`                                                                                                          |

## Web

| Canonical     | Providers → native names                                                                                                                                        |
|---------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `web_search`  | claude-code→`WebSearch`, codex→`web_search`, cursor→`web_search`, gemini-cli→`google_web_search`, github-copilot→_(none)_, grok-cli→`search_web`, opencode→`websearch` |
| `web_fetch`   | claude-code→`WebFetch`, gemini-cli→`web_fetch`, github-copilot→`web_fetch`, opencode→`webfetch`                                                                |

## Media / Generation

| Canonical          | Providers → native names                               |
|--------------------|--------------------------------------------------------|
| `image_generation` | codex→`image_generation`, cursor→`image_generation`, grok-cli→`generate_image` |
| `generate_video`   | grok-cli→`generate_video`                             |

## Agent / Orchestration

| Canonical          | Providers → native names                                                              |
|--------------------|---------------------------------------------------------------------------------------|
| `agent`            | claude-code→`Agent`                                                                   |
| `task`             | github-copilot→`task`, grok-cli→`task`, opencode→`task`                             |
| `delegate`         | grok-cli→`delegate`                                                                   |
| `spawn_agent`      | codex→`spawn_agent`                                                                   |
| `send_message`     | claude-code→`SendMessage`                                                             |
| `skill`            | claude-code→`Skill`, opencode→`skill`                                                 |
| `workflow`         | claude-code→`Workflow`                                                                |

## Desktop / Browser Automation

| Canonical       | Providers → native names                              |
|-----------------|-------------------------------------------------------|
| `computer_use`  | codex→`computer_use`, grok-cli→`computer`            |
| `browser`       | codex→`browser`, cursor→`browser`, roo-code→`browser`|

## Code Intelligence

| Canonical     | Providers → native names                  |
|---------------|-------------------------------------------|
| `lsp`         | claude-code→`LSP`, opencode→`lsp`        |
| `code_review` | codex→`code_review`                       |

## Notebook

| Canonical       | Providers → native names       |
|-----------------|--------------------------------|
| `notebook_edit` | claude-code→`NotebookEdit`     |

## Persistence / Memory

| Canonical     | Providers → native names            |
|---------------|-------------------------------------|
| `save_memory` | gemini-cli→`save_memory`            |
| `todo_write`  | claude-code→`TodoWrite`, opencode→`todowrite` |
| `todo_read`   | opencode→`todoread`                 |

## MCP Integration

| Canonical             | Providers → native names                  |
|-----------------------|-------------------------------------------|
| `list_mcp_resources`  | claude-code→`ListMcpResourcesTool`        |
| `read_mcp_resource`   | claude-code→`ReadMcpResourceTool`         |
| `wait_for_mcp_servers`| claude-code→`WaitForMcpServers`           |
| `tool_search`         | claude-code→`ToolSearch`, codex→`tool_search` |
| `mcp`                 | roo-code→`mcp`                            |

## Scheduling / Lifecycle

| Canonical         | Providers → native names                   |
|-------------------|--------------------------------------------|
| `cron_create`     | claude-code→`CronCreate`                   |
| `cron_delete`     | claude-code→`CronDelete`                   |
| `cron_list`       | claude-code→`CronList`                     |
| `schedule_wakeup` | claude-code→`ScheduleWakeup`               |
| `monitor`         | claude-code→`Monitor`                      |
| `remote_trigger`  | claude-code→`RemoteTrigger`                |

## Task Management

| Canonical      | Providers → native names       |
|----------------|--------------------------------|
| `task_create`  | claude-code→`TaskCreate`       |
| `task_get`     | claude-code→`TaskGet`          |
| `task_list`    | claude-code→`TaskList`         |
| `task_output`  | claude-code→`TaskOutput`       |
| `task_stop`    | claude-code→`TaskStop`         |
| `task_update`  | claude-code→`TaskUpdate`       |

## Team Management

| Canonical     | Providers → native names   |
|---------------|----------------------------|
| `team_create` | claude-code→`TeamCreate`   |
| `team_delete` | claude-code→`TeamDelete`   |

## Worktree / Planning (claude-code)

| Canonical         | Providers → native names          |
|-------------------|-----------------------------------|
| `enter_plan_mode` | claude-code→`EnterPlanMode`       |
| `exit_plan_mode`  | claude-code→`ExitPlanMode`        |
| `enter_worktree`  | claude-code→`EnterWorktree`       |
| `exit_worktree`   | claude-code→`ExitWorktree`        |

## User Interaction

| Canonical              | Providers → native names                               |
|------------------------|--------------------------------------------------------|
| `ask_user_question`    | claude-code→`AskUserQuestion`, cursor→`ask_questions`, opencode→`question` |
| `push_notification`    | claude-code→`PushNotification`                         |
| `share_onboarding_guide` | claude-code→`ShareOnboardingGuide`                   |

## Provider-Specific (X / Social)

| Canonical  | Providers → native names   |
|------------|----------------------------|
| `search_x` | grok-cli→`search_x`        |

## opencode-specific

| Canonical            | Providers → native names            |
|----------------------|-------------------------------------|
| `external_directory` | opencode→`external_directory`       |

## Goose-specific

| Canonical        | Providers → native names           |
|------------------|------------------------------------|
| `analyze`        | goose→`analyze`                    |
| `screen_capture` | goose→`screen_capture`             |
| `image_processor`| goose→`image_processor`            |

## Codex-specific

| Canonical    | Providers → native names   |
|--------------|----------------------------|
| `view_image` | codex→`view_image`         |

## Notes

- `apply_patch` is kept DISTINCT from `file_edit`: it is a structured diff
  application operation, not a string-replacement editor.
- grok-cli `search_web` maps to `web_search` (same logical operation as other
  providers' web search). `search_x` has no cross-provider equivalent and keeps
  its own canonical.
- gemini-cli `replace` and `edit` are both mapped to `file_edit` (both perform
  in-place file modifications).
- Cursor `run_terminal_command` → `bash` (shell execution, same semantics).
- roo-code `command` → `bash` (shell execution).
- goose `text_editor` → `file_edit` (the text_editor tool in goose handles file
  read/write/edit operations; the dominant usage is editing).
- github-copilot `view` → `read_file`; `read` → `read_file` (both are file
  readers).
- opencode `websearch` → `web_search`; `webfetch` → `web_fetch`.
- Aliases (case-insensitive): `WebSearch`==`websearch`==`web_search` all resolve
  to canonical `web_search`.
