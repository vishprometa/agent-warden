"""Session -- a governed agent execution context.

A Session represents a single governed execution of an agent. All tool calls,
API requests, database queries, and LLM invocations within a session are
evaluated against AgentWarden policies before execution.
"""

import json
import uuid
import time
import inspect
from typing import Any, Callable, Dict, Optional, TypeVar

from .exceptions import ActionDenied, ActionPendingApproval

T = TypeVar("T")


class Session:
    """Active governed session for an agent.

    Created via :meth:`AgentWarden.session` context manager. Provides
    typed methods for common agent actions (tool calls, API requests,
    database queries, LLM chat) that are evaluated against policies
    before execution.

    Each action follows the pattern:
    1. Send action details to AgentWarden for policy evaluation
    2. If allowed, execute the provided callable
    3. If denied/pending, raise the appropriate exception
    """

    def __init__(
        self,
        client: Any,
        agent_id: str,
        agent_version: str,
        metadata: Dict[str, str],
    ):
        self._client = client
        self.session_id = f"ses_{uuid.uuid4().hex[:8]}"
        self.agent_id = agent_id
        self.agent_version = agent_version
        self.metadata = metadata
        self._cost = 0.0
        self._action_count = 0
        self._started_at: Optional[float] = None

    async def start(self):
        """Register the session with AgentWarden."""
        self._started_at = time.time()
        await self._client._post("/v1/sessions/start", {
            "session_id": self.session_id,
            "agent_id": self.agent_id,
            "agent_version": self.agent_version,
            "metadata": self.metadata,
        })

    async def end(self):
        """End the session with AgentWarden."""
        await self._client._post(f"/v1/sessions/{self.session_id}/end", {
            "session_id": self.session_id,
        })

    async def tool(
        self,
        name: str,
        params: Dict[str, Any] = None,
        target: str = None,
        execute: Callable[[], T] = None,
    ) -> T:
        """Governed tool call.

        Sends the action to AgentWarden for policy evaluation BEFORE
        executing. Raises :class:`ActionDenied` if blocked, or
        :class:`ActionPendingApproval` if human approval is required.

        Args:
            name: Tool name (e.g. "github.merge_pr", "slack.send_message").
            params: Tool parameters to include in the policy evaluation.
            target: Optional target resource (e.g. "github.com/org/repo").
            execute: Callable to invoke if the action is allowed.
                     Can be sync or async.

        Returns:
            Result of execute() if allowed, None if no execute provided.

        Raises:
            ActionDenied: If the policy denies the action.
            ActionPendingApproval: If the action requires human approval.
        """
        verdict = await self._evaluate("tool.call", name, params, target)
        self._handle_verdict(verdict)

        if execute:
            return await self._run_callable(execute)
        return None

    async def api_call(
        self,
        name: str,
        params: Dict[str, Any] = None,
        target: str = None,
        execute: Callable[[], T] = None,
    ) -> T:
        """Governed API call. Same pattern as :meth:`tool`.

        Args:
            name: API action name (e.g. "stripe.create_charge").
            params: Request parameters for policy evaluation.
            target: Optional target (e.g. "api.stripe.com").
            execute: Callable to invoke if allowed.

        Returns:
            Result of execute() if allowed.
        """
        verdict = await self._evaluate("api.request", name, params, target)
        self._handle_verdict(verdict)

        if execute:
            return await self._run_callable(execute)
        return None

    async def db_query(
        self,
        query: str,
        target: str = None,
        execute: Callable[[], T] = None,
    ) -> T:
        """Governed database query.

        Args:
            query: The SQL query string for policy evaluation.
            target: Optional database target (e.g. "production.users").
            execute: Callable to invoke if allowed.

        Returns:
            Result of execute() if allowed.
        """
        verdict = await self._evaluate(
            "db.query", "db.query", {"query": query}, target
        )
        self._handle_verdict(verdict)

        if execute:
            return await self._run_callable(execute)
        return None

    async def chat(
        self,
        model: str,
        messages: list,
        execute: Callable[[], T] = None,
    ) -> T:
        """Governed LLM call. For budget enforcement and model restrictions.

        Args:
            model: Model identifier (e.g. "gpt-4o", "claude-3-opus").
            messages: Chat messages for context (not sent to AgentWarden,
                      only the model name is evaluated).
            execute: Callable to invoke if allowed.

        Returns:
            Result of execute() if allowed.
        """
        verdict = await self._evaluate(
            "llm.chat", f"llm.chat.{model}", {"model": model}, None
        )
        self._handle_verdict(verdict)

        if execute:
            return await self._run_callable(execute)
        return None

    def trace_llm(self, response: Any, model: str = None):
        """Trace an LLM call passively (no governance, just recording).

        Use this for fire-and-forget observability when you do not need
        policy evaluation on the call itself.

        Args:
            response: The LLM response object (e.g. OpenAI ChatCompletion).
                      Token usage is extracted if available.
            model: Optional model name override.
        """
        usage = getattr(response, "usage", None)
        if usage:
            tokens_in = getattr(usage, "prompt_tokens", 0)
            tokens_out = getattr(usage, "completion_tokens", 0)
            # Cost tracking could be enhanced with model-specific pricing
            _ = tokens_in, tokens_out

    async def score(
        self,
        task_completed: bool = False,
        quality: float = None,
        metadata: Dict[str, Any] = None,
    ):
        """Score the session outcome. Feeds the evolution engine.

        Call this at the end of an agent run to report quality metrics.
        AgentWarden uses scores to evolve policies over time.

        Args:
            task_completed: Whether the agent successfully completed its task.
            quality: Quality score between 0.0 and 1.0.
            metadata: Additional metrics (e.g. latency, token count).
        """
        await self._client._post(f"/v1/sessions/{self.session_id}/score", {
            "session_id": self.session_id,
            "task_completed": task_completed,
            "quality": quality or 0.0,
            "metrics": {k: str(v) for k, v in (metadata or {}).items()},
        })

    # -- Internal helpers ---------------------------------------------------

    async def _evaluate(
        self,
        action_type: str,
        name: str,
        params: dict,
        target: str,
    ) -> dict:
        """Send an action to AgentWarden for policy evaluation."""
        self._action_count += 1
        duration = (
            int(time.time() - self._started_at) if self._started_at else 0
        )

        return await self._client._post("/v1/events/evaluate", {
            "session_id": self.session_id,
            "agent_id": self.agent_id,
            "agent_version": self.agent_version,
            "action": {
                "type": action_type,
                "name": name,
                "params_json": json.dumps(params or {}),
                "target": target or "",
            },
            "context": {
                "session_cost": self._cost,
                "session_action_count": self._action_count,
                "session_duration_seconds": duration,
            },
            "metadata": self.metadata,
        })

    def _handle_verdict(self, verdict: dict):
        """Interpret the policy verdict and raise if not allowed."""
        v = verdict.get("verdict", "allow")
        if v == "deny":
            raise ActionDenied(
                policy_name=verdict.get("policy_name", ""),
                message=verdict.get("message", "Action denied"),
                suggestions=verdict.get("suggestions", []),
            )
        elif v == "approve":
            raise ActionPendingApproval(
                approval_id=verdict.get("approval_id", ""),
                policy_name=verdict.get("policy_name", ""),
                timeout_seconds=verdict.get("timeout_seconds", 0),
            )
        elif v == "terminate":
            raise ActionDenied(
                policy_name=verdict.get("policy_name", ""),
                message=verdict.get("message", "Session terminated"),
                suggestions=verdict.get("suggestions", []),
            )

    @staticmethod
    async def _run_callable(fn: Callable[[], T]) -> T:
        """Execute a callable, handling both sync and async functions."""
        if inspect.iscoroutinefunction(fn):
            return await fn()
        result = fn()
        if inspect.isawaitable(result):
            return await result
        return result
