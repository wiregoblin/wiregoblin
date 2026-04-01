---
name: write-config
description: Write or edit a WireGoblin project YAML config file. Use when the user asks to create a workflow, add a block, configure constants/secrets/variables, or generate a project.yaml for WireGoblin.
argument-hint: [description of what to build]
allowed-tools: Read, Glob, Write, Edit
alwaysApply: false
---

You are writing a WireGoblin project configuration file in YAML format.

## Project-level structure

```yaml
id: "unique-project-id"
name: "Human Readable Name"
version: 1

constants:          # string values, referenced as @name; support ${VAR} and ${VAR:=default}
  key: "value"
  api_host: "${API_HOST:=https://api.example.com}"

secrets:            # env-var backed, referenced as @name (same as constants)
  token: "${ENV_VAR_NAME}"
  optional_key: "${OPTIONAL_KEY:=fallback-value}"

variables:          # mutable runtime state, referenced as $name; support ${VAR} and ${VAR:=default}
  my_var: "${MY_VAR:=initial}"

secret_variables:   # mutable runtime secrets, referenced as $name; support ${VAR} and ${VAR:=default}
  session_token: ""

workflows:
  workflow_id:
    name: "Workflow Name"
    timeout_seconds: 30         # optional
    constants: {}               # workflow-scoped constants (override project-level)
    secrets: {}                 # workflow-scoped secrets
    variables: {}               # workflow-scoped variables
    secret_variables:           # runtime-secret variables (masked in logs/UI)
      session_token: ""
    outputs:                    # values exported when used as a child workflow
      key: "$variable"
    catch_error_blocks: []      # blocks that run on workflow failure
    blocks: []
```

## Block types reference

### http
```yaml
- id: "step_id"
  type: "http"
  method: "GET"          # GET POST PUT PATCH DELETE
  url: "@base_url"
  headers:
    X-Header: "value"
  body: '{"key":"value"}' # for POST/PUT/PATCH
  condition:             # optional — skip block unless true
    variable: "$var"
    operator: "="
    expected: "value"
  continue_on_error: false
  assign:
    $status: "outputs.statusCode"
    $body_field: "body.fieldName"
    $response_time: "outputs.responseTimeMs"
```

### grpc
```yaml
- id: "step_id"
  type: "grpc"
  address: "@grpc_host"
  tls: true
  method: "/package.Service/Method"
  request: '{"field":"value"}'
  metadata:
    x-header: "value"
  assign:
    $reply: "body.fieldName"
    $response_time: "outputs.responseTimeMs"
```

### assert
```yaml
- id: "step_id"
  type: "assert"
  variable: "$var"
  operator: "="           # = != > >= < <= contains
  expected: "value"
  error_message: "Descriptive failure message"
  continue_on_error: false
```

### goto
```yaml
- id: "step_id"
  type: "goto"
  variable: "$var"
  operator: "="
  expected: "retry"
  target_step_id: "some_earlier_step"
  wait_seconds: 1         # optional delay before jump
```

### postgres
```yaml
- id: "step_id"
  type: "postgres"
  dsn: "@postgres_dsn"
  query: "SELECT $1 AS result"
  params:
    - "@constant_or_$variable"
  assign:
    $row_count: "outputs.rowCount"
    $field: "body.rows.0.column_name"

# transaction variant
- id: "step_id"
  type: "postgres"
  dsn: "@postgres_dsn"
  transaction:
    - query: "INSERT INTO ..."
      params: ["$val"]
    - query: "SELECT ..."
      assign:
        $result: "body.rows.0.col"
```

### redis
```yaml
- id: "step_id"
  type: "redis"
  address: "@redis_address"
  command: "SET"          # any Redis command
  args:
    - "key:name"
    - "$value"
  assign:
    $result: "outputs.result"
    $cached: "body.result"
```

### setvars
```yaml
- id: "step_id"
  type: "setvars"
  set:
    $var_name: "literal or $other_var"
```

### log
```yaml
- id: "step_id"
  type: "log"
  level: "info"           # info warn error debug
  message: "Text with $variables and @constants"
```

### delay
```yaml
- id: "step_id"
  type: "delay"
  milliseconds: 500
```

### retry
```yaml
- id: "step_id"
  type: "retry"
  max_attempts: 3
  delay_ms: 200
  retry_on:
    match: "any"
    rules:
      - type: "transport_error"
      - type: "status_code"
        in: [429, 500, 502, 503]
      - type: "path"
        path: "body.data"
        operator: "empty"
  block:
    type: "http"           # inner block config (no id needed)
    method: "GET"
    url: "@url"
    assign:
      $status: "outputs.statusCode"
```

### foreach
```yaml
# array variant
- id: "step_id"
  type: "foreach"
  items: "$json_array_variable"
  concurrency: 1
  block:
    type: "log"
    level: "info"
    message: "Item !Each.Index/!Each.Count -> !Each.Item.field"
  collect:
    $ids: "item.id"
    $names: "item.name"

# range variant
- id: "step_id"
  type: "foreach"
  items:
    from: 1
    to: 10
    step: 1
  block:
    type: "log"
    level: "info"
    message: "Value: !Each.Item"
  collect:
    $values: "item"
```

### parallel
```yaml
- id: "step_id"
  type: "parallel"
  blocks:
    - id: "branch_a"
      type: "http"
      ...
    - id: "branch_b"
      type: "setvars"
      ...
  collect:
    $status_code: "branch_a.exports.statusCode"
    $session_token: "branch_b.output.set.$session_token"
```

### transform
```yaml
- id: "step_id"
  type: "transform"
  value:
    key: "$var"
    nested:
      field: "@constant"
  casts:
    key: "int"            # int float bool string
  regex:
    extracted_field:
      from: "value.text_field"
      pattern: "\\b(\\d+)\\b"
      group: 1
  assign:
    $json: "json"
    $field: "value.key"
    $extracted: "extracted.extracted_field"
```

### workflow (call another workflow)
```yaml
- id: "step_id"
  type: "workflow"
  target_workflow_id: "other_workflow_id"
  inputs:
    param_name: "$variable"
  assign:
    $output: "outputs.exported_key"
```

### smtp
```yaml
- id: "step_id"
  type: "smtp"
  host: "@smtp_host"
  port: "@smtp_port"
  from: "@smtp_from"
  to:
    - "@recipient"
  subject: "Subject line"
  text: "Plain text body"
  html: "<p>HTML body</p>"
  assign:
    $message_id: "outputs.message_id"
```

### imap
```yaml
- id: "step_id"
  type: "imap"
  host: "@imap_host"
  port: "@imap_port"
  username: "@imap_user"
  password: "@imap_pass"
  tls: false
  mailbox: "INBOX"
  criteria:
    message_id: "$sent_id"
    subject_contains: "text"
    body_contains: "text"
    unseen_only: true
  wait:
    timeout_ms: 10000
    poll_interval_ms: 500
  mark_as_seen: true
  assign:
    $subject: "body.message.subject"
    $text: "body.message.text"
```

### openai
```yaml
- id: "step_id"
  type: "openai"
  base_url: "@openai_base_url"   # optional, for proxies/mocks
  token: "@openai_token"
  model: "gpt-4.1-mini"
  system_prompt: "You are a helpful assistant."
  user_prompt: "$user_input"
  assign:
    $status: "outputs.statusCode"
    $reply: "body.choices.0.message.content"
```

### telegram
```yaml
- id: "step_id"
  type: "telegram"
  base_url: "@telegram_base_url"  # optional, for mocks
  token: "@telegram_token"
  chat_id: "@telegram_chat_id"
  message: "Text with $variables"
  assign:
    $status: "outputs.statusCode"
```

### container
```yaml
- id: "step_id"
  type: "container"
  image: "@container_image"
  command: printf '%s' "$MESSAGE"
  env:
    MESSAGE: "$variable"
  assign:
    $stdout: "body.stdout"
    $exit_code: "outputs.exitCode"
```

## Reference rules

- `@name` — reference a constant or secret (string)
- `$name` — reference a variable (mutable, string or JSON)
- `${ENV_VAR}` — inject OS environment variable into a constant/secret/variable value at load time
- `${ENV_VAR:=default}` — same, but use `default` if the variable is unset or empty

### Global built-ins (`!name`) — available in every step

| Built-in | Example value | Description |
|----------|---------------|-------------|
| `!RunID` | `"f47ac10b-..."` | Unique UUID for this run — use as correlation ID |
| `!StartTime` | `"2026-04-01T12:00:00Z"` | Workflow start time (RFC 3339, UTC) |
| `!StartUnix` | `"1743508800"` | Workflow start time as Unix epoch seconds |
| `!StartDate` | `"2026-04-01"` | Workflow start date (`YYYY-MM-DD`, UTC) |
| `!ProjectID` | `"my-project"` | Project ID |
| `!WorkflowID` | `"create_user"` | Current workflow ID |
| `!WorkflowName` | `"Create User"` | Current workflow display name |
| `!BlockStartTime` | `"2026-04-01T12:00:05Z"` | Current step start time (RFC 3339, UTC) |
| `!BlockStartUnix` | `"1743508805"` | Current step start time as Unix epoch seconds |

### Error built-ins — populated in `catch_error_blocks`

| Built-in | Description |
|----------|-------------|
| `!ErrorMessage` | Error message from the failed step |
| `!ErrorBlockID` | `id` of the failed step |
| `!ErrorBlockName` | `name` of the failed step |
| `!ErrorBlockType` | Block type of the failed step |
| `!ErrorBlockIndex` | 1-based index of the failed step |

### Parent built-ins — populated inside a child workflow invoked via `workflow` block

| Built-in | Description |
|----------|-------------|
| `!Parent.WorkflowID` | Parent workflow ID |
| `!Parent.WorkflowName` | Parent workflow name |
| `!Parent.RunID` | Parent run ID |
| `!Parent.StartTime` | Parent start time |

### foreach built-ins — available inside `foreach` block

| Built-in | Description |
|----------|-------------|
| `!Each.Index` | 0-based iteration index |
| `!Each.Count` | Total number of items |
| `!Each.First` | `"true"` on first iteration |
| `!Each.Last` | `"true"` on last iteration |
| `!Each.Item` | Current item value |
| `!Each.ItemJSON` | Current item serialized as JSON string |
| `!Each.Item.<field>` | Field of the current item (for objects) |

### retry built-ins — available inside `retry` block

| Built-in | Description |
|----------|-------------|
| `!Retry.Attempt` | Current attempt number (1-based) |
| `!Retry.MaxAttempts` | Configured max attempts |

## Rules to follow

1. Every block must have a unique `id` within the workflow (snake_case).
2. Assert blocks after every network call to validate the result.
3. Use `@constants` for URLs, DSNs, addresses. Never hardcode them in blocks.
4. Use `secrets:` for tokens and passwords; reference them with `@name` same as constants.
5. Variable names must start with `$`.
6. Constants are string-only; numeric port fields will be parsed by the block at decode time.
7. When generating a full project, always include at least one assert per workflow.
8. Output the final YAML as a code block.
9. Use `${VAR:=default}` in `constants`, `secrets`, `variables`, and `secret_variables` whenever a sensible default exists so the config works out-of-the-box without every env var being set.

## Task

$ARGUMENTS
