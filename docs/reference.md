# Block Reference

Every block is a step in a workflow. All steps share a set of common fields; the rest are block-specific.

## Common step fields

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Step identifier — used by `goto`, `assign`, and error built-ins |
| `type` | string | Block type (see table below) |
| `condition` | object | Skip this step unless condition is met |
| `condition.variable` | string | Value to test — use `$var`, `@const`, `!built-in`, or a literal |
| `condition.operator` | string | `=`, `!=`, `>`, `<`, `>=`, `<=`, `contains` |
| `condition.expected` | string | Expected value; supports `$`/`@` interpolation |
| `continue_on_error` | bool | When `true`, a failed step gets status `ignored-error` and the workflow continues |
| `assign` | map | Copy block output paths to runtime variables: `$var: "body.field"` |

### assign paths

| Prefix | Source |
|--------|--------|
| `body.*` | Main response payload |
| `outputs.*` | Block-specific output values (e.g. `outputs.responseTimeMs`) |

---

## Environment variable injection

Any value in `constants`, `secrets`, `variables`, or `secret_variables` — at both the project level and the workflow level — can reference an environment variable using `${VAR}` syntax.

| Syntax | Behaviour |
|--------|-----------|
| `"${VAR}"` | Replaced with the value of `$VAR`; empty string if the variable is not set |
| `"${VAR:=default}"` | Replaced with the value of `$VAR`; falls back to `default` if the variable is unset or empty |

The substitution happens at config load time, before the workflow runs.

```yaml
constants:
  api_host: "${API_HOST:=https://api.example.com}"

secrets:
  api_token: "${API_TOKEN}"

variables:
  page_size: "${PAGE_SIZE:=20}"

secret_variables:
  session_token: "${SESSION_TOKEN:=}"
```

Use `-e` to load a `.env` file before running:

```bash
wiregoblin-cli run -e .env my_workflow
```

---

## Global built-ins

Available in every step of every workflow via `!name`.

| Built-in | Example value | Description |
|----------|---------------|-------------|
| `!RunID` | `"f47ac10b-58cc-..."` | Unique UUID generated at the start of each run — useful as a correlation ID |
| `!StartTime` | `"2026-04-01T12:00:00Z"` | Workflow start time in RFC 3339 (UTC) |
| `!StartUnix` | `"1743508800"` | Workflow start time as Unix epoch seconds — useful for TTL/expiry arithmetic |
| `!StartDate` | `"2026-04-01"` | Workflow start date (`YYYY-MM-DD`, UTC) |
| `!ProjectID` | `"my-project"` | ID of the project this workflow belongs to |
| `!WorkflowID` | `"create_user"` | ID of the current workflow |
| `!WorkflowName` | `"Create User"` | Display name of the current workflow |
| `!BlockStartTime` | `"2026-04-01T12:00:05Z"` | Start time of the current step in RFC 3339 (UTC) |
| `!BlockStartUnix` | `"1743508805"` | Start time of the current step as Unix epoch seconds |

### Error built-ins

Populated automatically when a step fails and the workflow has `on_error` steps.

| Built-in | Description |
|----------|-------------|
| `!ErrorMessage` | Error message from the failed step |
| `!ErrorBlockID` | `id` of the failed step |
| `!ErrorBlockName` | `name` of the failed step |
| `!ErrorBlockType` | Block type of the failed step |
| `!ErrorBlockIndex` | 1-based index of the failed step |

### Parent built-ins

Populated inside a child workflow invoked via a `workflow` block.

| Built-in | Description |
|----------|-------------|
| `!Parent.WorkflowID` | Parent workflow ID |
| `!Parent.WorkflowName` | Parent workflow name |
| `!Parent.RunID` | Parent run ID |
| `!Parent.StartTime` | Parent start time |

---

## Blocks

| Block | Description |
|-------|-------------|
| [`http`](#http) | HTTP request |
| [`grpc`](#grpc) | gRPC unary call via server reflection |
| [`postgres`](#postgres) | SQL query or multi-statement transaction |
| [`redis`](#redis) | Redis command |
| [`openai`](#openai) | OpenAI-compatible chat completion |
| [`smtp`](#smtp) | Send an email via SMTP |
| [`imap`](#imap) | Wait for and read an email via IMAP |
| [`telegram`](#telegram) | Send a Telegram message |
| [`container`](#container) | Run a command inside a Docker container |
| [`delay`](#delay) | Pause execution |
| [`log`](#log) | Emit a message to step output |
| [`setvars`](#setvars) | Assign variables |
| [`assert`](#assert) | Fail the workflow if a condition is not met |
| [`goto`](#goto) | Conditional jump |
| [`transform`](#transform) | Build structured data and cast types |
| [`retry`](#retry) | Retry one nested block with backoff |
| [`foreach`](#foreach) | Iterate one nested block over a list |
| [`parallel`](#parallel) | Run heterogeneous blocks concurrently |
| [`workflow`](#workflow) | Invoke a nested workflow |

---

## http

Sends an HTTP request.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `method` | string | yes | | `GET`, `POST`, `PUT`, `PATCH`, `DELETE`, etc. |
| `url` | string | yes | | Request URL; supports `$`/`@` interpolation |
| `body` | string | no | | Raw request body |
| `headers` | map | no | | HTTP headers |
| `timeout_seconds` | int | no | | Per-request timeout |
| `tls_skip_verify` | bool | no | `false` | Skip TLS certificate verification |
| `sign` | object | no | | Request signing config (see [HTTP request signing](#http-request-signing)) |

**Exports:**

| Path | Description |
|------|-------------|
| `body.*` | Parsed JSON response body |
| `outputs.responseTimeMs` | Round-trip time in milliseconds |

```yaml
- id: "get_user"
  type: "http"
  method: "GET"
  url: "@api_host/users/1"
  headers:
    Authorization: "Bearer $token"
  assign:
    $user_id: "body.id"
    $response_ms: "outputs.responseTimeMs"
```

---

## http request signing

The `sign` block inside `http` computes an HMAC or RSA signature and writes it to a header, query parameter, or JSON body field.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `type` | string | yes | | `hmac_sha256`, `hmac_sha512`, or `rsa_sha256` |
| `key` | string | yes | | Signing key or PEM-encoded RSA private key; supports `$`/`@` references. For RSA keys stored as single-line secrets, `\n` sequences are treated as newlines. |
| `include` | []string | no | `[body]` | Parts to sign: `method`, `url`, `body` |
| `separator` | string | no | `\n` | Separator inserted between included parts |
| `body_format` | string | no | `raw` | `raw` or `sorted_json` — sorts all JSON keys recursively before signing |
| `header` | string | one of | | Write signature to this header |
| `query_param` | string | one of | | Write signature to this query parameter |
| `body_field` | string | one of | | Inject signature as a field into the JSON body |
| `prefix` | string | no | | Prefix prepended to the signature value (e.g. `sha256=`) |

Exactly one of `header`, `query_param`, or `body_field` must be set.

**Signature in a header:**

```yaml
- id: "call_webhook"
  type: "http"
  method: "POST"
  url: "https://api.example.com/webhook"
  body: '{"event":"deploy"}'
  sign:
    type: hmac_sha256
    key: "@webhook_secret"
    include: [body]
    header: X-Hub-Signature-256
    prefix: "sha256="
```

**Signature in a query parameter:**

```yaml
- id: "signed_request"
  type: "http"
  method: "POST"
  url: "https://api.example.com/events"
  body: '{"id":"123"}'
  sign:
    type: hmac_sha256
    key: "@api_secret"
    include: [method, url, body]
    query_param: sig
```

**Signature injected into the JSON body:**

```yaml
- id: "signed_payment"
  type: "http"
  method: "POST"
  url: "https://api.example.com/pay"
  body: '{"amount":100,"currency":"USD"}'
  sign:
    type: hmac_sha256
    key: "@payment_secret"
    include: [body]
    body_format: sorted_json
    body_field: sign
```

With `body_format: sorted_json`, all JSON keys are sorted recursively before signing. The receiver replicates the same sort-and-sign to verify.

**RSA-SHA256:**

```yaml
- id: "signed_request"
  type: "http"
  method: "POST"
  url: "https://api.example.com/payments"
  body: '{"amount":100}'
  sign:
    type: rsa_sha256
    key: "@rsa_private_key"
    include: [body]
    header: X-Signature
    prefix: "rsa-sha256="
```

The RSA signature is base64-encoded (standard encoding). The key must be a PEM-encoded RSA private key in PKCS#1 (`RSA PRIVATE KEY`) or PKCS#8 (`PRIVATE KEY`) format.

---

## grpc

Sends a gRPC unary request using server reflection.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `address` | string | yes | | Server address, e.g. `localhost:50051` |
| `method` | string | yes | | Fully qualified method, e.g. `package.Service/Method` |
| `request` | string | no | | JSON-encoded request body; supports `$`/`@`/`!` interpolation |
| `metadata` | map | no | | gRPC metadata headers |
| `tls` | bool | no | `false` | Enable TLS |

`bytes` fields in `request` use raw strings by default. Prefix with `base64:` for binary: `{"payload":"base64:SGVsbG8="}`.

**Exports:**

| Path | Description |
|------|-------------|
| `body.*` | Parsed response message |
| `outputs.responseTimeMs` | Round-trip time in milliseconds |

```yaml
- id: "grpc_call"
  type: "grpc"
  address: "@grpc_host"
  method: "users.UserService/GetUser"
  request: '{"id":"$user_id"}'
  metadata:
    Authorization: "Bearer $token"
  assign:
    $name: "body.name"
```

---

## postgres

Runs a SQL query or a multi-statement transaction.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `dsn` | string | yes | PostgreSQL DSN, e.g. `postgres://user:pass@host/db` |
| `query` | string | one of | Single SQL query |
| `params` | []any | no | Positional parameters for `query` |
| `transaction` | []object | one of | Ordered list of statements (run in a single transaction) |
| `transaction[].query` | string | yes | SQL statement |
| `transaction[].params` | []any | no | Positional parameters |
| `transaction[].assign` | map | no | Assign row values to variables after this statement |

Use either `query` (single) or `transaction` (multi-statement), not both.

**Exports:**

| Path | Description |
|------|-------------|
| `body.rows` | Array of result rows for the last (or only) query |
| `body.rows_affected` | Rows affected by the last (or only) statement |

```yaml
- id: "get_user"
  type: "postgres"
  dsn: "@postgres_dsn"
  query: "SELECT id, name FROM users WHERE id = $1"
  params: ["$user_id"]
  assign:
    $name: "body.rows.0.name"
```

```yaml
- id: "run_transaction"
  type: "postgres"
  dsn: "@postgres_dsn"
  transaction:
    - query: "INSERT INTO users (id, status) VALUES ($1, $2)"
      params: ["user-1", "created"]
    - query: "SELECT count(*) AS total FROM users WHERE id = $1"
      params: ["user-1"]
      assign:
        $row_count: "body.rows.0.total"
    - query: "UPDATE metrics SET total = $1"
      params: ["$row_count"]
```

---

## redis

Runs a Redis command.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `address` | string | yes | | Redis address, e.g. `localhost:6379` |
| `command` | string | no | `PING` | Redis command |
| `args` | []string | no | | Command arguments; supports `$`/`@` interpolation |
| `password` | string | no | | Redis password |
| `db` | int | no | `0` | Redis database index |
| `timeout_seconds` | int | no | `5` | Command timeout |

**Exports:**

| Path | Description |
|------|-------------|
| `body.result` | Command result value |

```yaml
- id: "cache_user"
  type: "redis"
  address: "@redis_address"
  command: "SET"
  args: ["user:$user_id", "$user_json", "EX", "3600"]
```

---

## openai

Sends a chat completion request to an OpenAI-compatible API.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `token` | string | yes | API token; supports `@`/`$` references |
| `model` | string | yes | Model name, e.g. `gpt-4o` |
| `user_prompt` | string | yes | User message content |
| `base_url` | string | no | API base URL (default: OpenAI) |
| `system_prompt` | string | no | System message content |
| `temperature` | string | no | Sampling temperature |
| `max_tokens` | string | no | Maximum tokens to generate |
| `headers` | map | no | Extra HTTP headers |
| `timeout_seconds` | int | no | Request timeout |

**Exports:**

| Path | Description |
|------|-------------|
| `body.content` | Generated text |
| `body.model` | Model used |
| `body.usage.*` | Token usage stats |

```yaml
- id: "summarize"
  type: "openai"
  token: "@openai_key"
  model: "gpt-4o"
  system_prompt: "You are a concise summarizer."
  user_prompt: "Summarize: $article_text"
  assign:
    $summary: "body.content"
```

---

## smtp

Sends an email via SMTP.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `host` | string | yes | | SMTP server host |
| `port` | int | yes | | SMTP server port |
| `username` | string | yes | | SMTP username |
| `password` | string | yes | | SMTP password |
| `from` | string | yes | | Sender address |
| `to` | []string | yes | | Recipient addresses |
| `subject` | string | yes | | Email subject |
| `tls` | bool | no | `false` | Use implicit TLS (port 465) |
| `starttls` | bool | no | `false` | Use STARTTLS (port 587) |
| `cc` | []string | no | | CC addresses |
| `bcc` | []string | no | | BCC addresses |
| `text` | string | no | | Plain text body |
| `html` | string | no | | HTML body |
| `timeout_seconds` | int | no | | Send timeout |

**Exports:**

| Path | Description |
|------|-------------|
| `body.subject` | Subject of the sent email |
| `outputs.message_id` | SMTP Message-ID |

```yaml
- id: "send_email"
  type: "smtp"
  host: "@smtp_host"
  port: 587
  username: "@smtp_user"
  password: "@smtp_pass"
  starttls: true
  from: "noreply@example.com"
  to: ["$test_email"]
  subject: "Verify your email"
  text: "Your code is 123456"
  html: "<p>Your code is <b>123456</b></p>"
```

---

## imap

Waits for a matching email and reads it.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `host` | string | yes | | IMAP server host |
| `port` | int | yes | | IMAP server port |
| `username` | string | yes | | IMAP username |
| `password` | string | yes | | IMAP password |
| `tls` | bool | no | `false` | Use TLS |
| `mailbox` | string | no | `INBOX` | Mailbox to search |
| `criteria` | object | no | | Search criteria |
| `criteria.message_id` | string | no | | Match by Message-ID |
| `criteria.from` | string | no | | Match by sender address |
| `criteria.to` | string | no | | Match by recipient address |
| `criteria.subject_contains` | string | no | | Subject substring match |
| `criteria.body_contains` | string | no | | Body substring match |
| `criteria.unseen_only` | bool | no | `false` | Only match unseen messages |
| `wait.timeout_ms` | int | no | | How long to poll before failing |
| `wait.poll_interval_ms` | int | no | `1000` | How often to poll |
| `select_mode` | string | no | `latest` | Which matching message to select |
| `mark_as_seen` | bool | no | `false` | Mark the matched message as seen |
| `delete` | bool | no | `false` | Delete the matched message after reading |
| `timeout_seconds` | int | no | | Overall block timeout |

**Exports:**

| Path | Description |
|------|-------------|
| `body.message.subject` | Email subject |
| `body.message.text` | Plain text body |
| `body.message.html` | HTML body |
| `body.message.message_id` | Message-ID header |

```yaml
- id: "wait_email"
  type: "imap"
  host: "@imap_host"
  port: 993
  username: "@imap_user"
  password: "@imap_pass"
  tls: true
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

---

## telegram

Sends a message via the Telegram Bot API.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `token` | string | yes | Telegram bot token |
| `chat_id` | string | yes | Target chat ID |
| `message` | string | yes | Message text; supports `$`/`@`/`!` interpolation |
| `parse_mode` | string | no | `HTML` or `Markdown` |
| `base_url` | string | no | Alternative Telegram API base URL |
| `timeout_seconds` | int | no | Request timeout |

```yaml
- id: "notify"
  type: "telegram"
  token: "@telegram_token"
  chat_id: "@chat_id"
  message: "Deploy of $service finished with status: $status"
```

---

## container

Runs a shell command inside a Docker container.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `image` | string | yes | | Docker image |
| `command` | string | yes | | Command passed to `sh -c`; treat as trusted input |
| `env` | map | no | | Environment variables |
| `workdir` | string | no | `/workspace` | Working directory inside the container |
| `mount_source` | string | no | | Host path to mount into `workdir` |
| `timeout_seconds` | int | no | `300` | Container run timeout |
| `docker_path` | string | no | | Path to the `docker` binary |

**Exports:**

| Path | Description |
|------|-------------|
| `body.stdout` | Command stdout |
| `body.exit_code` | Exit code |

```yaml
- id: "run_migration"
  type: "container"
  image: "migrate/migrate:latest"
  command: "migrate -path /migrations -database $db_url up"
  env:
    db_url: "@postgres_dsn"
  mount_source: "./migrations"
```

---

## delay

Pauses execution.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `milliseconds` | int | no | `1000` | How long to pause |

```yaml
- id: "wait"
  type: "delay"
  milliseconds: 2000
```

---

## log

Emits a message to step output.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `message` | string | yes | | Message text; supports `$`/`@`/`!` interpolation |
| `level` | string | no | `info` | `info`, `debug`, `warn`, or `error` |

```yaml
- id: "debug_user"
  type: "log"
  message: "User $user_id has status: $status"
  level: "debug"
```

---

## setvars

Assigns one or more variables.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `set` | map | yes | Keys are variable names (with `$` prefix); values support `$`/`@`/`!` interpolation |

```yaml
- id: "init"
  type: "setvars"
  set:
    $status: "pending"
    $token: "@api_token"
```

---

## assert

Fails the workflow if a condition is not met.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `variable` | string | yes | Value to test — use `$var`, `@const`, `!built-in`, or a literal |
| `operator` | string | yes | `=`, `!=`, `>`, `<`, `>=`, `<=`, `contains` |
| `expected` | string | yes | Expected value; supports `$`/`@` interpolation |
| `error_message` | string | no | Custom failure message |

```yaml
- id: "check_status"
  type: "assert"
  variable: "$status_code"
  operator: "="
  expected: "200"
  error_message: "Expected HTTP 200 but got $status_code"
```

---

## goto

Jumps to another step if a condition is met. Used to build loops.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `variable` | string | yes | Value to test — use `$var`, `@const`, `!built-in`, or a literal |
| `operator` | string | yes | `=`, `!=`, `>`, `<`, `>=`, `<=`, `contains` |
| `expected` | string | yes | Expected value; supports `$`/`@` interpolation |
| `target_step_id` | string | yes | ID of the step to jump to |
| `wait_seconds` | int | no | Pause before jumping |

```yaml
- id: "poll_until_ready"
  type: "goto"
  variable: "$job_status"
  operator: "!="
  expected: "done"
  target_step_id: "poll_until_ready"
  wait_seconds: 5
```

---

## transform

Builds a structured value, casts types, and extracts data with regex.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `value` | any | no | Arbitrary value (object, array, string); supports `$`/`@`/`!` interpolation in strings |
| `casts` | map | no | Dot-path → target type: `int`, `float`, `bool`, `string`, `json` |
| `regex` | map | no | Named regex extractions (see below) |
| `regex.<name>.from` | string | yes | Dot-path in `value` to read from |
| `regex.<name>.pattern` | string | yes | Regular expression |
| `regex.<name>.group` | int | no | Capture group index (0 = full match) |

**Exports:**

| Path | Description |
|------|-------------|
| `body.value` | Computed value |
| `body.json` | `value` serialized to a JSON string |
| `body.extracted.<name>` | Each regex extraction result |

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
    $payload_json: "body.json"
```

```yaml
- id: "extract_code"
  type: "transform"
  value:
    text: "$email_body"
  regex:
    verification_code:
      from: "value.text"
      pattern: "\\b(\\d{6})\\b"
      group: 1
  assign:
    $code: "body.extracted.verification_code"
```

---

## retry

Retries one nested block with exponential backoff.

The step succeeds only when the nested block stops matching the retry condition before the attempt budget is exhausted. If the final attempt still matches `retry_on`, the `retry` step fails with an exhaustion error even when the nested block itself returned no transport error.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `block` | object | yes | | The block to retry (any block type) |
| `max_attempts` | int | no | `1` | Maximum number of attempts |
| `delay_ms` | int | no | | Initial delay before first retry; doubles after each failure |
| `retry_on` | object | no | | Rules that decide whether another attempt should run |
| `retry_on.match` | string | no | `any` | `any` retries when one rule matches; `all` requires every rule to match |
| `retry_on.rules` | []object | yes* | | Retry rules evaluated against the last result/error when `retry_on` is present |
| `retry_on.rules[].type` | string | yes | | One of `transport_error`, `status_code`, `path` |
| `retry_on.rules[].in` | []int | status_code | | Status codes that should be retried |
| `retry_on.rules[].path` | string | path | | Result path such as `body.data` or `body.items.length` |
| `retry_on.rules[].operator` | string | path | | One of `empty`, `not_empty`, `=`, `!=`, `>`, `<`, `>=`, `<=` |
| `retry_on.rules[].expected` | any | path* | | Required for comparison operators other than `empty` and `not_empty` |

**Built-ins available inside `block`:**

| Built-in | Description |
|----------|-------------|
| `!Retry.Attempt` | Current attempt number (1-based) |
| `!Retry.MaxAttempts` | Configured max attempts |

**Output fields:**

| Field | Type | Description |
|-------|------|-------------|
| `attempts` | int | Number of attempts that ran |
| `max_attempts` | int | Configured attempt budget |
| `delay_ms` | int | Initial retry delay |
| `succeeded` | bool | Whether retry finished successfully |
| `retryable` | bool | Whether the last attempt still matched `retry_on` |
| `stopped_early` | bool | Whether execution stopped before exhausting attempts because the last failure was not retryable |
| `result` | any | Nested block output from the last attempt |
| `last_error` | string | Last nested error, or `retry exhausted after N attempts` on exhaustion |
| `history` | []object | Per-attempt records for logging and JSON output |

**`history[]` fields:**

| Field | Type | Description |
|-------|------|-------------|
| `attempt` | int | 1-based attempt index |
| `request` | object | Resolved request config for that attempt |
| `result` | any | Nested block output for that attempt |
| `error` | string | Nested block error text, if any |
| `retryable` | bool | Whether this attempt triggered another retry |
| `next_delay_ms` | int | Delay before the next attempt, or `0` for the final attempt |

```yaml
- id: "wait_ready"
  type: "retry"
  max_attempts: 5
  delay_ms: 500
  retry_on:
    match: "any"
    rules:
      - type: "transport_error"
      - type: "status_code"
        in: [429, 500, 502, 503]
      - type: "path"
        path: "body.data"
        operator: "empty"
      - type: "path"
        path: "body.items.length"
        operator: "="
        expected: 0
  block:
    type: "http"
    method: "GET"
    url: "@api_host/health"
```

---

## foreach

Iterates one nested block over a list of items.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `items` | any | yes | Array, JSON array string, or a range object (`from`, `to`, `step`) |
| `block` | object | yes | The block to run per item |
| `concurrency` | int | no | Run up to N iterations concurrently (default: sequential) |
| `collect` | map | no | Aggregate per-iteration values into arrays |

**`collect` paths:**

| Prefix | Source |
|--------|--------|
| `item.*` | Item value or field |
| `output.*` | Block output (same paths as `assign`) |
| `exports.*` | Variables assigned by the block |
| `error` | Error message if the iteration failed |
| `index` | Iteration index |

**Built-ins available inside `block`:**

| Built-in | Description |
|----------|-------------|
| `!Each.Index` | 0-based index |
| `!Each.Count` | Total item count |
| `!Each.First` | `true` on first iteration |
| `!Each.Last` | `true` on last iteration |
| `!Each.Item` | Current item value |
| `!Each.ItemJSON` | Current item as JSON string |
| `!Each.Item.<field>` | Field of the current item (for objects) |

When `concurrency > 1`, nested blocks cannot use runtime `assign` — use `collect` instead. Results and `collect` output remain in input order.

```yaml
- id: "fetch_each_user"
  type: "foreach"
  items: "$user_ids"
  concurrency: 5
  block:
    type: "http"
    method: "GET"
    url: "@api_host/users/!Each.Item"
  collect:
    $names: "output.body.name"
    $errors: "error"
```

**Numeric range:**

```yaml
- id: "range_loop"
  type: "foreach"
  items:
    from: 1
    to: 10
    step: 2
  block:
    type: "log"
    message: "Item !Each.Item at index !Each.Index"
```

---

## parallel

Runs heterogeneous blocks concurrently.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `blocks` | []object | yes | List of blocks to run concurrently; each must have an `id` |
| `collect` | map | no | Aggregate branch outputs into variables |

**`collect` paths** must start with the branch `id`:

```
<branch_id>.output.body.*
<branch_id>.output.outputs.*
<branch_id>.error
```

Nested branches cannot use runtime `assign` — use `collect` instead. All branches run to completion; any failure returns an aggregate error but all branch results are still available.

```yaml
- id: "prepare"
  type: "parallel"
  blocks:
    - id: "fetch_user"
      type: "http"
      method: "GET"
      url: "@users_url"
    - id: "read_session"
      type: "redis"
      address: "@redis_address"
      command: "GET"
      args: ["session:$session_key"]
  collect:
    $user: "fetch_user.output.body"
    $session: "read_session.output.body.result"
```

---

## workflow

Invokes another workflow defined in the same project.

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `target_workflow_id` | string | yes | ID of the workflow to invoke |
| `inputs` | map | no | Values to pass into the child workflow's scope |

The child workflow starts with its own isolated scope. Pass parent values explicitly via `inputs`. Child workflows expose their public contract via `outputs`; the parent sees only those declared keys via `outputs.*`.

`!Parent.*` built-ins are populated inside the child only when invoked via a `workflow` block.

```yaml
# Parent workflow
- id: "create"
  type: "workflow"
  target_workflow_id: "create_user"
  inputs:
    name: "$new_name"
  assign:
    $created_id: "outputs.user_id"
```

```yaml
# Child workflow
create_user:
  outputs:
    user_id: "$created_user_id"
  blocks:
    - id: "post"
      type: "http"
      method: "POST"
      url: "@api_host/users"
      body: '{"name":"$name"}'
      assign:
        $created_user_id: "body.id"
```
