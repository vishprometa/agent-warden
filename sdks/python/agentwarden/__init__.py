"""AgentWarden Python SDK v2 -- runtime governance for AI agents.

Session-based governance with HTTP transport (gRPC optional)::

    from agentwarden import AgentWarden

    warden = AgentWarden()

    async with warden.session(agent_id="support-bot") as session:
        response = await session.chat(
            model="gpt-4o",
            messages=[{"role": "user", "content": "Hello"}],
            execute=lambda: openai_client.chat.completions.create(...),
        )
        await session.tool(
            name="zendesk.reply",
            params={"ticket_id": 123, "message": response},
            execute=lambda: zendesk.reply(123, response),
        )
        await session.score(task_completed=True, quality=0.95)

Decorator for automatic session management::

    from agentwarden import governed

    @governed(agent_id="support-bot")
    async def handle_ticket(ticket_id: str, session):
        await session.tool(name="zendesk.reply", ...)

Framework integrations::

    from agentwarden.integrations.langchain import AgentWardenCallbackHandler
    from agentwarden.integrations.crewai import GovernedCrew
    from agentwarden.integrations.openai_agents import GovernedRunner
"""

from agentwarden.client import AgentWarden
from agentwarden.session import Session
from agentwarden.decorators import governed
from agentwarden.exceptions import ActionDenied, ActionPendingApproval

__all__ = [
    "AgentWarden",
    "Session",
    "governed",
    "ActionDenied",
    "ActionPendingApproval",
]

__version__ = "0.2.0"
