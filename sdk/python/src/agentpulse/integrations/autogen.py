"""
AutoGen / AG2 integration for AgentPulse.

Supports two code paths based on the installed package version:

  Legacy (pyautogen / autogen < 0.4):
    - Patches ConversableAgent.generate_reply() → llm.call span
    - Patches ConversableAgent.execute_function() → tool.call span
    - Patches ConversableAgent.send() → agent.handoff span
    - Patches GroupChatManager.run_chat() + GroupChat.select_speaker() for
      group-chat topology

  AG2 (autogen-agentchat >= 0.4):
    - Patches BaseChatAgent.on_messages() → agent.handoff span
      (generate_reply no longer exists on the base class in AG2 0.4+)
    - Group chat handoffs via SelectorGroupChat._select_speaker()

Usage::

    from agentpulse import init_tracer
    from agentpulse.integrations.autogen import instrument_autogen

    init_tracer()
    instrument_autogen()

    # All subsequent agent interactions are traced automatically.

Requires: pip install 'agentpulse[autogen]'
"""

from __future__ import annotations

import logging
from typing import Any, Optional

from opentelemetry import context as context_api, trace
from opentelemetry.trace import StatusCode

from agentpulse import attributes as attrs
from agentpulse._context import get_run_id
from agentpulse.integrations._base import (
    extract_usage,
    is_instrumented,
    mark_instrumented,
    record_usage_from_response,
    safe_truncate,
    set_common_attrs,
)

logger = logging.getLogger(__name__)

# ── Import resolution: try AG2 first, fall back to legacy autogen ─────────────

_AG2 = False
_autogen_mod: Any = None

try:
    import autogen_agentchat as _ag2  # type: ignore[import]
    from packaging.version import Version

    _ag2_version = Version(getattr(_ag2, "__version__", "0.0.0"))
    if _ag2_version >= Version("0.4"):
        _AG2 = True
        _autogen_mod = _ag2
        logger.debug("AgentPulse AutoGen: using AG2 code path (autogen_agentchat %s)", _ag2_version)
    else:
        # Installed but too old — fall through to legacy check
        logger.debug("AgentPulse AutoGen: autogen_agentchat %s < 0.4, trying legacy", _ag2_version)
except ImportError:
    pass

if not _AG2:
    try:
        import autogen as _legacy_autogen  # type: ignore[import]
        _autogen_mod = _legacy_autogen
        logger.debug("AgentPulse AutoGen: using legacy autogen code path")
    except ImportError:
        pass

if _autogen_mod is None:
    raise ImportError(
        "AutoGen integration requires either autogen-agentchat>=0.4 (AG2) "
        "or pyautogen (legacy). "
        "Install with: pip install 'agentpulse[autogen]'"
    )

# ── Packaging import (optional, for version comparison) ─────────────────────
try:
    from packaging.version import Version as _Version  # type: ignore[import]
    _HAS_PACKAGING = True
except ImportError:
    _HAS_PACKAGING = False

def _get_tracer() -> trace.Tracer:
    return trace.get_tracer("agentpulse.autogen")

# Saved original methods for uninstrument_autogen()
_originals: dict[str, Any] = {}


# ── Shared span wrappers ──────────────────────────────────────────────────────


def _wrap_sync(original_fn: Any, span_fn: Any) -> Any:
    """Return a sync wrapper that opens a span before calling original_fn."""
    import functools

    @functools.wraps(original_fn)
    def wrapper(self: Any, *args: Any, **kwargs: Any) -> Any:
        return span_fn(self, original_fn, *args, **kwargs)

    return wrapper


def _wrap_async(original_fn: Any, span_fn: Any) -> Any:
    """Return an async wrapper that opens a span before calling original_fn."""
    import functools

    @functools.wraps(original_fn)
    async def wrapper(self: Any, *args: Any, **kwargs: Any) -> Any:
        return await span_fn(self, original_fn, *args, **kwargs)

    return wrapper


# ── Legacy autogen (< 0.4) patches ───────────────────────────────────────────


def _legacy_generate_reply(self: Any, original: Any, *args: Any, **kwargs: Any) -> Any:
    import inspect
    agent_name = getattr(self, "name", type(self).__name__)
    model = "unknown"
    llm_config = getattr(self, "llm_config", {}) or {}
    config_list = llm_config.get("config_list", [{}])
    if config_list:
        model = config_list[0].get("model", "unknown")

    span = _get_tracer().start_span(f"llm.{model}")
    set_common_attrs(span, attrs.LLM_CALL, agent_name=agent_name, extra={attrs.MODEL_ID: model})
    token = context_api.attach(trace.set_span_in_context(span))
    try:
        if inspect.iscoroutinefunction(original):
            import asyncio
            result = asyncio.get_event_loop().run_until_complete(original(self, *args, **kwargs))
        else:
            result = original(self, *args, **kwargs)
        # Try to extract token usage from the reply if it has usage metadata
        if isinstance(result, dict):
            usage = result.get("usage")
            if usage:
                input_t, output_t = extract_usage(type("U", (), usage)())
                if input_t:
                    span.set_attribute(attrs.INPUT_TOKENS, input_t)
                if output_t:
                    span.set_attribute(attrs.OUTPUT_TOKENS, output_t)
        return result
    except Exception as exc:
        span.set_status(StatusCode.ERROR, str(exc))
        raise
    finally:
        context_api.detach(token)
        span.end()


def _legacy_execute_function(self: Any, original: Any, func_call: Any, *args: Any, **kwargs: Any) -> Any:
    func_name = func_call.get("name", "unknown_tool") if isinstance(func_call, dict) else getattr(func_call, "name", "unknown_tool")
    agent_name = getattr(self, "name", type(self).__name__)

    span = _get_tracer().start_span(f"tool.{func_name}")
    set_common_attrs(
        span, attrs.TOOL_CALL,
        agent_name=agent_name,
        extra={attrs.TOOL_NAME: func_name},
    )
    if isinstance(func_call, dict) and "arguments" in func_call:
        span.set_attribute("tool.input", safe_truncate(str(func_call["arguments"])))
    token = context_api.attach(trace.set_span_in_context(span))
    try:
        result = original(self, func_call, *args, **kwargs)
        if result:
            span.set_attribute("tool.output", safe_truncate(str(result)))
        return result
    except Exception as exc:
        span.set_status(StatusCode.ERROR, str(exc))
        raise
    finally:
        context_api.detach(token)
        span.end()


def _legacy_send(self: Any, original: Any, message: Any, recipient: Any, *args: Any, **kwargs: Any) -> Any:
    from_name = getattr(self, "name", type(self).__name__)
    to_name = getattr(recipient, "name", type(recipient).__name__)

    span = _get_tracer().start_span(f"handoff.{from_name}->{to_name}")
    set_common_attrs(
        span, attrs.AGENT_HANDOFF,
        agent_name=from_name,
        extra={attrs.HANDOFF_TARGET: to_name},
    )
    token = context_api.attach(trace.set_span_in_context(span))
    try:
        return original(self, message, recipient, *args, **kwargs)
    except Exception as exc:
        span.set_status(StatusCode.ERROR, str(exc))
        raise
    finally:
        context_api.detach(token)
        span.end()


def _instrument_legacy() -> None:
    """Instrument pyautogen (< 0.4) by monkey-patching ConversableAgent."""
    try:
        from autogen import ConversableAgent  # type: ignore[import]
    except ImportError:
        logger.warning("AgentPulse AutoGen: ConversableAgent not found in legacy autogen")
        return

    if is_instrumented(ConversableAgent):
        return

    # generate_reply
    if hasattr(ConversableAgent, "generate_reply"):
        _originals["legacy.generate_reply"] = ConversableAgent.generate_reply
        ConversableAgent.generate_reply = _wrap_sync(
            ConversableAgent.generate_reply, _legacy_generate_reply
        )

    # execute_function
    if hasattr(ConversableAgent, "execute_function"):
        _originals["legacy.execute_function"] = ConversableAgent.execute_function
        ConversableAgent.execute_function = _wrap_sync(
            ConversableAgent.execute_function, _legacy_execute_function
        )

    # send
    if hasattr(ConversableAgent, "send"):
        _originals["legacy.send"] = ConversableAgent.send
        ConversableAgent.send = _wrap_sync(
            ConversableAgent.send, _legacy_send
        )

    mark_instrumented(ConversableAgent)

    # GroupChatManager
    try:
        from autogen import GroupChatManager, GroupChat  # type: ignore[import]
        _instrument_legacy_groupchat(GroupChatManager, GroupChat)
    except ImportError:
        pass


def _instrument_legacy_groupchat(GroupChatManager: Any, GroupChat: Any) -> None:
    if is_instrumented(GroupChatManager):
        return

    original_run_chat = getattr(GroupChatManager, "run_chat", None)
    if original_run_chat:
        _originals["legacy.run_chat"] = original_run_chat

        def traced_run_chat(self: Any, messages: Any = None, sender: Any = None, config: Any = None) -> Any:
            name = getattr(self, "name", "GroupChatManager")
            span = _get_tracer().start_span("group_chat")
            set_common_attrs(span, attrs.AGENT_HANDOFF, agent_name=name)
            token = context_api.attach(trace.set_span_in_context(span))
            try:
                return original_run_chat(self, messages=messages, sender=sender, config=config)
            except Exception as exc:
                span.set_status(StatusCode.ERROR, str(exc))
                raise
            finally:
                context_api.detach(token)
                span.end()

        GroupChatManager.run_chat = traced_run_chat

    # Hook select_speaker to emit handoff events
    original_select = getattr(GroupChat, "select_speaker", None)
    if original_select:
        _originals["legacy.select_speaker"] = original_select

        def traced_select_speaker(self: Any, last_speaker: Any, selector: Any) -> Any:
            result = original_select(self, last_speaker, selector)
            # Emit an event on the current active span
            current_span = trace.get_current_span()
            if current_span and result:
                current_span.add_event(
                    "agent.handoff",
                    attributes={
                        attrs.AGENT_NAME: getattr(last_speaker, "name", "unknown"),
                        attrs.HANDOFF_TARGET: getattr(result, "name", "unknown"),
                    },
                )
            return result

        GroupChat.select_speaker = traced_select_speaker

    mark_instrumented(GroupChatManager)


# ── AG2 (autogen_agentchat >= 0.4) patches ───────────────────────────────────


def _instrument_ag2() -> None:
    """Instrument AG2 (autogen_agentchat >= 0.4) by patching BaseChatAgent."""
    try:
        from autogen_agentchat.base import BaseChatAgent  # type: ignore[import]
    except ImportError:
        logger.warning("AgentPulse AutoGen: BaseChatAgent not found in autogen_agentchat")
        return

    if is_instrumented(BaseChatAgent):
        return

    # on_messages (async) — the main agent execution hook in AG2 0.4+
    if hasattr(BaseChatAgent, "on_messages"):
        _originals["ag2.on_messages"] = BaseChatAgent.on_messages

        async def traced_on_messages(self: Any, messages: Any, cancellation_token: Any = None) -> Any:
            agent_name = getattr(self, "name", type(self).__name__)
            get_run_id()  # ensure run_id is pinned
            span = _get_tracer().start_span(f"agent.{agent_name}")
            set_common_attrs(span, attrs.AGENT_HANDOFF, agent_name=agent_name)
            token = context_api.attach(trace.set_span_in_context(span))
            try:
                original = _originals["ag2.on_messages"]
                result = await original(self, messages, cancellation_token)
                # Record output
                if result:
                    output_text = getattr(result, "chat_message", None)
                    if output_text:
                        content = getattr(output_text, "content", None)
                        if content:
                            span.set_attribute(attrs.COMPLETION, safe_truncate(str(content)))
                return result
            except Exception as exc:
                span.set_status(StatusCode.ERROR, str(exc))
                raise
            finally:
                context_api.detach(token)
                span.end()

        BaseChatAgent.on_messages = traced_on_messages

    mark_instrumented(BaseChatAgent)

    # SelectorGroupChat handoffs
    try:
        from autogen_agentchat.teams import SelectorGroupChat  # type: ignore[import]
        _instrument_ag2_selector(SelectorGroupChat)
    except ImportError:
        pass


def _instrument_ag2_selector(SelectorGroupChat: Any) -> None:
    if is_instrumented(SelectorGroupChat):
        return

    select_fn = getattr(SelectorGroupChat, "_select_speaker", None)
    if select_fn is None:
        return
    _originals["ag2.select_speaker"] = select_fn

    async def traced_select(self: Any, *args: Any, **kwargs: Any) -> Any:
        result = await _originals["ag2.select_speaker"](self, *args, **kwargs)
        current_span = trace.get_current_span()
        if current_span and result:
            current_span.add_event(
                "agent.handoff",
                attributes={attrs.HANDOFF_TARGET: getattr(result, "name", str(result))},
            )
        return result

    SelectorGroupChat._select_speaker = traced_select
    mark_instrumented(SelectorGroupChat)


# ── Public API ────────────────────────────────────────────────────────────────


def instrument_autogen() -> None:
    """Instrument the installed AutoGen / AG2 package.

    Automatically detects which version is installed and applies the
    appropriate patches. Safe to call multiple times — subsequent calls
    are no-ops.

    For AG2 (autogen_agentchat >= 0.4):
        Patches BaseChatAgent.on_messages and SelectorGroupChat._select_speaker.

    For legacy autogen (pyautogen < 0.4):
        Patches ConversableAgent.generate_reply, execute_function, send,
        GroupChatManager.run_chat, and GroupChat.select_speaker.
    """
    if _AG2:
        _instrument_ag2()
    else:
        _instrument_legacy()


def uninstrument_autogen() -> None:
    """Remove AgentPulse patches from AutoGen classes.

    Restores all patched methods to their original implementations.
    """
    if _AG2:
        try:
            from autogen_agentchat.base import BaseChatAgent  # type: ignore[import]
            if "ag2.on_messages" in _originals:
                BaseChatAgent.on_messages = _originals.pop("ag2.on_messages")
            try:
                delattr(BaseChatAgent, "_agentpulse_instrumented")
            except AttributeError:
                pass
            try:
                from autogen_agentchat.teams import SelectorGroupChat  # type: ignore[import]
                if "ag2.select_speaker" in _originals:
                    SelectorGroupChat._select_speaker = _originals.pop("ag2.select_speaker")
                try:
                    delattr(SelectorGroupChat, "_agentpulse_instrumented")
                except AttributeError:
                    pass
            except ImportError:
                pass
        except ImportError:
            pass
    else:
        try:
            from autogen import ConversableAgent  # type: ignore[import]
            for key, attr in [
                ("legacy.generate_reply", "generate_reply"),
                ("legacy.execute_function", "execute_function"),
                ("legacy.send", "send"),
            ]:
                if key in _originals:
                    setattr(ConversableAgent, attr, _originals.pop(key))
            try:
                delattr(ConversableAgent, "_agentpulse_instrumented")
            except AttributeError:
                pass
        except ImportError:
            pass
        try:
            from autogen import GroupChatManager, GroupChat  # type: ignore[import]
            if "legacy.run_chat" in _originals:
                GroupChatManager.run_chat = _originals.pop("legacy.run_chat")
            if "legacy.select_speaker" in _originals:
                GroupChat.select_speaker = _originals.pop("legacy.select_speaker")
            try:
                delattr(GroupChatManager, "_agentpulse_instrumented")
            except AttributeError:
                pass
        except ImportError:
            pass

    _originals.clear()
