"""
AgentPulse framework auto-instrumentation integrations.

Each integration is opt-in and requires the corresponding framework to be
installed. Import the integration module directly — do not import from this
package, as integration modules have optional third-party dependencies that
are not installed by default.

Available integrations
----------------------

LangChain (pip install 'agentpulse[langchain]')::

    from agentpulse.integrations.langchain import AgentPulseCallbackHandler
    handler = AgentPulseCallbackHandler()
    chain.invoke({"input": "..."}, config={"callbacks": [handler]})

OpenAI Agents SDK (pip install 'agentpulse[openai]')::

    from agentpulse.integrations.openai_agents import instrument_openai_agents
    hooks = instrument_openai_agents()
    result = await Runner.run(agent, "task", hooks=hooks)

CrewAI (pip install 'agentpulse[crewai]')::

    from agentpulse.integrations.crewai import instrument_crewai
    instrument_crewai()   # patches Crew, Agent, BaseTool globally

AutoGen / AG2 (pip install 'agentpulse[autogen]')::

    from agentpulse.integrations.autogen import instrument_autogen
    instrument_autogen()  # auto-detects AG2 vs legacy autogen

LlamaIndex (pip install 'agentpulse[llamaindex]')::

    from agentpulse.integrations.llamaindex import instrument_llamaindex
    instrument_llamaindex()  # registers non-destructively on the root Dispatcher
"""
