"""Tests for metadata_service.py with mocked OpenAI client."""
import json
import pytest
from unittest.mock import AsyncMock, MagicMock, patch


SAMPLE_TRANSCRIPT = "Learn how to build muscle in just 30 days using science-backed techniques."

MOCK_METADATA = {
    "title": "Build Muscle in 30 Days",
    "description": "Science-backed techniques to transform your body in just one month.",
    "hashtags": ["fitness", "muscle", "gym", "workout", "bodybuilding"],
    "keywords": ["muscle building", "30 day challenge", "fitness tips"],
    "category": "Health & Fitness",
    "optimal_post_times": ["7:00 PM EST on Weekdays", "10:00 AM EST on Weekends"],
}


def make_openai_response(data: dict) -> MagicMock:
    resp = MagicMock()
    resp.choices = [MagicMock()]
    resp.choices[0].message.content = json.dumps(data)
    return resp


@pytest.fixture
def mock_openai():
    mock_client = MagicMock()
    mock_client.chat.completions.create = AsyncMock(
        return_value=make_openai_response(MOCK_METADATA)
    )
    with patch("app.services.metadata_service._client", mock_client):
        yield mock_client


@pytest.mark.asyncio
async def test_generate_metadata_returns_required_fields(mock_openai):
    """Should return a dict with all required metadata fields."""
    from app.services.metadata_service import generate_metadata

    result = await generate_metadata(SAMPLE_TRANSCRIPT, platform="tiktok")

    assert "title" in result
    assert "description" in result
    assert "hashtags" in result
    assert "keywords" in result
    assert "category" in result
    assert "optimal_post_times" in result


@pytest.mark.asyncio
async def test_generate_metadata_title_populated(mock_openai):
    """Title should be a non-empty string."""
    from app.services.metadata_service import generate_metadata

    result = await generate_metadata(SAMPLE_TRANSCRIPT, platform="tiktok")

    assert isinstance(result["title"], str)
    assert len(result["title"]) > 0


@pytest.mark.asyncio
async def test_generate_metadata_hashtags_are_list(mock_openai):
    """Hashtags should be returned as a list."""
    from app.services.metadata_service import generate_metadata

    result = await generate_metadata(SAMPLE_TRANSCRIPT, platform="instagram")

    assert isinstance(result["hashtags"], list)


@pytest.mark.asyncio
async def test_generate_metadata_tiktok_hashtag_limit(mock_openai):
    """TikTok should be capped at 10 hashtags."""
    many_tags = [f"tag{i}" for i in range(20)]
    mock_openai.chat.completions.create = AsyncMock(
        return_value=make_openai_response({**MOCK_METADATA, "hashtags": many_tags})
    )

    from app.services.metadata_service import generate_metadata

    result = await generate_metadata(SAMPLE_TRANSCRIPT, platform="tiktok")

    assert len(result["hashtags"]) <= 10


@pytest.mark.asyncio
async def test_generate_metadata_instagram_hashtag_limit(mock_openai):
    """Instagram should be capped at 30 hashtags."""
    many_tags = [f"tag{i}" for i in range(40)]
    mock_openai.chat.completions.create = AsyncMock(
        return_value=make_openai_response({**MOCK_METADATA, "hashtags": many_tags})
    )

    from app.services.metadata_service import generate_metadata

    result = await generate_metadata(SAMPLE_TRANSCRIPT, platform="instagram")

    assert len(result["hashtags"]) <= 30


@pytest.mark.asyncio
async def test_generate_metadata_youtube_hashtag_limit(mock_openai):
    """YouTube should be capped at 15 hashtags."""
    many_tags = [f"tag{i}" for i in range(20)]
    mock_openai.chat.completions.create = AsyncMock(
        return_value=make_openai_response({**MOCK_METADATA, "hashtags": many_tags})
    )

    from app.services.metadata_service import generate_metadata

    result = await generate_metadata(SAMPLE_TRANSCRIPT, platform="youtube")

    assert len(result["hashtags"]) <= 15


@pytest.mark.asyncio
async def test_generate_metadata_unknown_platform_default_limit(mock_openai):
    """Unknown platforms should fall back to default hashtag limit of 10."""
    many_tags = [f"tag{i}" for i in range(20)]
    mock_openai.chat.completions.create = AsyncMock(
        return_value=make_openai_response({**MOCK_METADATA, "hashtags": many_tags})
    )

    from app.services.metadata_service import generate_metadata

    result = await generate_metadata(SAMPLE_TRANSCRIPT, platform="snapchat")

    assert len(result["hashtags"]) <= 10


@pytest.mark.asyncio
async def test_generate_metadata_with_niche_and_tone(mock_openai):
    """Niche and tone should appear in the prompt context."""
    from app.services.metadata_service import generate_metadata

    await generate_metadata(SAMPLE_TRANSCRIPT, platform="tiktok", niche="fitness", tone="motivational")

    call_args = mock_openai.chat.completions.create.call_args
    messages = call_args[1]["messages"]
    user_msg = next(m for m in messages if m["role"] == "user")
    assert "fitness" in user_msg["content"]
    assert "motivational" in user_msg["content"]


@pytest.mark.asyncio
async def test_generate_metadata_platform_in_prompt(mock_openai):
    """Platform should always appear in the prompt."""
    from app.services.metadata_service import generate_metadata

    await generate_metadata(SAMPLE_TRANSCRIPT, platform="youtube")

    call_args = mock_openai.chat.completions.create.call_args
    messages = call_args[1]["messages"]
    user_msg = next(m for m in messages if m["role"] == "user")
    assert "YOUTUBE" in user_msg["content"]


@pytest.mark.asyncio
async def test_generate_metadata_default_category_fallback(mock_openai):
    """Missing category in response should default to 'Entertainment'."""
    mock_openai.chat.completions.create = AsyncMock(
        return_value=make_openai_response({**MOCK_METADATA, "category": None})
    )

    from app.services.metadata_service import generate_metadata

    result = await generate_metadata(SAMPLE_TRANSCRIPT, platform="tiktok")

    assert result["category"] is not None
