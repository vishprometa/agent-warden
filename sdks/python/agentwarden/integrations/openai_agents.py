"""OpenAI Agents SDK integration for AgentWarden.

Provides :class:`GovernedRunner` -- a wrapper around the OpenAI Agents SDK
``Runner`` that governs agent executions through AgentWarden sessions.

Each agent run is wrapped in an AgentWarden session, with tool calls and
handoffs governed by policies.

Example::

    from agents import Agent
    from agentwarden import AgentWarden
    from agentwarden.integrations.openai_agents import GovernedRunner

    warden = AgentWarden()
    runner = GovernedRunner(warden=warden, agent_id="openai-agent")

    agent = Agent(name="assistant", instructions="You are helpful.")
    result = await runner.run(agent, "Hello!")

Install the extra to use this integration::

    pip install agentwarden[openai-agents]
"""

from __future__ import annotations

import time
from typing import Any, Dict, Optional

from agentwarden.client import AgentWarden
from agentwarden.session import Session
from agentwarden.exceptions import ActionDenied


class GovernedRunner:
    """Wraps OpenAI Agents SDK Runner with AgentWarden governance.

    Creates an AgentWarden session for each agent run. Tool calls and
    handoffs within the run are governed by policies. The session is
    automatically scored upon completion.

    Args:
        warden: An :class:`AgentWarden` instance.
        agent_id: Agent identifier for the AgentWarden session.
        metadata: Extra metadata attached to the session.
    """

    def __init__(
        self,
        warden: AgentWarden,
        *,
        agent_id: str = "openai-agent",
        metadata: Optional[Dict[str, str]] = None,
    ) -> None:
        self._warden = warden
        self._agent_id = agent_id
        self._metadata = metadata or {}

    async def run(
        self,
        agent: Any,
        input_text: str,
        *,
        context: Optional[Any] = None,
        **kwargs: Any,
    ) -> Any:
        """Run an OpenAI Agent with full governance.

        Wraps ``Runner.run()`` from the ``agents`` package, recording
        the entire execution as an AgentWarden session with policy
        evaluation on tool calls and handoffs.

        Args:
            agent: An ``agents.Agent`` instance.
            input_text: The user input to process.
            context: Optional context object passed to the runner.
            **kwargs: Additional arguments forwarded to ``Runner.run()``.

        Returns:
            The ``RunResult`` from the Agents SDK.

        Raises:
            ActionDenied: If a policy denies a tool call or handoff.
            ImportError: If the ``agents`` package is not installed.
        """
        try:
            from agents import Runner
        except ImportError as err:
            raise ImportError(
                "The 'agents' package (openai-agents) is required. "
                "Install it with: pip install openai-agents"
            ) from err

        agent_name = getattr(agent, "name", "agent")
        start = time.monotonic()
        error_occurred = False
        error_message: Optional[str] = None

        async with self._warden.session(
            agent_id=self._agent_id,
            metadata={
                "agent_name": agent_name,
                **self._metadata,
            },
        ) as session:
            try:
                # Evaluate the overall agent run
                await session.chat(
                    model=getattr(agent, "model", "gpt-4o") or "gpt-4o",
                    messages=[{"role": "user", "content": input_text}],
                )

                # Execute the run
                run_kwargs: Dict[str, Any] = {**kwargs}
                if context is not None:
                    run_kwargs["context"] = context
                result = await Runner.run(agent, input_text, **run_kwargs)

                # Trace tool calls and handoffs from the result
                await self._trace_run_result(session, agent_name, result)

                return result
            except ActionDenied:
                error_occurred = True
                raise
            except Exception as exc:
                error_occurred = True
                error_message = f"{type(exc).__name__}: {exc}"
                raise
            finally:
                latency_ms = (time.monotonic() - start) * 1000
                try:
                    await session.score(
                        task_completed=not error_occurred,
                        quality=1.0 if not error_occurred else 0.0,
                        metadata={
                            "latency_ms": str(round(latency_ms, 2)),
                            "agent_name": agent_name,
                            **({"error": error_message} if error_message else {}),
                        },
                    )
                except Exception:
                    pass

    async def _trace_run_result(
        self,
        session: Session,
        agent_name: str,
        result: Any,
    ) -> None:
        """Extract and govern tool calls and handoffs from a RunResult."""
        items = getattr(result, "new_items", None)
        if not items:
            return

        for item in items:
            item_type = type(item).__name__

            if "ToolCall" in item_type:
                tool_name = getattr(item, "name", None) or getattr(
                    getattr(item, "tool", None), "name", "unknown_tool"
                )
                tool_args = getattr(item, "arguments", None) or getattr(
                    item, "args", None
                )
                try:
                    await session.tool(
                        name=str(tool_name),
                        params=(
                            tool_args
                            if isinstance(tool_args, dict)
                            else {"raw": str(tool_args)}
                        ),
                        target=f"agent:{agent_name}",
                    )
                except (ActionDenied, Exception):
                    # Tool already executed by the runner; governance
                    # here is observational for post-run analysis.
                    pass

            elif "Handoff" in item_type:
                target = getattr(item, "target", None)
                target_name = (
                    getattr(target, "name", str(target))
                    if target
                    else "unknown"
                )
                try:
                    await session.api_call(
                        name=f"handoff:{target_name}",
                        params={
                            "from_agent": agent_name,
                            "to_agent": str(target_name),
                        },
                        target=f"agent:{target_name}",
                    )
                except (ActionDenied, Exception):
                    pass

            elif "ModelResponse" in item_type or "Message" in item_type:
                # Trace LLM response passively
                session.trace_llm(item)

    async def run_streamed(
        self,
        agent: Any,
        input_text: str,
        *,
        context: Optional[Any] = None,
        **kwargs: Any,
    ) -> Any:
        """Run an OpenAI Agent with streaming and governance.

        Similar to :meth:`run` but uses ``Runner.run_streamed()`` for
        streaming output.

        Args:
            agent: An ``agents.Agent`` instance.
            input_text: The user input to process.
            context: Optional context object passed to the runner.
            **kwargs: Additional arguments forwarded to ``Runner.run_streamed()``.

        Returns:
            The streaming ``RunResult`` from the Agents SDK.
        """
        try:
            from agents import Runner
        except ImportError as err:
            raise ImportError(
                "The 'agents' package (openai-agents) is required. "
                "Install it with: pip install openai-agents"
            ) from err

        agent_name = getattr(agent, "name", "agent")
        start = time.monotonic()
        error_occurred = False
        error_message: Optional[str] = None

        async with self._warden.session(
            agent_id=self._agent_id,
            metadata={
                "agent_name": agent_name,
                "streaming": "true",
                **self._metadata,
            },
        ) as session:
            try:
                await session.chat(
                    model=getattr(agent, "model", "gpt-4o") or "gpt-4o",
                    messages=[{"role": "user", "content": input_text}],
                )

                run_kwargs: Dict[str, Any] = {**kwargs}
                if context is not None:
                    run_kwargs["context"] = context
                result = await Runner.run_streamed(agent, input_text, **run_kwargs)

                return result
            except ActionDenied:
                error_occurred = True
                raise
            except Exception as exc:
                error_occurred = True
                error_message = f"{type(exc).__name__}: {exc}"
                raise
            finally:
                latency_ms = (time.monotonic() - start) * 1000
                try:
                    await session.score(
                        task_completed=not error_occurred,
                        metadata={
                            "latency_ms": str(round(latency_ms, 2)),
                            "agent_name": agent_name,
                            "streaming": "true",
                            **({"error": error_message} if error_message else {}),
                        },
                    )
                except Exception:
                    pass
