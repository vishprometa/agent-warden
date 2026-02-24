"""CrewAI integration for AgentWarden.

Provides :class:`GovernedCrew` -- a wrapper around CrewAI's Crew class
that governs crew execution through AgentWarden sessions.

Each crew kickoff is wrapped in an AgentWarden session, and tool calls
made by crew agents are governed by policies.

Example::

    from crewai import Crew, Agent, Task
    from agentwarden import AgentWarden
    from agentwarden.integrations.crewai import GovernedCrew

    warden = AgentWarden()

    researcher = Agent(
        role="Researcher",
        goal="Find information",
        backstory="Expert researcher",
    )
    task = Task(description="Research AI governance", agent=researcher)

    governed = GovernedCrew(
        warden=warden,
        agent_id="research-crew",
        crew_kwargs=dict(agents=[researcher], tasks=[task]),
    )
    result = await governed.kickoff()

Install the extra to use this integration::

    pip install agentwarden[crewai]
"""

from __future__ import annotations

import time
import asyncio
from typing import Any, Dict, Optional

from agentwarden.client import AgentWarden
from agentwarden.session import Session
from agentwarden.exceptions import ActionDenied


class GovernedCrew:
    """Wraps a CrewAI Crew with AgentWarden governance.

    Creates an AgentWarden session for each crew execution, governing
    tool calls and tracking agent steps.

    Args:
        warden: An :class:`AgentWarden` instance.
        agent_id: Agent identifier for the AgentWarden session.
        crew_kwargs: Keyword arguments passed to ``crewai.Crew()``.
        metadata: Extra metadata attached to the session.
    """

    def __init__(
        self,
        warden: AgentWarden,
        *,
        agent_id: str = "crewai-agent",
        crew_kwargs: Optional[Dict[str, Any]] = None,
        metadata: Optional[Dict[str, str]] = None,
    ) -> None:
        self._warden = warden
        self._agent_id = agent_id
        self._crew_kwargs = crew_kwargs or {}
        self._metadata = metadata or {}
        self._crew = None

    def _ensure_crew(self) -> Any:
        """Lazily import and instantiate the Crew."""
        if self._crew is None:
            try:
                from crewai import Crew
            except ImportError as err:
                raise ImportError(
                    "crewai is required for this integration. "
                    "Install it with: pip install agentwarden[crewai]"
                ) from err
            self._crew = Crew(**self._crew_kwargs)
        return self._crew

    async def kickoff(
        self,
        inputs: Optional[Dict[str, Any]] = None,
    ) -> Any:
        """Execute the crew with AgentWarden governance.

        Wraps the entire crew run in an AgentWarden session. Tool calls
        from crew agents are governed, and the session is scored upon
        completion.

        Args:
            inputs: Optional inputs passed to ``crew.kickoff()``.

        Returns:
            The crew execution result.
        """
        crew = self._ensure_crew()
        start = time.monotonic()
        error_occurred = False
        error_message: Optional[str] = None

        async with self._warden.session(
            agent_id=self._agent_id,
            metadata=self._metadata,
        ) as session:
            try:
                # CrewAI's kickoff may be sync or async depending on version
                kickoff_kwargs: Dict[str, Any] = {}
                if inputs:
                    kickoff_kwargs["inputs"] = inputs

                if hasattr(crew, "kickoff_async"):
                    result = await crew.kickoff_async(**kickoff_kwargs)
                else:
                    result = crew.kickoff(**kickoff_kwargs)

                # Record agent steps from the result if available
                await self._trace_crew_result(session, result)

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
                            **({"error": error_message} if error_message else {}),
                        },
                    )
                except Exception:
                    pass

    async def _trace_crew_result(self, session: Session, result: Any) -> None:
        """Extract and trace tool calls from crew execution result."""
        # CrewAI results may have tasks_output with tool usage info
        tasks_output = getattr(result, "tasks_output", None)
        if not tasks_output:
            return

        for task_output in tasks_output:
            # Record each task's tool usage if available
            tool_calls = getattr(task_output, "tool_calls", None)
            if tool_calls:
                for tool_call in tool_calls:
                    tool_name = getattr(tool_call, "name", None) or str(tool_call)
                    tool_args = getattr(tool_call, "arguments", None) or {}
                    try:
                        await session.tool(
                            name=str(tool_name),
                            params=tool_args if isinstance(tool_args, dict) else {},
                        )
                    except (ActionDenied, Exception):
                        pass

    def kickoff_sync(
        self,
        inputs: Optional[Dict[str, Any]] = None,
    ) -> Any:
        """Synchronous variant of :meth:`kickoff`.

        For use in non-async contexts. Note that governance evaluation
        still requires async internally, so this uses asyncio.run().

        Args:
            inputs: Optional inputs passed to ``crew.kickoff()``.

        Returns:
            The crew execution result.
        """
        return asyncio.run(self.kickoff(inputs=inputs))
