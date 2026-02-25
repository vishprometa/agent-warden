# Support Bot Test Agent

A simple test agent that demonstrates AgentWarden governance in action.

## What It Does

The support bot:
1. Takes a user question as input
2. Searches a simulated knowledge base for relevant context
3. Calls an LLM (via OpenRouter) to generate a response
4. Returns the answer to the user

All actions (tool calls and LLM requests) are governed by AgentWarden policies.

## Setup

1. Install dependencies:
   ```bash
   pip install -r requirements.txt
   ```

2. Install the AgentWarden Python SDK:
   ```bash
   cd ../../../sdks/python
   pip install -e .
   ```

3. Set environment variables:
   ```bash
   export OPENROUTER_API_KEY="your-openrouter-api-key"
   export AGENTWARDEN_HOST="localhost"
   export AGENTWARDEN_PORT="6777"
   ```

## Usage

**IMPORTANT**: You must start the AgentWarden server first. See `../../RUN_SUPPORT_BOT_TEST.md` for complete setup instructions.

Quick start:
```bash
# Terminal 1: Start AgentWarden
cd ../../config
../../../agentwarden start -c agentwarden.yaml

# Terminal 2: Run the bot
cd ../agents/support-bot
python support_bot.py "How do I reset my password?"
```

## Governance

The bot is governed by AgentWarden policies configured in `../../config/agentwarden.yaml`:
- **Cost limit**: Deny if session cost > $0.50
- **Daily budget**: Terminate if agent's daily cost > $5.00
- **Loop detection**: Alert if same tool called > 5 times in 60 seconds
- **Model restrictions**: Only allow `openai/gpt-4o-mini` model
- **Rate limiting**: Max 30 LLM calls per minute, max 50 tool calls per minute

Agent configuration files are in `../support-bot/v1/`:
- `AGENT.md`: Agent capabilities, risk profile, recommendations
- `EVOLVE.md`: Evolution strategy, success metrics, constraints
- `PROMPT.md`: System prompt template for the LLM

## Testing Policies

Try these commands to trigger different policy behaviors:

```bash
# Normal operation (should succeed)
python support_bot.py "What are your business hours?"

# Test loop detection (run multiple times quickly)
for i in {1..10}; do python support_bot.py "Hello"; done

# Test cost limits (requires actual LLM calls)
python support_bot.py "Write a very long detailed guide about everything"
```
