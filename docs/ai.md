# AI Assistant Usage

[`SKILL.md`](./SKILL.md) teaches an AI assistant the full WireGoblin config format: block types, field names, assign paths, and reference syntax. Use it to generate or extend project YAML from a plain-language description.

## Codex

Install it as a Codex skill once, then reuse it from any project.

```bash
mkdir -p "${CODEX_HOME:-$HOME/.codex}/skills/write-config"
curl -o "${CODEX_HOME:-$HOME/.codex}/skills/write-config/SKILL.md" \
  https://raw.githubusercontent.com/wiregoblin/wiregoblin/main/docs/SKILL.md
```

Restart Codex after installing the skill.

Example prompt:

```text
Use the write-config skill to create a WireGoblin workflow that calls the auth API, stores the token, and asserts a 200 response.
```

## LM Studio

LM Studio does not load project skills automatically, so use [`docs/SKILL.md`](./SKILL.md) as prompt context. Paste it into the chat or system prompt, then ask the model to generate or edit a WireGoblin config.

Example prompt:

```text
You generate valid WireGoblin YAML configs.

Follow these rules:
[paste docs/SKILL.md here]

Task:
Create a WireGoblin workflow that logs in to an API, saves the session token, fetches the user profile, and asserts a 200 response.
```

## Ollama

Ollama also does not load project skills automatically, so use [`docs/SKILL.md`](./SKILL.md) as instruction text. Paste it into the prompt or system message before your request.

Example:

```text
You generate valid WireGoblin YAML configs.

Follow these rules:
[paste docs/SKILL.md here]

Task:
Create a workflow that calls the auth API, stores the token, and asserts a 200 response.
```

## Claude Code

Install it as a project skill once per repository to get a `/write-config` command:

```bash
mkdir -p .claude/skills/write-config
curl -o .claude/skills/write-config/SKILL.md \
  https://raw.githubusercontent.com/wiregoblin/wiregoblin/main/docs/SKILL.md
```

Then run:

```text
/write-config create a workflow that calls the auth API, checks the token, and asserts a 200 response
```
