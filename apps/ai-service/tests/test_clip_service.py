"""Tests for clip_service.py with mocked OpenAI client."""
import json
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from app.models.schemas import TranscriptSegment


def make_segments(count: int = 3) -> list[TranscriptSegment]:
    segments = []
    for i in range(count):
        segments.append(
            TranscriptSegment(
                start=float(i * 30),
                end=float((i + 1) * 30),
                text=f"Segment {i} text content here.",
                confidence=0.95,
            )
        )
    return segments


def make_openai_response(clips_data: list) -> MagicMock:
    """Build a mock OpenAI chat completion response."""
    response = MagicMock()
    response.choices = [MagicMock()]
    response.choices[0].message.content = json.dumps({"clips": clips_data})
    return response


VALID_CLIP = {
    "start_time": 5.0,
    "end_time": 45.0,
    "viral_score": 0.87,
    "rationale": "Highly engaging moment with emotional peak.",
    "hook_text": "You won't believe what happens next...",
    "suggested_title": "Incredible Moment Caught on Video",
    "hashtags": ["viral", "wow", "trending"],
    "suggested_for": ["tiktok", "reels", "shorts"],
}


@pytest.fixture
def mock_openai():
    mock_client = MagicMock()
    mock_client.chat = MagicMock()
    mock_client.chat.completions = MagicMock()
    mock_client.chat.completions.create = AsyncMock(
        return_value=make_openai_response([VALID_CLIP])
    )
    with patch("app.services.clip_service._client", mock_client):
        yield mock_client


@pytest.mark.asyncio
async def test_identify_viral_segments_returns_clips(mock_openai):
    """Should return a list of ClipSegment objects."""
    from app.services.clip_service import identify_viral_segments

    segments = make_segments(3)
    clips = await identify_viral_segments(segments)

    assert len(clips) == 1
    assert clips[0].start_time == 5.0
    assert clips[0].end_time == 45.0
    assert clips[0].viral_score == pytest.approx(0.87)


@pytest.mark.asyncio
async def test_identify_viral_segments_sorted_by_score(mock_openai):
    """Clips should be sorted in descending order of viral_score."""
    mock_openai.chat.completions.create = AsyncMock(
        return_value=make_openai_response([
            {**VALID_CLIP, "start_time": 0.0, "end_time": 30.0, "viral_score": 0.6},
            {**VALID_CLIP, "start_time": 40.0, "end_time": 80.0, "viral_score": 0.9},
            {**VALID_CLIP, "start_time": 90.0, "end_time": 120.0, "viral_score": 0.75},
        ])
    )

    from app.services.clip_service import identify_viral_segments

    clips = await identify_viral_segments(make_segments())

    scores = [c.viral_score for c in clips]
    assert scores == sorted(scores, reverse=True)


@pytest.mark.asyncio
async def test_identify_viral_segments_filters_short_clips(mock_openai):
    """Clips shorter than min_duration should be excluded."""
    mock_openai.chat.completions.create = AsyncMock(
        return_value=make_openai_response([
            {**VALID_CLIP, "start_time": 0.0, "end_time": 5.0},   # 5s - too short
            {**VALID_CLIP, "start_time": 10.0, "end_time": 40.0}, # 30s - valid
        ])
    )

    from app.services.clip_service import identify_viral_segments

    clips = await identify_viral_segments(make_segments(), min_duration=15.0, max_duration=90.0)

    assert len(clips) == 1
    assert clips[0].start_time == 10.0


@pytest.mark.asyncio
async def test_identify_viral_segments_filters_long_clips(mock_openai):
    """Clips longer than max_duration should be excluded."""
    mock_openai.chat.completions.create = AsyncMock(
        return_value=make_openai_response([
            {**VALID_CLIP, "start_time": 0.0, "end_time": 200.0},  # 200s - too long
            {**VALID_CLIP, "start_time": 10.0, "end_time": 50.0},  # 40s - valid
        ])
    )

    from app.services.clip_service import identify_viral_segments

    clips = await identify_viral_segments(make_segments(), min_duration=15.0, max_duration=90.0)

    assert len(clips) == 1
    assert clips[0].end_time == 50.0


@pytest.mark.asyncio
async def test_identify_viral_segments_respects_max_clips(mock_openai):
    """Should not return more clips than max_clips."""
    many_clips = [
        {**VALID_CLIP, "start_time": float(i * 30), "end_time": float(i * 30 + 20), "viral_score": 0.5}
        for i in range(10)
    ]
    mock_openai.chat.completions.create = AsyncMock(
        return_value=make_openai_response(many_clips)
    )

    from app.services.clip_service import identify_viral_segments

    clips = await identify_viral_segments(make_segments(), max_clips=3)

    assert len(clips) <= 3


@pytest.mark.asyncio
async def test_identify_viral_segments_clamps_viral_score(mock_openai):
    """Viral score should be clamped between 0 and 1."""
    mock_openai.chat.completions.create = AsyncMock(
        return_value=make_openai_response([
            {**VALID_CLIP, "viral_score": 1.5},   # over 1
            {**VALID_CLIP, "start_time": 40.0, "end_time": 70.0, "viral_score": -0.3},  # under 0
        ])
    )

    from app.services.clip_service import identify_viral_segments

    clips = await identify_viral_segments(make_segments())

    for c in clips:
        assert 0.0 <= c.viral_score <= 1.0


@pytest.mark.asyncio
async def test_identify_viral_segments_skips_malformed(mock_openai):
    """Malformed clip data should be skipped without crashing."""
    mock_openai.chat.completions.create = AsyncMock(
        return_value=make_openai_response([
            {"invalid": "data"},                   # missing required fields
            {**VALID_CLIP},                         # valid
        ])
    )

    from app.services.clip_service import identify_viral_segments

    clips = await identify_viral_segments(make_segments())

    # Should only include the valid clip
    assert len(clips) >= 1


@pytest.mark.asyncio
async def test_identify_viral_segments_with_content_profile(mock_openai):
    """Content profile should be included in the prompt context."""
    from app.services.clip_service import identify_viral_segments

    profile = {"niche": "fitness", "tone": "motivational", "platforms": ["tiktok"]}
    clips = await identify_viral_segments(make_segments(), content_profile=profile)

    mock_openai.chat.completions.create.assert_called_once()
    call_args = mock_openai.chat.completions.create.call_args
    messages = call_args[1]["messages"]
    user_message = next(m for m in messages if m["role"] == "user")
    assert "fitness" in user_message["content"]
