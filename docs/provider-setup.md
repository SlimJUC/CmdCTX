# AI Provider Setup

`cmdctx` works **without any AI provider** using its built-in rule-based parser. Configure an AI provider to get richer intent parsing, better explanations, and more accurate command generation for ambiguous requests.

---

## Ollama (local, recommended)

Ollama runs AI models locally. No data ever leaves your machine.

**Install Ollama:**
```bash
curl -fsSL https://ollama.ai/install.sh | sh
# or: see https://ollama.ai for other install methods
```

**Pull a model:**
```bash
ollama pull llama3.2          # recommended (4.7GB)
ollama pull mistral            # alternative (4.1GB)
ollama pull codellama          # code-focused alternative
```

**Configure cmdctx:**
```bash
cmdctx providers add \
  --name local \
  --type ollama \
  --model llama3.2

cmdctx providers use local
```

**Custom Ollama endpoint:**
```bash
cmdctx providers add \
  --name remote-ollama \
  --type ollama \
  --url http://192.168.1.100:11434 \
  --model llama3.2
```

---

## OpenAI

```bash
cmdctx providers add \
  --name openai \
  --type openai \
  --key sk-your-api-key \
  --model gpt-4o-mini

cmdctx providers use openai
```

**Recommended models (cheapest to most capable):**
- `gpt-4o-mini` — fast, cheap, good enough for intent parsing
- `gpt-4o` — more accurate for ambiguous requests

**Custom OpenAI-compatible endpoint (e.g., Azure, Together.ai):**
```bash
cmdctx providers add \
  --name azure-openai \
  --type openai \
  --url https://your-resource.openai.azure.com/openai/deployments/your-deployment \
  --key your-azure-api-key \
  --model gpt-4o-mini
```

---

## Anthropic Claude

```bash
cmdctx providers add \
  --name claude \
  --type anthropic \
  --key sk-ant-your-key \
  --model claude-3-5-haiku-20241022

cmdctx providers use claude
```

**Recommended models:**
- `claude-3-5-haiku-20241022` — fast, affordable
- `claude-3-5-sonnet-20241022` — higher accuracy

---

## Managing providers

```bash
cmdctx providers list           # list all configured providers
cmdctx providers use local      # switch active provider
cmdctx providers add ...        # add or update a provider
```

Or edit `~/.cmdctx/config.yaml` directly:
```yaml
active_provider: local
providers:
  - name: local
    type: ollama
    base_url: http://localhost:11434
    model: llama3.2
  - name: openai
    type: openai
    model: gpt-4o-mini
    api_key: sk-...
```

---

## What gets sent to AI providers

When using a **remote** provider (OpenAI, Anthropic):
- Your natural language request text
- Short context snippets (max 5 × 200 chars) from local context files
- The system prompt (fixed schema description)

**Never sent:**
- File contents
- Raw directory listings
- Paths containing usernames
- Any credentials or tokens (redacted before transmission)

To enforce local-only mode even when a remote provider is configured:
```yaml
ai_permission_mode: local_only
```

---

## Troubleshooting providers

```bash
cmdctx doctor   # shows active provider + model
```

**Ollama not responding:**
```bash
ollama serve    # start Ollama service
ollama list     # verify model is installed
```

**"AI returned unparseable response":**
- The model may not support structured JSON output well
- Try a larger model (llama3.2 > llama3.2:1b)
- cmdctx will fall back to rule-based parsing automatically
