# Running the Support Bot Test

This guide shows how to run the support bot agent with AgentWarden governance.

## Prerequisites

1. **Build AgentWarden**:
   ```bash
   cd /Users/vish/Developer/agentwarden
   go build -o agentwarden ./cmd/agentwarden
   ```

2. **Install Python SDK**:
   ```bash
   cd sdks/python
   pip install -e .
   ```

3. **Install Support Bot Dependencies**:
   ```bash
   cd test/agents/support-bot
   pip install -r requirements.txt
   ```

4. **Set Environment Variables**:
   ```bash
   export OPENROUTER_API_KEY="your-openrouter-api-key"
   export AGENTWARDEN_HOST="localhost"
   export AGENTWARDEN_PORT="6777"
   ```

## Running the Test

### Step 1: Start AgentWarden Server

In one terminal:

```bash
cd test/config
../../agentwarden start -c agentwarden.yaml
```

You should see:
```
AgentWarden starting...
✓ Loaded config from agentwarden.yaml
✓ Connected to SQLite database (test-agentwarden.db)
✓ HTTP server listening on :6777
✓ gRPC server listening on :6778
✓ Dashboard available at http://localhost:6777
```

### Step 2: Run the Support Bot

In another terminal:

```bash
cd test/agents/support-bot
python support_bot.py "How do I reset my password?"
```

Expected output:
```
User: How do I reset my password?
------------------------------------------------------------
→ Searching knowledge base...
✓ Found relevant articles:
To reset your password, visit https://example.com/reset and enter your email...

→ Generating response with LLM...
✓ LLM response generated
------------------------------------------------------------
Bot: To reset your password, visit https://example.com/reset and enter your email. You'll receive a reset link within 5 minutes.
------------------------------------------------------------

✓ Session completed successfully
```

### Step 3: View the Dashboard

Open http://localhost:6777 in your browser to see:
- Session trace (start → tool call → LLM call → score → end)
- Cost tracking (session total)
- Policy evaluations (all actions evaluated against policies)
- Detection alerts (if any loops/spirals detected)

## Testing Different Policies

### Test 1: Normal Operation (Should Succeed)
```bash
python support_bot.py "What are your business hours?"
```
Expected: ✓ Success, response generated, cost ~$0.001

### Test 2: Cost Limit Policy
The support bot's single question flow shouldn't hit the $0.50 cost limit, but you can verify the policy is configured by checking the dashboard's "Policies" tab.

### Test 3: Loop Detection
Run the same question multiple times rapidly:
```bash
for i in {1..10}; do
  python support_bot.py "Hello" &
done
wait
```
Expected: After 5 identical `knowledge_base.search` calls, you should see a loop detection alert in the dashboard.

### Test 4: Model Restriction Policy
To test model restrictions, you would need to modify the support_bot.py to try using a different model (e.g., `openai/gpt-4`). The policy should deny the action.

### Test 5: Rate Limiting
Run many questions in quick succession:
```bash
for i in {1..20}; do
  python support_bot.py "Question $i" &
done
wait
```
Expected: Some requests may be throttled (delayed by 2s) due to the 30 LLM calls/minute limit.

## Cleanup

1. Stop the AgentWarden server (Ctrl+C)
2. Remove the test database:
   ```bash
   rm test/config/test-agentwarden.db
   ```

## What to Verify

- [ ] AgentWarden server starts successfully
- [ ] Support bot connects and creates a session
- [ ] Tool calls are evaluated and allowed
- [ ] LLM calls are evaluated and allowed
- [ ] Session cost is tracked correctly
- [ ] Session score is recorded
- [ ] Dashboard shows complete trace
- [ ] Loop detection triggers after threshold
- [ ] Policies are evaluated for each action

## Troubleshooting

**"Error: AgentWarden Python SDK not installed"**
- Run: `cd sdks/python && pip install -e .`

**"Error: OPENROUTER_API_KEY environment variable not set"**
- Get an API key from https://openrouter.ai/
- Export it: `export OPENROUTER_API_KEY="sk-..."`

**"Connection refused" errors**
- Make sure AgentWarden server is running
- Check the port matches (6777 for HTTP, 6778 for gRPC)
- Verify with: `curl http://localhost:6777/health`

**"No module named 'httpx'"**
- Install dependencies: `cd test/agents/support-bot && pip install -r requirements.txt`
