# 🧌 WireGoblin

Workflow runner for integration testing and automation. Define steps in YAML — call HTTP APIs, gRPC services, databases, run containers, assert results, and loop with goto.

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

Verbosity:

- default: live progress and final summary
- `-v`: add step outcome and timing
- `-vv`: add compact response summaries
- `-vvv`: add full request/response payloads
- `--json`: stream NDJSON events to stdout

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

References:

- `$name` — runtime variable or secret variable
- `@name` — project/workflow constant or secret
- `!name` — read-only runtime built-in, for example `!ErrorMessage`, `!ErrorBlockID`, or `!Parent.WorkflowID`
- secret variables are mutable but redacted in logs and step output
- assignment targets always use `$name`; if `name` exists in `secret_variables`, the write goes there
- inline interpolation is supported: `"Bearer @token"`, `"Hello, $user!"`
- inline interpolation also works inside structured string payloads such as gRPC `request` JSON: `{"sentence":"Hello, $user from !Parent.WorkflowName"}`
- `assign` maps runtime variables to block data paths; use `body.*` for the main payload and `outputs.*` for block output values such as status codes
- every step can define `condition:` with the same `variable`, `operator`, and `expected` shape as `assert`; when it does not match, the step is skipped
- every step can define `continue_on_error: true`; failed steps then get status `ignored-error`, do not stop the workflow, and do not trigger `catch_error_blocks`
- every workflow can define `timeout_seconds`; this caps total runtime for the whole workflow, not just one block
- `condition.variable`, `assert.variable`, and `goto.variable` are explicit expressions: use a leading `$` to read one runtime variable such as `$status` or `$cached_$user_id`; without `$`, the field is treated as a literal or reference-aware string
- `assert` and `goto` support `=`, `!=`, `>`, `<`, `>=`, `<=`, and `contains` (`like` is an alias)
- `goto` also supports optional `wait_seconds` before jumping to `target_step_id`
- `assert.expected` supports the same `@name` and `$name` interpolation as other reference-aware string fields
- `http` also supports optional `tls_skip_verify: true` for self-signed or staging TLS endpoints and exports `outputs.responseTimeMs` for latency assertions
- `grpc` exports `outputs.responseTimeMs` for latency assertions
- `smtp` sends one email and exposes message metadata such as `body.subject` and `outputs.message_id`
- `imap` waits for and reads one email; use `body.message.text`, `body.message.html`, `body.message.subject`, and `body.message.message_id`
- `imap.criteria` supports `message_id`, `from`, `to`, `subject_contains`, `body_contains`, and `unseen_only`
- `workflow` blocks start with the child workflow's own scope; pass parent values explicitly via `inputs:`, and use `!Parent.*` for read-only built-ins from the parent
- `!Parent.*` values are populated only when a workflow is invoked by a parent `workflow` block
- workflows declare their public contract through `outputs:`; a parent `workflow` block sees only those declared keys via `outputs.*`, and without `outputs:` nothing is exported outward
- `retry` exposes `!Retry.Attempt` and `!Retry.MaxAttempts` inside its nested `block`; `delay_ms` is the base delay and doubles after each failed attempt
- `retry` also supports optional `retry_on:` with `status_codes` and `transport_errors` to limit which errors are retried
- `foreach` exposes `!Each.Index`, `!Each.Count`, `!Each.First`, `!Each.Last`, `!Each.Item`, `!Each.ItemJSON`, and object fields like `!Each.Item.id` inside its nested `block`
- `foreach.items` accepts either an array / JSON array string or a numeric range object with `from`, `to`, and optional `step`
- `foreach` accepts optional `concurrency`; when `concurrency > 1`, iterations run in parallel, but `results` and `collect` stay in input order
- `foreach` also supports `collect:` with `$target: path` entries to aggregate per-iteration values into arrays and write them directly into runtime variables; collected values are also exposed in `body.collected.*`
- with `foreach.concurrency > 1`, nested blocks cannot use runtime `assign`; use `collect` instead
- in `foreach.collect`, use local paths like `item.*`, `output.*`, `exports.*`, plus `error` and `index` for per-iteration metadata
- when `foreach.concurrency > 1`, the block waits for all started iterations to finish; if any iteration fails, `foreach` returns one aggregate error and still includes per-iteration results and errors
- `parallel` runs heterogeneous nested blocks concurrently and exposes branch results by branch `id`
- `parallel` also supports `collect:` with paths like `fetch_user.output.body` or `read_session.exports.statusCode`; unlike `foreach.collect`, these paths must start with the branch `id`
- nested `parallel` branches cannot use runtime `assign`; use `collect` instead
- `parallel` also waits for all started branches to finish; if any branch fails, `parallel` returns one aggregate error and still includes all branch results and errors
- `collect` paths are treated as literal data paths and are not interpolated as `$`/`@`/`!` references
- `transform` returns both `value` and serialized `json`; use `casts` to convert interpolated strings into `int`, `float`, `bool`, `string`, or parsed `json`
- `transform` also supports `regex:` extraction; use `extracted.<name>` in `assign` paths after matching against `value` or `value.*`

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

Examples:

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

```yaml
- id: "staging_healthcheck"
  type: "http"
  method: "GET"
  url: "https://internal.staging.svc/health"
  tls_skip_verify: true
```

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
- id: "nested_workflow"
  type: "workflow"
  target_workflow_id: "create_user"
  inputs:
    name: "Alice"
  assign:
    $created_id: "outputs.user_id"
```

```yaml
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

```yaml
- id: "send_for_each_user"
  type: "foreach"
  items: "$users_json"
  block:
    type: "http"
    method: "POST"
    url: "@api_host/users/!Each.Item.id/notify"
  collect:
    $user_ids_json: "item.id"
    $statuses_json: "output.status"
```

Notes:

- `http`, `openai`, and `telegram` accept optional `timeout_seconds`
- project and workflow `constants` are string-valued; numeric-looking constants such as ports are still configured in the `@name` string namespace
- `smtp` and `imap` examples in `config/example.project.yaml` use the local GreenMail service from `docker-compose.example.yaml`
- `container` runs `sh -c` inside the configured image; treat `command` as trusted input
- gRPC `bytes` fields use raw strings by default; prefix with `base64:` for binary values
  Example: `request: '{"payload":"base64:SGVsbG8="}'`

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
