# Support Bot Agent

A customer support agent that helps users with common questions by searching a knowledge base and generating responses using an LLM.

## Purpose

The support bot provides automated customer support by:
1. Searching a simulated knowledge base for relevant context
2. Using an LLM to generate natural, helpful responses
3. Tracking session outcomes (completion rate, quality scores)

## Capabilities

### Tools
- `knowledge_base.search`: Search the knowledge base for relevant articles
  - Input: User question (string)
  - Output: Relevant article content or "No relevant articles found"
  - Cost: Free (no external API calls)

### LLM Models
- `openai/gpt-4o-mini` (via OpenRouter): Generate responses based on knowledge base context
  - Cost: ~$0.15 per 1M input tokens, ~$0.60 per 1M output tokens
  - Typical session cost: $0.001 - $0.01 per conversation

## Session Flow

1. **Start session**: Agent receives a user question
2. **Search KB**: Call `knowledge_base.search` with the question
3. **Generate response**: Call LLM with KB context + user question
4. **Score outcome**: Report completion (true/false) and quality (0-1)
5. **End session**: Clean up resources

## Metrics

- **Task completion rate**: % of sessions that successfully answer the user question
- **Quality score**: Average quality rating (0-1) across all sessions
- **Cost per task**: Average cost per completed conversation
- **Average latency**: Time from question to final response

## Risk Profile

- **Low risk**: Read-only operations (no writes, no external actions beyond API calls)
- **Moderate cost**: LLM calls can accumulate cost if many questions are asked
- **No data exfiltration**: Only accesses internal knowledge base + OpenRouter API

## Governance Recommendations

1. **Cost limit**: Set per-session budget ($0.50 recommended) to prevent runaway costs
2. **Loop detection**: Alert if same tool called >5 times (may indicate stuck behavior)
3. **Model restrictions**: Only allow specific, pre-approved models (gpt-4o-mini)
4. **Rate limiting**: Limit API calls per minute to avoid billing spikes
