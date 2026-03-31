package block

import "strings"

// ResolveRuntimeVariable reads one runtime value by bare variable name, checking
// secret variables before regular variables.
func ResolveRuntimeVariable(runCtx *RunContext, name string) (string, bool) {
	if runCtx == nil {
		return "", false
	}
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", false
	}
	if value, ok := runCtx.SecretVariables[trimmed]; ok {
		return value, true
	}
	value, ok := runCtx.Variables[trimmed]
	return value, ok
}

// ResolveReferences interpolates @, $, and ! references inside one string using
// the provided field policy. Unresolved references are left untouched.
func ResolveReferences(runCtx *RunContext, value string, policy ReferencePolicy) string {
	if runCtx == nil {
		return value
	}

	var builder strings.Builder
	builder.Grow(len(value))

	for i := 0; i < len(value); {
		if !isReferencePrefix(value[i]) {
			builder.WriteByte(value[i])
			i++
			continue
		}

		prefix := value[i]
		start := i + 1
		end := start
		for end < len(value) && isReferenceNameChar(prefix, value[end]) {
			end++
		}
		if start == end {
			builder.WriteByte(value[i])
			i++
			continue
		}

		name := value[start:end]
		if resolved, ok := lookupReference(runCtx, prefix, name, policy); ok {
			builder.WriteString(resolved)
		} else {
			builder.WriteByte(prefix)
			builder.WriteString(name)
		}
		i = end
	}

	return builder.String()
}

// ResolveVariableExpression resolves a variable field using explicit syntax:
// a leading $ reads one runtime variable, while all other expressions are
// treated as reference-aware string values.
func ResolveVariableExpression(runCtx *RunContext, expr string) (string, string, bool) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return "", "", false
	}

	policy := ReferencePolicy{
		Constants:  true,
		Variables:  true,
		InlineOnly: true,
	}
	if strings.HasPrefix(trimmed, "$") {
		nameExpr := strings.TrimPrefix(trimmed, "$")
		resolvedName := ResolveReferences(runCtx, nameExpr, policy)
		value, ok := ResolveRuntimeVariable(runCtx, resolvedName)
		return "$" + resolvedName, value, ok
	}

	resolved := ResolveReferences(runCtx, trimmed, policy)
	return trimmed, resolved, true
}

func isReferencePrefix(ch byte) bool {
	return ch == '@' || ch == '$' || ch == '!'
}

func isReferenceNameChar(prefix byte, ch byte) bool {
	if prefix == '!' && ch == '.' {
		return true
	}
	return ch == '_' || ch == '-' ||
		(ch >= 'a' && ch <= 'z') ||
		(ch >= 'A' && ch <= 'Z') ||
		(ch >= '0' && ch <= '9')
}

func lookupReference(runCtx *RunContext, prefix byte, name string, policy ReferencePolicy) (string, bool) {
	switch prefix {
	case '$':
		if !policy.Variables {
			return "", false
		}
		if value, ok := runCtx.SecretVariables[name]; ok {
			return value, true
		}
		value, ok := runCtx.Variables[name]
		return value, ok
	case '@':
		if !policy.Constants {
			return "", false
		}
		if value, ok := runCtx.Secrets[name]; ok {
			return value, true
		}
		value, ok := runCtx.Constants[name]
		return value, ok
	case '!':
		// Builtins are allowed wherever constants or variables are allowed.
		if !policy.Constants && !policy.Variables {
			return "", false
		}
		value, ok := runCtx.Builtins[name]
		return value, ok
	default:
		return "", false
	}
}
