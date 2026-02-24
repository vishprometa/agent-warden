"""Framework integrations for AgentWarden v2.

Each integration module gracefully handles missing dependencies with
``try/except ImportError`` so installing the core SDK does not pull in
framework-specific packages.

Available integrations:

- **LangChain**: ``agentwarden.integrations.langchain.AgentWardenCallbackHandler``
- **CrewAI**: ``agentwarden.integrations.crewai.GovernedCrew``
- **OpenAI Agents SDK**: ``agentwarden.integrations.openai_agents.GovernedRunner``

Install the corresponding extras to use an integration::

    pip install agentwarden[langchain]
    pip install agentwarden[crewai]
    pip install agentwarden[openai-agents]
    pip install agentwarden[all]
"""
