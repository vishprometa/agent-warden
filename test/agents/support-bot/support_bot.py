"""Simple support bot agent governed by AgentWarden.

This bot demonstrates governance in action:
- Tool calls are evaluated against policies before execution
- LLM requests are checked for model restrictions and cost limits
- Session lifecycle is tracked (start, actions, end, score)
"""

import os
import sys
import asyncio
import json
from typing import Dict, List, Optional

import httpx


# Simulated knowledge base (in a real bot, this would be a vector DB or search API)
KNOWLEDGE_BASE = {
    "password_reset": {
        "keywords": ["password", "reset", "forgot", "login"],
        "content": "To reset your password, visit https://example.com/reset and enter your email. You'll receive a reset link within 5 minutes.",
    },
    "business_hours": {
        "keywords": ["hours", "open", "when", "time", "available"],
        "content": "We are open Monday-Friday 9am-5pm EST. Weekend support is available via email only.",
    },
    "refund_policy": {
        "keywords": ["refund", "return", "money back", "cancel"],
        "content": "We offer full refunds within 30 days of purchase. Contact support@example.com with your order number.",
    },
    "contact": {
        "keywords": ["contact", "email", "phone", "support"],
        "content": "Email: support@example.com, Phone: 1-800-555-0100, Live Chat: https://example.com/chat",
    },
}


class SupportBot:
    """A simple support bot governed by AgentWarden."""

    def __init__(self, openrouter_api_key: str, warden_client):
        self.openrouter_api_key = openrouter_api_key
        self.warden = warden_client
        self.http_client = httpx.AsyncClient(timeout=30.0)

    async def search_knowledge_base(self, query: str) -> str:
        """Search the knowledge base for relevant articles.

        This is a simulated tool call. In a real bot, this would call
        a vector database, search API, or document retrieval system.
        """
        query_lower = query.lower()
        results = []

        for article_id, article in KNOWLEDGE_BASE.items():
            # Simple keyword matching
            if any(kw in query_lower for kw in article["keywords"]):
                results.append(article["content"])

        if results:
            return "\n\n".join(results)
        return "No relevant articles found in the knowledge base."

    async def call_llm(self, messages: List[Dict[str, str]], model: str = "openai/gpt-4o-mini") -> str:
        """Call OpenRouter LLM API.

        This makes a real API call to OpenRouter. The model and messages
        are sent to the LLM, and the response is returned.
        """
        headers = {
            "Authorization": f"Bearer {self.openrouter_api_key}",
            "Content-Type": "application/json",
        }
        payload = {
            "model": model,
            "messages": messages,
        }

        resp = await self.http_client.post(
            "https://openrouter.ai/api/v1/chat/completions",
            headers=headers,
            json=payload,
        )
        resp.raise_for_status()
        data = resp.json()

        return data["choices"][0]["message"]["content"]

    async def handle_question(self, question: str):
        """Handle a user question with AgentWarden governance.

        This method demonstrates the full governance flow:
        1. Start a session with AgentWarden
        2. Perform governed actions (tool calls, LLM requests)
        3. Handle policy verdicts (allow/deny/approve)
        4. End the session and report outcome
        """
        async with self.warden.session(
            agent_id="support-bot",
            agent_version="v1",
            metadata={"question_length": str(len(question))},
        ) as session:
            try:
                print(f"\nUser: {question}")
                print("-" * 60)

                # Step 1: Search knowledge base (governed tool call)
                print("→ Searching knowledge base...")
                kb_results = await session.tool(
                    name="knowledge_base.search",
                    params={"query": question},
                    execute=lambda: self.search_knowledge_base(question),
                )
                print(f"✓ Found relevant articles:\n{kb_results[:100]}...")

                # Step 2: Call LLM to generate response (governed LLM call)
                print("\n→ Generating response with LLM...")
                messages = [
                    {
                        "role": "system",
                        "content": "You are a helpful support agent. Use the provided knowledge base context to answer user questions concisely.",
                    },
                    {
                        "role": "user",
                        "content": f"Context from knowledge base:\n{kb_results}\n\nUser question: {question}",
                    },
                ]

                response = await session.chat(
                    model="openai/gpt-4o-mini",
                    messages=messages,
                    execute=lambda: self.call_llm(messages),
                )
                print(f"✓ LLM response generated")

                # Step 3: Show final response
                print("-" * 60)
                print(f"Bot: {response}")
                print("-" * 60)

                # Step 4: Score the session
                await session.score(
                    task_completed=True,
                    quality=0.9,
                    metadata={"response_length": len(response)},
                )
                print("\n✓ Session completed successfully")

            except Exception as e:
                print(f"\n✗ Error: {e}")
                await session.score(
                    task_completed=False,
                    quality=0.0,
                    metadata={"error": str(e)},
                )
                raise

    async def close(self):
        """Clean up resources."""
        await self.http_client.aclose()
        await self.warden.close()


async def main():
    """Main entry point for the support bot."""
    if len(sys.argv) < 2:
        print("Usage: python support_bot.py <question>")
        print('Example: python support_bot.py "How do I reset my password?"')
        sys.exit(1)

    question = " ".join(sys.argv[1:])

    # Load config from environment
    openrouter_api_key = os.getenv("OPENROUTER_API_KEY")
    if not openrouter_api_key:
        print("Error: OPENROUTER_API_KEY environment variable not set")
        print("Get your API key from https://openrouter.ai/")
        sys.exit(1)

    # Import AgentWarden SDK
    try:
        # Try to import from installed package
        from agentwarden import AgentWarden, ActionDenied, ActionPendingApproval
    except ImportError:
        print("Error: AgentWarden Python SDK not installed")
        print("Install it with: cd ../../../sdks/python && pip install -e .")
        sys.exit(1)

    # Create bot and handle question
    warden = AgentWarden()
    bot = SupportBot(openrouter_api_key, warden)

    try:
        await bot.handle_question(question)
    except ActionDenied as e:
        print(f"\n✗ Action denied by policy: {e.policy_name}")
        print(f"  Message: {e.message}")
        if e.suggestions:
            print(f"  Suggestions: {', '.join(e.suggestions)}")
        sys.exit(1)
    except ActionPendingApproval as e:
        print(f"\n⏸ Action requires approval: {e.policy_name}")
        print(f"  Approval ID: {e.approval_id}")
        print(f"  Timeout: {e.timeout_seconds}s")
        sys.exit(2)
    except Exception as e:
        print(f"\n✗ Unexpected error: {e}")
        sys.exit(1)
    finally:
        await bot.close()


if __name__ == "__main__":
    asyncio.run(main())
