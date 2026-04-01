# WireGoblin

Declarative workflow runner for integration testing and service orchestration. Define steps in YAML — call HTTP APIs, gRPC services, databases, send emails, run containers, assert results, loop with goto.

No glue code. No one-off scripts. One workflow file, one command.

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

**Verbosity:**

| Flag | Output |
|------|--------|
| _(default)_ | Live progress and final summary |
| `-v` | Step outcome and timing |
| `-vv` | Compact response summaries |
| `-vvv` | Full request/response payloads |
| `--json` | Stream NDJSON events to stdout |

## Config

```yaml
id: "my-project"
name: "My Project"

constants:
  api_host: "https://api.example.com"

secrets:
  api_token: "${API_TOKEN}"

variables:
  page_size: "20"

secret_variables:
  session_token: ""

workflows:
  my_workflow:
    name: "My Workflow"
    timeout_seconds: 60

    # Workflow-level sections override project-level values with the same key.
    constants: {}
    secrets: {}
    variables: {}
    secret_variables: {}

    blocks:
      - id: "get_user"
        type: "http"
        method: "GET"
        url: "@api_host/users/1"
        headers:
          Authorization: "Bearer $session_token"
        assign:
          $user_id: "body.id"
```

### References

| Prefix | Resolves to |
|--------|-------------|
| `$name` | Runtime variable or secret variable |
| `@name` | Constant or secret |
| `!name` | Built-in: `!ErrorMessage`, `!ErrorBlockID`, `!Parent.WorkflowID`, etc. |

Interpolation works inside any string field: `"Bearer @token"`, `"Hello, $user!"`.

### Variables and assign

- `assign` maps block output paths to variables: `$var: "body.field"` or `$ms: "outputs.responseTimeMs"`.
- `secret_variables` are mutable but redacted in all logs and output.
- `collect` paths in `foreach`/`parallel` are literal data paths, not interpolated.

### Step control

- `condition:` — skip a step unless `variable operator expected` is satisfied.
- `continue_on_error: true` — failed step gets status `ignored-error`; workflow continues.
- `timeout_seconds` on a workflow — caps total runtime for that workflow.

### Operators

`assert` and `goto` support: `=`, `!=`, `>`, `<`, `>=`, `<=`, `contains` (`like` is an alias).

## Blocks

| Block | Description |
|-------|-------------|
| `http` | HTTP request |
| `grpc` | gRPC unary call via server reflection |
| `postgres` | SQL query or multi-statement transaction |
| `redis` | Redis command |
| `openai` | OpenAI-compatible chat completion |
| `smtp` | Send an email via SMTP |
| `imap` | Wait for and read an email via IMAP |
| `telegram` | Telegram message |
| `container` | Docker container job |
| `delay` | Pause execution |
| `log` | Emit a message to step output |
| `setvars` | Assign variables |
| `assert` | Fail if condition is not met |
| `goto` | Conditional jump (loop support) |
| `transform` | Build structured data and cast types |
| `retry` | Retry one nested block with backoff |
| `foreach` | Iterate one nested block over a list |
| `parallel` | Run heterogeneous blocks concurrently |
| `workflow` | Invoke a nested workflow |

Full field reference, exports, and examples: **[docs/blocks.md](docs/blocks.md)**

## Docker

```bash
docker run --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  wiregoblin/wiregoblin:cli-latest \
  run http_example
```

Pass secrets from the host with `-e`:

```bash
docker run --rm \
  -v "$(pwd):/workspace" \
  -w /workspace \
  -e API_TOKEN="$API_TOKEN" \
  wiregoblin/wiregoblin:cli-latest \
  run -p /workspace/config/example.project.yaml http_example
```

## Local examples

```bash
make compose-up
make run-http-example
make run-grpc-example
make run-goto-example
make run-retry-example
make run-foreach-example
make run-parallel-example
make run-postgres-transaction-example
make run-transform-example
make run-email-example
make run-workflow-block-example
make compose-down
```

Main example project: `config/example.project.yaml`.

## AI assistant skill

See [`docs/ai.md`](docs/ai.md) for Codex and Claude Code usage.
