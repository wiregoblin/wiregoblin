# WireGoblin

**Test your entire stack from one YAML file.**

Chain HTTP, gRPC, Postgres, Redis, and email steps. Assert results, retry on failure, get AI-powered diagnostics when something breaks.

---

## Why WireGoblin

- Replace one-off integration scripts with declarative workflows.
- Exercise APIs, databases, email flows, containers, and nested workflows from one runner.
- Capture outputs into variables, branch on conditions, retry flaky operations, and fan out work.
- From smoke tests and local stack validation to full end-to-end and service orchestration scenarios.

## What It Looks Like

```yaml
id: "demo"
name: "Demo"

constants:
  api_host: "https://api.example.com"

secrets:
  password: "${PASSWORD}"

secret_variables:
  session_token: ""

workflows:
  - id: login_and_fetch_user
    name: "Login and Fetch User"
    blocks:
      - id: login
        type: http
        method: POST
        url: "@api_host/login"
        body: '{"email":"demo@example.com","password":"@password"}'
        assign:
          $session_token: "body.token"

      - id: get_user
        type: http
        method: GET
        url: "@api_host/users/me"
        headers:
          Authorization: "Bearer $session_token"
        assign:
          $user_id: "body.id"

      - id: assert_user
        type: assert
        variable: "$user_id"
        operator: "!="
        expected: ""
        error_message: "User id was not returned"
```

Run it with:

```bash
wiregoblin-cli run -p wiregoblin.yaml login_and_fetch_user
```

## Install

```bash
go install github.com/wiregoblin/wiregoblin/cmd/wiregoblin-cli@latest
```

## Quick Start

Run the bundled local demo stack:

```bash
make compose-up
make run-http-example
make run-local-stack-example
make compose-down
```

This starts local dependencies for the examples:

- WireMock on `http://127.0.0.1:18080`
- Postgres on `127.0.0.1:15432`
- Redis on `127.0.0.1:16379`
- GreenMail SMTP/IMAP on `127.0.0.1:13025` / `127.0.0.1:13143`

Main example project: `config/example.project.yaml`

## Run

```bash
wiregoblin-cli run
wiregoblin-cli run -p config/example.project.yaml
wiregoblin-cli run <workflow_id>
wiregoblin-cli run -p config/example.project.yaml http_example
wiregoblin-cli run -e .env -p config/example.project.yaml http_example
wiregoblin-cli run --ai-summary-success -p config/example.project.yaml http_example
```

If `<workflow_id>` is omitted, WireGoblin runs all workflows sequentially in config order.

If `-p` is omitted, WireGoblin looks for `wiregoblin.yaml` or `wiregoblin.yml` in the current directory.

Use `-e` to load a `.env` file before running. Variables are injected into the process environment and resolved in all `${VAR}` / `${VAR:=default}` references.

**Verbosity:**

| Flag | Output |
|------|--------|
| _(default)_ | Live progress and final summary |
| `-v` | Step outcome and timing |
| `-vv` | Compact response summaries |
| `-vvv` | Full request/response payloads |
| `--json` | Stream NDJSON events to stdout |

---

## AI Analysis

WireGoblin can connect to a local AI model (Ollama, LM Studio, or any OpenAI-compatible server) to automatically analyze run results.

**On failure** — when `ai.enabled: true` is set, WireGoblin sends the failing step context (step type, error, request, response) to the model and prints a structured explanation to `stderr`:

```
Summary: The HTTP block failed with a 401 Unauthorized response.
Likely cause: The Authorization header references $session_token which was
  not assigned before this step ran.
Next checks:
- Verify the login step assigns $session_token via `assign`
- Check that the login step runs before get_user in the workflow order
- Confirm the token format expected by the API
```

**On success** — pass `--ai-summary-success` to also receive a digest after a successful run, highlighting key outcomes, notable behavior (retries, skips, slow steps), and a summary of important blocks.

AI output is always printed to `stderr` and never affects the workflow exit code.

### Supported providers

| Provider | `provider` value |
|----------|-----------------|
| [Ollama](https://ollama.com) | `ollama` |
| LM Studio / any OpenAI-compatible server | `openai_compatible` |

### AI config reference

```yaml
ai:
  enabled: true
  provider: "ollama"               # ollama | openai_compatible
  base_url: "${OLLAMA_URL:=http://127.0.0.1:11434}"
  model: "${OLLAMA_MODEL:=qwen3:4b}"
  timeout_seconds: 30
  redact_secrets: true             # strip secrets from prompts (default: true)
```

**Fields:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` (if section present) | Enables AI features |
| `provider` | string | — | `ollama` or `openai_compatible` |
| `base_url` | string | — | Base URL of the AI server |
| `model` | string | — | Model name to use |
| `timeout_seconds` | int | — | Per-request timeout |
| `redact_secrets` | bool | `true` | Redact secrets before building prompts |

**Quick start:**

```bash
make run-ai-summary-example OLLAMA_URL=http://127.0.0.1:11434 OLLAMA_MODEL=qwen3:4b
make run-ai-success-summary-example OLLAMA_URL=http://127.0.0.1:11434 OLLAMA_MODEL=qwen3:4b
```

---

## Config

Projects are defined in one YAML file with:

- project metadata
- optional shared AI settings
- constants, secrets, variables, and secret variables
- one or more workflows composed from blocks

```yaml
id: "my-project"
name: "My Project"

ai:
  enabled: true
  provider: "ollama"
  base_url: "${OLLAMA_URL:=http://127.0.0.1:11434}"
  model: "${OLLAMA_MODEL:=qwen3:4b}"

constants:
  api_host: "${API_HOST:=https://api.example.com}"

secrets:
  api_token: "${API_TOKEN}"

variables:
  page_size: "${PAGE_SIZE:=20}"

secret_variables:
  session_token: ""

workflows:
  - id: my_workflow
    name: "My Workflow"
    timeout_seconds: 60

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
| `!name` | Built-in: `!RunID`, `!StartTime`, `!WorkflowID`, `!ErrorMessage`, `!Parent.WorkflowID`, etc. |

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

---

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
| `slack` | Send a Slack message |
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

Common patterns:

- smoke-test APIs with `http`, `grpc`, and `assert`
- validate local stacks with `postgres`, `redis`, `container`, `smtp`, and `imap`
- recover from flaky dependencies with `retry`, `goto`, and `continue_on_error`
- reshape and aggregate data with `transform`, `foreach`, and `parallel`
- reuse flows with nested `workflow` blocks

Full field reference, exports, and examples: **[docs/reference.md](docs/reference.md)**

---

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

---

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
make run-retry-exhaustion-example
make run-foreach-example
make run-foreach-range-example
make run-parallel-example
make run-postgres-transaction-example
make run-transform-example
make run-email-example
make run-error-handler-example
make run-continue-on-error-example
make run-secret-variables-example
make run-workflow-block-example
make compose-down
```

Notable workflows in `config/example.project.yaml`:

- `http_example`: request, extract fields, and assert status/timing
- `local_stack_example`: HTTP + Redis + Postgres + container + local AI/Telegram mocks
- `retry_example`: retry transport and status-code failures with backoff
- `foreach_example`: iterate over structured items and collect outputs
- `parallel_example`: run multiple heterogeneous blocks concurrently
- `workflow_block_example`: compose parent and child workflows

## AI assistant skill

See [`docs/ai.md`](docs/ai.md) for Codex and Claude Code usage.
