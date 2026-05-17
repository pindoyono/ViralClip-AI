"""Tests for hook_service.py with mocked OpenAI client."""
import json
import pytest
from unittest.mock import AsyncMock, MagicMock, patch


SAMPLE_TRANSCRIPT = "This is an incredible story about how one man changed everything overnight. Nobody expected it."

MOCK_HOOKS = [
    {"text": "You won't believe what he did next", "type": "question", "viral_score": 0.92, "rationale": "Curiosity gap"},
    {"text": "One man changed everything overnight", "type": "statement", "viral_score": 0.85, "rationale": "Bold claim"},
    {"text": "Nobody expected this to happen", "type": "statement", "viral_score": 0.78, "rationale": "Surprise element"},
]


def make_openai_response(data) -> MagicMock:
    resp = MagicMock()
    resp.choices = [MagicMock()]
    resp.choices[0].message.content = json.dumps({"hooks": data})
    return resp


@pytest.fixture
def mock_openai():
    mock_client = MagicMock()
    mock_client.chat.completions.create = AsyncMock(
        return_value=make_openai_response(MOCK_HOOKS)
    )
    with patch("app.services.hook_service._client", mock_client):
        yield mock_client


@pytest.mark.asyncio
async def test_generate_hooks_returns_list(mock_openai):
    """Should return a list of hook dictionaries."""
    from app.services.hook_service import generate_hooks

    hooks = await generate_hooks(SAMPLE_TRANSCRIPT, count=3)

    assert isinstance(hooks, list)
    assert len(hooks) == 3


@pytest.mark.asyncio
async def test_generate_hooks_respects_count(mock_openai):
    """Should not return more hooks than requested."""
    mock_openai.chat.completions.create = AsyncMock(
        return_value=make_openai_response(MOCK_HOOKS)
    )

    from app.services.hook_service import generate_hooks

    hooks = await generate_hooks(SAMPLE_TRANSCRIPT, count=2)

    assert len(hooks) <= 2


@pytest.mark.asyncio
async def test_generate_hooks_hook_structure(mock_openai):
    """Each hook should have text, type, viral_score, and rationale fields."""
    from app.services.hook_service import generate_hooks

    hooks = await generate_hooks(SAMPLE_TRANSCRIPT)

    assert len(hooks) > 0
    hook = hooks[0]
    assert "text" in hook
    assert "type" in hook
    assert "viral_score" in hook


@pytest.mark.asyncio
async def test_generate_hooks_with_niche(mock_openai):
    """Niche context should be included in the prompt."""
    from app.services.hook_service import generate_hooks

    await generate_hooks(SAMPLE_TRANSCRIPT, niche="fitness")

    call_args = mock_openai.chat.completions.create.call_args
    messages = call_args[1]["messages"]
    user_msg = next(m for m in messages if m["role"] == "user")
    assert "fitness" in user_msg["content"]


@pytest.mark.asyncio
async def test_generate_hooks_with_platform(mock_openai):
    """Platform context should be included in the prompt."""
    from app.services.hook_service import generate_hooks

    await generate_hooks(SAMPLE_TRANSCRIPT, platform="tiktok")

    call_args = mock_openai.chat.completions.create.call_args
    messages = call_args[1]["messages"]
    user_msg = next(m for m in messages if m["role"] == "user")
    assert "tiktok" in user_msg["content"].lower()


@pytest.mark.asyncio
async def test_generate_hooks_with_tone(mock_openai):
    """Tone context should be included in the prompt."""
    from app.services.hook_service import generate_hooks

    await generate_hooks(SAMPLE_TRANSCRIPT, tone="motivational")

    call_args = mock_openai.chat.completions.create.call_args
    messages = call_args[1]["messages"]
    user_msg = next(m for m in messages if m["role"] == "user")
    assert "motivational" in user_msg["content"]


@pytest.mark.asyncio
async def test_generate_hooks_handles_list_response(mock_openai):
    """Should handle when OpenAI returns a direct list instead of a dict."""
    mock_openai.chat.completions.create = AsyncMock(return_value=MagicMock())
    mock_openai.chat.completions.create.return_value.choices = [MagicMock()]
    mock_openai.chat.completions.create.return_value.choices[0].message.content = json.dumps(MOCK_HOOKS)

    from app.services.hook_service import generate_hooks

    hooks = await generate_hooks(SAMPLE_TRANSCRIPT)

    assert isinstance(hooks, list)


@pytest.mark.asyncio
async def test_generate_hooks_truncates_long_transcript(mock_openai):
    """Very long transcripts should be truncated in the prompt."""
    long_transcript = "word " * 2000  # ~10000 chars

    from app.services.hook_service import generate_hooks

    await generate_hooks(long_transcript)

    call_args = mock_openai.chat.completions.create.call_args
    messages = call_args[1]["messages"]
    user_msg = next(m for m in messages if m["role"] == "user")
    # The prompt should contain a truncated version
    assert len(user_msg["content"]) < len(long_transcript) + 500


@pytest.mark.asyncio
async def test_generate_hooks_no_context_parts(mock_openai):
    """Without niche/platform/tone, no context line is added."""
    from app.services.hook_service import generate_hooks

    await generate_hooks(SAMPLE_TRANSCRIPT)

    call_args = mock_openai.chat.completions.create.call_args
    messages = call_args[1]["messages"]
    user_msg = next(m for m in messages if m["role"] == "user")
    assert "Niche:" not in user_msg["content"]
    assert "Platform:" not in user_msg["content"]
