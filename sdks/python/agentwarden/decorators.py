"""Decorator API for governed functions.

The ``@governed`` decorator wraps an async function with automatic
AgentWarden session lifecycle management. The decorated function
receives a ``session`` keyword argument.

Example::

    from agentwarden import governed

    @governed(agent_id="support-bot")
    async def handle_ticket(ticket_id: str, session):
        response = await session.chat(
            model="gpt-4o",
            messages=[{"role": "user", "content": f"Handle ticket {ticket_id}"}],
            execute=lambda: call_llm(ticket_id),
        )
        await session.tool(
            name="zendesk.reply",
            params={"ticket_id": ticket_id, "message": response},
            execute=lambda: zendesk.reply(ticket_id, response),
        )
        await session.score(task_completed=True)
"""

import functools
from typing import Any, Callable, Dict, Optional

from .client import AgentWarden

_default_client: Optional[AgentWarden] = None


def get_default_client() -> AgentWarden:
    """Return or create a default AgentWarden client.

    The default client auto-discovers AgentWarden on localhost:6777.
    """
    global _default_client
    if _default_client is None:
        _default_client = AgentWarden()
    return _default_client


def governed(
    agent_id: str,
    client: AgentWarden = None,
    metadata: dict = None,
) -> Callable:
    """Decorator that wraps an async function with AgentWarden governance.

    The decorated function receives a ``session`` keyword argument which
    is an active :class:`Session` instance. The session is automatically
    started before the function runs and ended after it returns (or raises).

    Args:
        agent_id: Identifier for the agent being governed.
        client: Optional AgentWarden client instance. If None, a default
                client is created that connects to localhost:6777.
        metadata: Optional key-value metadata attached to the session.

    Returns:
        A decorator that wraps the target async function.

    Example::

        @governed(agent_id="support-bot")
        async def handle_ticket(ticket_id: str, session):
            await session.tool(name="zendesk.reply", ...)
            await session.score(task_completed=True)

        # Call it normally -- session is injected automatically
        await handle_ticket("TICKET-123")
    """
    def decorator(func: Callable) -> Callable:
        @functools.wraps(func)
        async def wrapper(*args: Any, **kwargs: Any) -> Any:
            warden = client or get_default_client()
            async with warden.session(
                agent_id=agent_id,
                metadata=metadata or {},
            ) as session:
                return await func(*args, session=session, **kwargs)
        return wrapper
    return decorator
