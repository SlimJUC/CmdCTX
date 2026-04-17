# Security Policy

## Philosophy

`cmdctx` is built with safety as a first-class feature, not an afterthought.

**Core guarantees:**
- No command executes without explicit user confirmation
- AI output is never used as raw shell code
- All commands are validated against a hardcoded policy before display
- Sensitive data is redacted before being stored in history
- No sensitive file contents are ever sent to AI providers

## Safety architecture

### Command generation

1. User provides natural language request
2. AI (or rule-based fallback) returns a **structured JSON intent** — not shell code
3. Intent is **validated** against a strict schema
4. A typed **command builder** constructs argv from the validated intent
5. The argv is **validated against policy** before being shown to the user
6. User sees the final command and **explicitly confirms** before execution
7. Command is executed using `exec.CommandContext` — **never `sh -c`**

### Blocked operations (enforced in code)

The following are always blocked regardless of user request or AI output:

```
rm, rmdir, mv, cp, chmod, chown, sudo, su, dd, mkfs, truncate, shred
systemctl, service, kill, killall, reboot, shutdown
bash, sh, zsh, eval, exec
>, >>, 2>, | sh, | bash, $(, `, <<
curl | sh, wget | sh
apt, npm, pip, yarn, brew
```

This list is hardcoded in `internal/policy/policy.go` and cannot be overridden by config.

### Scanner safety

The local file scanner never reads:
- `.env` files
- Private key files (`.pem`, `.key`, `.p12`, `.pfx`)
- SSH keys (`id_rsa`, `id_ed25519`, etc.)
- Credential files (`.netrc`, `.htpasswd`, `credentials`)
- Browser data, session stores, cookie files

Even in **deep mode**, only a curated allowlist of safe manifests is read:
`README.md`, `go.mod`, `package.json`, `composer.json`, `docker-compose.yml`, etc.

### Redaction

Before any data is stored in the history database, it is run through a redaction filter that removes:
- API keys and tokens (`api_key=...`, `token=...`)
- AWS access keys (`AKIA...`)
- JWT tokens
- SSH private key material
- Long hex strings (≥32 chars)

### AI permission mode

By default (`ai_permission_mode: local_only`), no data leaves your machine.

When configured to use a remote AI provider (OpenAI, Anthropic), only the following is sent:
- The natural language request text
- Short context snippets from local context files (never raw file contents)
- No file paths containing usernames or secrets

## Reporting a vulnerability

If you discover a security issue, please **do not open a public GitHub issue**.

Email: security@[your-domain] (replace with actual contact)

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix if known

You will receive a response within 48 hours.

## Known limitations

- The `--yes` flag skips the interactive confirmation prompt. It is restricted to `low` and `medium` risk commands. **Do not use `--yes` in automated scripts** without carefully reviewing what it would execute.
- The rule-based parser (fallback when no AI is configured) may produce less accurate commands than AI-assisted parsing. Always review generated commands.
- The output truncation limit (512 KiB by default) prevents memory exhaustion but means you may not see all results from broad searches.
