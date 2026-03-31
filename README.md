# WireGoblin

Declarative workflow runner for integration testing and service orchestration. Define steps in YAML — call HTTP APIs, gRPC services, databases, send emails, run containers, assert results, loop with goto.

No glue code. No one-off scripts. One workflow file, one command.

## Features

- **HTTP, gRPC, Postgres, Redis, Docker** — built-in blocks for the most common integration targets
- **Assertions and conditions** — fail fast or skip steps based on response data
- **Retry with backoff** — configurable attempts, delay doubling, and per-status-code rules
- **Loops and goto** — build dynamic flows with conditional jumps and wait intervals
- **Foreach and parallel** — iterate over lists or run blocks concurrently with collect aggregation
- **Nested workflows** — compose reusable workflows with explicit inputs and outputs
- **Email** — send via SMTP and wait for delivery via IMAP with criteria matching
- **Transform** — build structured payloads, cast types, and extract values with regex
- **Secret handling** — secrets are redacted in all logs and step output
- **Streaming output** — NDJSON event stream for CI integration and custom tooling

## Install

```bash
go install github.com/wiregoblin/wiregoblin/cmd/wiregoblin-cli@latest
```

## Run

```bash
wiregoblin-cli run <workflow_id>
wiregoblin-cli run -p config/example.project.yaml http_example
```

If `-p` is omitted, WireGoblin looks for `wiregoblin.yaml` or `wiregoblin.yml` in the current directory.

**Verbosity flags:**

| Flag | Output |
|------|--------|
| _(default)_ | Live progress and final summary |
| `-v` | Step outcome and timing |
| `-vv` | Compact response summaries |
| `-vvv` | Full request/response payloads |
| `--json` | Stream NDJSON events to stdout |

## Config

```yaml
id: "demo"
name: "Demo"

constants:
  api_host: "https://api.example.com"

secrets:
  api_token: "${API_TOKEN}"

secret_variables:
  session_token: ""

workflows:
  http_example:
    name: "HTTP Example"
    blocks:
      - id: "seed_session"
        type: "setvars"
        set:
          $session_token: "@api_token"

      - id: "get_user"
        type: "http"
        method: "GET"
        url: "@api_host/users/1"
        headers:
          Authorization: "Bearer $session_token"
        assign:
          $user_id: "body.id"

      - id: "notify"
        type: "telegram"
        token: "@api_token"
        chat_id: "123456789"
        message: "User $user_id has been created"
```

### References

| Prefix | Resolves to |
|--------|-------------|
| `$name` | Runtime variable or secret variable |
| `@name` | Project/workflow constant or secret |
| `!name` | Read-only built-in: `!ErrorMessage`, `!ErrorBlockID`, `!Parent.WorkflowID`, etc. |

- Inline interpolation is supported in string fields: `"Bearer @token"`, `"Hello, $user!"`
- Inline interpolation also works inside structured string payloads such as gRPC `request` JSON: `{"sentence":"Hello, $user from !Parent.WorkflowName"}`
- `$`, `@`, and `!` prefixes in `condition.variable`, `assert.variable`, and `goto.variable` are explicit: use `$status` to read a runtime variable, or omit `$` for a literal.
- Project and workflow `constants` are string-valued; numeric-looking values (e.g. ports) remain in the `@name` string namespace.

### Variables

- `assign` maps block data paths to runtime variables: use `body.*` for the main payload, `outputs.*` for block output values such as status codes.
- Secret variables declared in `secret_variables` are mutable but redacted in logs and step output.
- Assignment targets always use `$name`; if `name` exists in `secret_variables`, the write goes there.
- `collect` paths are treated as literal data paths and are not interpolated as `$`/`@`/`!` references.

### Step control

- Every step can define `condition:` with `variable`, `operator`, and `expected`; when the condition does not match, the step is skipped.
- Every step can define `continue_on_error: true`; failed steps then get status `ignored-error`, do not stop the workflow, and do not trigger `catch_error_blocks`.
- Every workflow can define `timeout_seconds` to cap total runtime for the entire workflow.

### Operators

`assert` and `goto` support: `=`, `!=`, `>`, `<`, `>=`, `<=`, `contains` (`like` is an alias).

`goto` also supports optional `wait_seconds` before jumping to `target_step_id`.

`assert.expected` supports the same `@name` and `$name` interpolation as other string fields.

### Block-specific notes

- `http` supports optional `tls_skip_verify: true` for self-signed or staging endpoints; exports `outputs.responseTimeMs`.
- `grpc` exports `outputs.responseTimeMs`; `bytes` fields use raw strings by default — prefix with `base64:` for binary: `{"payload":"base64:SGVsbG8="}`.
- `http`, `openai`, and `telegram` accept optional `timeout_seconds`.
- `smtp` sends one email; exposes `body.subject` and `outputs.message_id`.
- `imap` waits for and reads one email; use `body.message.text`, `body.message.html`, `body.message.subject`, `body.message.message_id`. Criteria: `message_id`, `from`, `to`, `subject_contains`, `body_contains`, `unseen_only`.
- `container` runs `sh -c` inside the configured image; treat `command` as trusted input.
- `transform` returns `value` and serialized `json`; use `casts` to convert interpolated strings into `int`, `float`, `bool`, `string`, or parsed `json`; supports `regex:` extraction via `extracted.<name>` in `assign` paths.

### Nested blocks

**`retry`**
- Exposes `!Retry.Attempt` and `!Retry.MaxAttempts` inside `block`; `delay_ms` doubles after each failed attempt.
- Optional `retry_on:` with `status_codes` and `transport_errors` limits which errors are retried.

**`foreach`**
- Exposes `!Each.Index`, `!Each.Count`, `!Each.First`, `!Each.Last`, `!Each.Item`, `!Each.ItemJSON`, and object fields like `!Each.Item.id` inside `block`.
- `items` accepts an array, a JSON array string, or a numeric range object with `from`, `to`, and optional `step`.
- Optional `concurrency`; when `concurrency > 1`, iterations run in parallel but `results` and `collect` stay in input order.
- `collect:` aggregates per-iteration values into arrays; paths use `item.*`, `output.*`, `exports.*`, `error`, and `index`.
- When `concurrency > 1`, nested blocks cannot use runtime `assign`; use `collect` instead.
- When `concurrency > 1`, the block waits for all iterations; any failure returns an aggregate error but still includes per-iteration results.

**`parallel`**
- Runs heterogeneous nested blocks concurrently; exposes branch results by branch `id`.
- `collect:` paths must start with the branch `id`, e.g. `fetch_user.output.body`.
- Nested branches cannot use runtime `assign`; use `collect` instead.
- Waits for all branches; any failure returns an aggregate error but still includes all branch results.

**`workflow`**
- Child workflow starts with its own scope; pass parent values explicitly via `inputs:`.
- `!Parent.*` built-ins are populated only when a workflow is invoked by a parent `workflow` block.
- Workflows declare their public contract via `outputs:`; a parent `workflow` block sees only those declared keys via `outputs.*`.

## Blocks

| Block | Description |
|-------|-------------|
| `http` | HTTP request |
| `grpc` | gRPC unary call via server reflection |
| `log` | Emit a message into step output |
| `postgres` | SQL query or multi-statement transaction |
| `redis` | Redis command |
| `openai` | OpenAI-compatible chat completion |
| `smtp` | Send an email via SMTP |
| `imap` | Wait for and read an email via IMAP |
| `telegram` | Telegram message |
| `container` | Docker container job |
| `delay` | Pause execution |
| `foreach` | Iterate one nested block over a list |
| `parallel` | Run heterogeneous nested blocks concurrently |
| `setvars` | Assign variables |
| `assert` | Fail if condition is not met |
| `goto` | Conditional jump (loop support) |
| `retry` | Retry one nested block with doubling backoff from `delay_ms` |
| `transform` | Build structured data and cast values |
| `workflow` | Run a nested workflow |

## Examples

### Conditional step and continue on error

```yaml
- id: "notify_on_failure"
  type: "telegram"
  condition:
    variable: "$status"
    operator: "!="
    expected: "ok"
  token: "@telegram_token"
  chat_id: "@telegram_chat_id"
  message: "Workflow failed: $status"
```

```yaml
- id: "best_effort_notify"
  type: "telegram"
  continue_on_error: true
  token: "@telegram_token"
  chat_id: "@telegram_chat_id"
  message: "Deployment finished"
```

### Workflow timeout with goto loop

```yaml
workflow_timeout_example:
  name: "Workflow Timeout Example"
  timeout_seconds: 3
  variables:
    loop_state: "loop"
  blocks:
    - id: "loop_forever"
      type: "goto"
      variable: "$loop_state"
      operator: "="
      expected: "loop"
      target_step_id: "loop_forever"
      wait_seconds: 1
```

### Retry with backoff

```yaml
- id: "wait_ready"
  type: "retry"
  max_attempts: 5
  delay_ms: 500
  retry_on:
    status_codes: [429, 500, 502, 503]
    transport_errors: true
  block:
    type: "http"
    method: "GET"
    url: "@api_host/health"
```

### Foreach with concurrency and collect

```yaml
- id: "notify_each_user"
  type: "foreach"
  items: "$users_batch"
  concurrency: 5
  block:
    type: "http"
    method: "GET"
    url: "@api_host/users/!Each.Item.id"
  collect:
    $user_ids: "item.id"
    $statuses: "output.status"
    $errors: "error"
```

### Foreach with numeric range

```yaml
- id: "generate_range"
  type: "foreach"
  items:
    from: 1
    to: 5
    step: 2
  block:
    type: "log"
    message: "Range item !Each.Item at index !Each.Index"
  collect:
    $range_values: "item"
```

### Postgres transaction

```yaml
- id: "run_postgres_transaction"
  type: "postgres"
  dsn: "@postgres_dsn"
  transaction:
    - query: "insert into users (id, status) values ($1, $2)"
      params: ["user-1", "created"]
    - query: "select count(*) as total from users where id = $1"
      params: ["user-1"]
      assign:
        $row_count: "body.rows.0.total"
    - query: "update metrics set total = $1"
      params: ["$row_count"]
```

### Parallel with collect

```yaml
- id: "prepare_context"
  type: "parallel"
  blocks:
    - id: "fetch_user"
      type: "http"
      method: "GET"
      url: "@users_url"
    - id: "read_session"
      type: "redis"
      command: "GET"
      args: ["session:key"]
  collect:
    $user: "fetch_user.output.body"
    $session: "read_session.output.body.result"
```

### TLS skip verify

```yaml
- id: "staging_healthcheck"
  type: "http"
  method: "GET"
  url: "https://internal.staging.svc/health"
  tls_skip_verify: true
```

### Transform with casts and regex

```yaml
- id: "build_payload"
  type: "transform"
  value:
    user:
      id: "$user_id"
      active: "$is_active"
  casts:
    user.id: "int"
    user.active: "bool"
  assign:
    $payload_json: "json"
```

```yaml
- id: "extract_code"
  type: "transform"
  value:
    text: "$email_text"
  regex:
    verification_code:
      from: "value.text"
      pattern: "\\b(\\d{6})\\b"
      group: 1
  assign:
    $verification_code: "extracted.verification_code"
```

### Email (SMTP + IMAP)

```yaml
- id: "send_email"
  type: "smtp"
  host: "@smtp_host"
  port: 587
  username: "@smtp_user"
  password: "@smtp_pass"
  starttls: true
  from: "noreply@example.com"
  to:
    - "$test_email"
  subject: "Verify your email"
  text: "Your code is 123456"
  html: "<p>Your code is <b>123456</b></p>"
```

```yaml
- id: "wait_email"
  type: "imap"
  host: "@imap_host"
  port: 993
  username: "@imap_user"
  password: "@imap_pass"
  tls: true
  mailbox: "INBOX"
  criteria:
    to: "$test_email"
    subject_contains: "Verify your email"
    unseen_only: true
  wait:
    timeout_ms: 60000
    poll_interval_ms: 3000
  assign:
    $email_text: "body.message.text"
```

### Nested workflow

```yaml
# Parent
- id: "nested_workflow"
  type: "workflow"
  target_workflow_id: "create_user"
  inputs:
    name: "Alice"
  assign:
    $created_id: "outputs.user_id"
```

```yaml
# Child
create_user:
  outputs:
    user_id: "$created_user_id"
  blocks:
    - id: "create"
      type: "http"
      method: "POST"
      url: "@users_create_url"
      assign:
        $created_user_id: "body.id"
```

## Docker

```bash
docker run --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  wiregoblin/wiregoblin:cli-latest \
  run http_example
```

Use `-p` if your config is not named `wiregoblin.yaml`:

```bash
docker run --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  wiregoblin/wiregoblin:cli-latest \
  run -p /workspace/config/example.project.yaml http_example
```

Pass secrets from the host with `-e`:

```bash
docker run --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  -e API_TOKEN="$API_TOKEN" \
  -e OPENAI_API_KEY="$OPENAI_API_KEY" \
  wiregoblin/wiregoblin:cli-latest \
  run -p /workspace/config/example.project.yaml http_example
```

This matches config entries like `api_token: ${API_TOKEN}` in the `secrets` section.

## Local examples

```bash
make compose-up
make run-http-example
make run-http-post-example
make run-grpc-example
make run-local-stack-example
make run-goto-example
make run-workflow-timeout-example
make run-retry-example
make run-foreach-example
make run-foreach-range-example
make run-postgres-transaction-example
make run-parallel-example
make run-transform-example
make run-email-example
make run-error-handler-example
make run-continue-on-error-example
make run-secret-variables-example
make run-workflow-block-example
make compose-down
```

Main example project: `config/example.project.yaml`.
