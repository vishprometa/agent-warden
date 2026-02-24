"""LangChain integration for AgentWarden v2.

Provides :class:`AgentWardenCallbackHandler` -- a LangChain async callback
handler that traces LLM calls and tool calls through AgentWarden governance
sessions.

Unlike the v1 integration which only traced passively, the v2 handler
creates a governed session and records actions with full policy evaluation
context.

Example::

    from langchain_openai import ChatOpenAI
    from agentwarden import AgentWarden
    from agentwarden.integrations.langchain import AgentWardenCallbackHandler

    warden = AgentWarden()
    handler = AgentWardenCallbackHandler(warden=warden, agent_id="langchain-bot")

    llm = ChatOpenAI(model="gpt-4o", callbacks=[handler])
    llm.invoke("Hello, world!")

Install the extra to use this integration::

    pip install agentwarden[langchain]
"""

from __future__ import annotations

import time
import uuid
from typing import Any, Dict, List, Optional

try:
    from langchain_core.callbacks import AsyncCallbackHandler
    from langchain_core.outputs import LLMResult
    from langchain_core.agents import AgentAction, AgentFinish
    from langchain_core.messages import BaseMessage
except ImportError as _err:
    raise ImportError(
        "langchain-core is required for the LangChain integration. "
        "Install it with: pip install agentwarden[langchain]"
    ) from _err

from agentwarden.client import AgentWarden
from agentwarden.session import Session


class AgentWardenCallbackHandler(AsyncCallbackHandler):
    """LangChain async callback handler that traces actions through AgentWarden.

    Records LLM calls (start/end with latency and token usage), tool calls,
    and chain-level events. Each chain run is mapped to an AgentWarden session.

    This handler uses the v2 session-based governance pattern. A session is
    created automatically on the first event and ended when the chain completes.

    Args:
        warden:   An :class:`AgentWarden` instance.
        agent_id: Agent identifier for the session.
        metadata: Extra metadata attached to the session.
    """

    def __init__(
        self,
        warden: AgentWarden,
        *,
        agent_id: str = "langchain-agent",
        metadata: Optional[Dict[str, str]] = None,
    ) -> None:
        super().__init__()
        self._warden = warden
        self._agent_id = agent_id
        self._metadata = metadata or {}
        self._session: Optional[Session] = None
        self._run_timings: Dict[str, float] = {}

    async def _ensure_session(self) -> Session:
        """Lazily create a session on first event."""
        if self._session is None:
            self._session = Session(
                client=self._warden,
                agent_id=self._agent_id,
                agent_version="v1",
                metadata=self._metadata,
            )
            await self._session.start()
        return self._session

    async def _end_session(self) -> None:
        """End the session if one was started."""
        if self._session is not None:
            try:
                await self._session.end()
            except Exception:
                pass
            self._session = None

    # -- LLM callbacks ------------------------------------------------------

    async def on_llm_start(
        self,
        serialized: Dict[str, Any],
        prompts: List[str],
        *,
        run_id: Any = None,
        **kwargs: Any,
    ) -> None:
        """Record the start time of an LLM call."""
        rid = str(run_id) if run_id else uuid.uuid4().hex
        self._run_timings[rid] = time.monotonic()

    async def on_chat_model_start(
        self,
        serialized: Dict[str, Any],
        messages: List[List[BaseMessage]],
        *,
        run_id: Any = None,
        **kwargs: Any,
    ) -> None:
        """Record the start time of a chat model call."""
        rid = str(run_id) if run_id else uuid.uuid4().hex
        self._run_timings[rid] = time.monotonic()

    async def on_llm_end(
        self,
        response: LLMResult,
        *,
        run_id: Any = None,
        **kwargs: Any,
    ) -> None:
        """Record a completed LLM call with latency and token usage."""
        rid = str(run_id) if run_id else ""
        start = self._run_timings.pop(rid, None)
        latency_ms = int((time.monotonic() - start) * 1000) if start else 0

        tokens_in = 0
        tokens_out = 0
        model_name = ""
        if response.llm_output and isinstance(response.llm_output, dict):
            usage = response.llm_output.get("token_usage", {})
            tokens_in = usage.get("prompt_tokens", 0)
            tokens_out = usage.get("completion_tokens", 0)
            model_name = response.llm_output.get("model_name", "")

        session = await self._ensure_session()
        session.trace_llm(response, model=model_name)

    async def on_llm_error(
        self,
        error: BaseException,
        *,
        run_id: Any = None,
        **kwargs: Any,
    ) -> None:
        """Record a failed LLM call."""
        rid = str(run_id) if run_id else ""
        self._run_timings.pop(rid, None)

    # -- Tool callbacks -----------------------------------------------------

    async def on_tool_start(
        self,
        serialized: Dict[str, Any],
        input_str: str,
        *,
        run_id: Any = None,
        **kwargs: Any,
    ) -> None:
        """Record the start time of a tool invocation."""
        rid = str(run_id) if run_id else uuid.uuid4().hex
        self._run_timings[rid] = time.monotonic()

    async def on_tool_end(
        self,
        output: str,
        *,
        run_id: Any = None,
        name: Optional[str] = None,
        **kwargs: Any,
    ) -> None:
        """Record a completed tool call via governed session."""
        rid = str(run_id) if run_id else ""
        start = self._run_timings.pop(rid, None)
        latency_ms = int((time.monotonic() - start) * 1000) if start else 0

        session = await self._ensure_session()
        try:
            await session.tool(
                name=name or "langchain_tool",
                params={"output_preview": str(output)[:500] if output else ""},
            )
        except Exception:
            # Never let tracing failures break the chain.
            pass

    async def on_tool_error(
        self,
        error: BaseException,
        *,
        run_id: Any = None,
        name: Optional[str] = None,
        **kwargs: Any,
    ) -> None:
        """Record a failed tool call."""
        rid = str(run_id) if run_id else ""
        self._run_timings.pop(rid, None)

    # -- Chain callbacks ----------------------------------------------------

    async def on_chain_start(
        self,
        serialized: Dict[str, Any],
        inputs: Dict[str, Any],
        *,
        run_id: Any = None,
        **kwargs: Any,
    ) -> None:
        """Called when a chain begins. Ensures session is started."""
        rid = str(run_id) if run_id else uuid.uuid4().hex
        self._run_timings[rid] = time.monotonic()
        await self._ensure_session()

    async def on_chain_end(
        self,
        outputs: Dict[str, Any],
        *,
        run_id: Any = None,
        **kwargs: Any,
    ) -> None:
        """Called when a chain ends. Ends the session."""
        rid = str(run_id) if run_id else ""
        self._run_timings.pop(rid, None)
        await self._end_session()

    async def on_chain_error(
        self,
        error: BaseException,
        *,
        run_id: Any = None,
        **kwargs: Any,
    ) -> None:
        """Called on chain error. Ends the session."""
        rid = str(run_id) if run_id else ""
        self._run_timings.pop(rid, None)
        await self._end_session()

    # -- Agent callbacks ----------------------------------------------------

    async def on_agent_action(
        self,
        action: AgentAction,
        *,
        run_id: Any = None,
        **kwargs: Any,
    ) -> None:
        """Record an agent action decision as a governed tool call."""
        session = await self._ensure_session()
        try:
            await session.tool(
                name=action.tool,
                params=(
                    action.tool_input
                    if isinstance(action.tool_input, dict)
                    else {"input": str(action.tool_input)}
                ),
            )
        except Exception:
            pass

    async def on_agent_finish(
        self,
        finish: AgentFinish,
        *,
        run_id: Any = None,
        **kwargs: Any,
    ) -> None:
        """Record the final agent output and score the session."""
        session = await self._ensure_session()
        try:
            await session.score(task_completed=True)
        except Exception:
            pass
