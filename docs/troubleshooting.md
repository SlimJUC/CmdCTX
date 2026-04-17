# Troubleshooting

## Diagnosis

Always start with:

```bash
cmdctx doctor
```

This checks config files, tools, PATH, and AI provider configuration.

---

## Common issues

### "command not found: cmdctx"

The binary is not in your PATH.

```bash
# Option 1: Add install dir to PATH
export PATH="$HOME/.local/bin:$PATH"
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc  # or ~/.bashrc

# Option 2: Install globally
make install-global

# Option 3: Run directly
./bin/cmdctx doctor
```

### "no AI provider configured"

cmdctx still works — it uses rule-based parsing as fallback. To get better results:

```bash
# Ollama (local, no internet required)
ollama pull llama3.2
cmdctx providers add --name local --type ollama --model llama3.2

# Or configure an OpenAI key
cmdctx providers add --name openai --type openai --key sk-... --model gpt-4o-mini
```

### "ollama request failed (is Ollama running?)"

Start the Ollama service:

```bash
ollama serve
# or on systemd:
systemctl start ollama
```

Verify Ollama is working:
```bash
curl http://localhost:11434/api/tags
ollama list
```

### "AI returned unparseable response"

The model didn't return valid JSON. cmdctx fell back to rule-based parsing.

Possible causes:
- Model too small (e.g., 1B parameter models often fail at structured JSON)
- Model not instruction-tuned

Try a larger model:
```bash
ollama pull llama3.2  # 3B — recommended minimum
ollama pull mistral   # alternative
cmdctx providers add --name local --type ollama --model llama3.2
```

### "command blocked by policy: ..."

The generated command was blocked because it contains a forbidden operation. This is working as intended.

If you believe the request is legitimate:
- Rephrase to be more specific about what you want to *search for*
- cmdctx is designed for read-only search/investigation, not for mutations

### "intent is 'unknown' — please clarify your request"

The parser couldn't determine what you want. Try:
- More specific phrasing
- Include a target: "in this project", "in nginx logs", "in php files"
- Use `cmdctx ask` to see the full error

### Scan taking too long / machine context is huge

The default scan visits up to 50,000 files. To reduce scope:

```bash
cmdctx init --mode safe   # safe mode is faster (no file content reading)
```

Or edit `~/.cmdctx/config.yaml` to add more default excludes:
```yaml
default_excludes:
  - .git
  - node_modules
  - vendor
  - dist
  - .cache
  - your-large-directory
```

### Machine context file is outdated

```bash
cmdctx refresh
```

### History database is corrupted

```bash
rm ~/.cmdctx/history.db
cmdctx init   # recreates the database
```

### "output truncated" message

The output exceeded the 512 KiB limit. To see more output:
1. Edit `~/.cmdctx/config.yaml`:
   ```yaml
   output_max_bytes: 2097152  # 2 MiB
   ```
2. Or run the generated command directly in your terminal (copy it from the result)

---

## Debug logging

Enable debug logging:
```bash
CMDCTX_LOG_LEVEL=debug cmdctx ask "find php files"
```

Or in config:
```yaml
log_level: debug
```

---

## Reset everything

```bash
make uninstall-all    # removes binary + all data

# Or manually:
rm -rf ~/.cmdctx
rm ~/.local/bin/cmdctx
```
