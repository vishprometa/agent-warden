"""AgentWarden Python SDK -- runtime governance for AI agents."""

import os
import json
from typing import Optional, Dict, Any
from contextlib import asynccontextmanager

import httpx


class AgentWarden:
    """Main client for AgentWarden governance server.

    Auto-discovers AgentWarden on localhost:6777 if no args provided.
    Uses HTTP transport by default, with optional gRPC for high-throughput
    use cases.

    Example::

        warden = AgentWarden()

        async with warden.session(agent_id="support-bot") as session:
            await session.tool(
                name="github.merge_pr",
                params={"repo": "org/repo", "pr": 42},
                execute=lambda: github.merge(42),
            )
    """

    def __init__(
        self,
        host: str = None,
        port: int = None,
        grpc_port: int = None,
        use_grpc: bool = False,
        api_key: str = None,
    ):
        """
        Initialize the AgentWarden client.

        Args:
            host: AgentWarden host (default: localhost, env: AGENTWARDEN_HOST)
            port: HTTP port (default: 6777, env: AGENTWARDEN_PORT)
            grpc_port: gRPC port (default: 6778, env: AGENTWARDEN_GRPC_PORT)
            use_grpc: Use gRPC transport (default: False, uses HTTP)
            api_key: Optional API key for authentication (env: AGENTWARDEN_API_KEY)
        """
        self.host = host or os.getenv("AGENTWARDEN_HOST", "localhost")
        self.port = port or int(os.getenv("AGENTWARDEN_PORT", "6777"))
        self.grpc_port = grpc_port or int(os.getenv("AGENTWARDEN_GRPC_PORT", "6778"))
        self.use_grpc = use_grpc
        self.api_key = api_key or os.getenv("AGENTWARDEN_API_KEY")
        self._http = httpx.AsyncClient(
            base_url=f"http://{self.host}:{self.port}",
            timeout=30.0,
        )

    @asynccontextmanager
    async def session(
        self,
        agent_id: str,
        agent_version: str = None,
        metadata: Dict[str, str] = None,
    ):
        """Context manager for a governed agent session.

        Creates a session on enter, ends it on exit. All governed actions
        within the block are scoped to this session.

        Args:
            agent_id: Identifier for the agent being governed.
            agent_version: Version string (default: "v1").
            metadata: Key-value metadata attached to the session.

        Yields:
            A :class:`Session` bound to this agent session.

        Example::

            async with warden.session(agent_id="support-bot") as session:
                await session.tool(name="zendesk.reply", ...)
        """
        from agentwarden.session import Session

        session = Session(
            client=self,
            agent_id=agent_id,
            agent_version=agent_version or "v1",
            metadata=metadata or {},
        )
        await session.start()
        try:
            yield session
        finally:
            await session.end()

    async def _post(self, path: str, data: dict) -> dict:
        """HTTP POST to AgentWarden.

        Attaches the API key as a Bearer token if configured.

        Args:
            path: URL path (e.g. "/v1/sessions/start").
            data: JSON body.

        Returns:
            Parsed JSON response dict.
        """
        headers = {}
        if self.api_key:
            headers["Authorization"] = f"Bearer {self.api_key}"
        resp = await self._http.post(path, json=data, headers=headers)
        resp.raise_for_status()
        return resp.json()

    async def close(self):
        """Close the underlying HTTP client."""
        await self._http.aclose()
