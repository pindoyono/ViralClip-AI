"""Tests for transcript_service.py using mocked Whisper."""
import pytest
from unittest.mock import AsyncMock, MagicMock, patch


MOCK_WHISPER_RESULT = {
    "language": "en",
    "text": "Hello world this is a test transcript.",
    "segments": [
        {"start": 0.0, "end": 2.5, "text": "Hello world", "no_speech_prob": 0.05},
        {"start": 2.5, "end": 5.0, "text": "this is a test transcript.", "no_speech_prob": 0.02},
    ],
}


@pytest.fixture(autouse=True)
def mock_whisper_model():
    """Patch whisper.load_model globally for all tests in this module."""
    mock_model = MagicMock()
    mock_model.transcribe.return_value = MOCK_WHISPER_RESULT

    with patch("app.services.transcript_service._whisper_model", mock_model):
        yield mock_model


@pytest.fixture(autouse=True)
def mock_ffmpeg():
    """Patch ffmpeg utilities so no real files are needed."""
    with (
        patch("app.services.transcript_service.extract_audio") as mock_extract,
        patch("app.services.transcript_service.get_video_info") as mock_info,
        patch("app.services.transcript_service.generate_temp_path", return_value="/tmp/fake_audio.wav"),
        patch("app.services.transcript_service.clean_temp_file"),
    ):
        mock_info.return_value = {"duration": 5.0}
        mock_extract.return_value = None
        yield mock_extract, mock_info


@pytest.mark.asyncio
async def test_transcribe_video_returns_correct_structure():
    """transcribe_video should return language, duration, segments, full_text."""
    from app.services.transcript_service import transcribe_video

    result = await transcribe_video("/fake/video.mp4")

    assert result["language"] == "en"
    assert result["duration"] == 5.0
    assert result["full_text"] == "Hello world this is a test transcript."
    assert len(result["segments"]) == 2


@pytest.mark.asyncio
async def test_transcribe_video_segment_fields():
    """Each segment should have start, end, text, and confidence."""
    from app.services.transcript_service import transcribe_video

    result = await transcribe_video("/fake/video.mp4")
    seg = result["segments"][0]

    assert seg.start == 0.0
    assert seg.end == 2.5
    assert seg.text == "Hello world"
    assert isinstance(seg.confidence, float)


@pytest.mark.asyncio
async def test_transcribe_video_with_language_hint():
    """Language hint should be passed through to Whisper model."""
    from app.services.transcript_service import transcribe_video

    with patch("app.services.transcript_service._whisper_model") as mock_model:
        mock_model.transcribe.return_value = MOCK_WHISPER_RESULT
        await transcribe_video("/fake/video.mp4", language="es")
        call_kwargs = mock_model.transcribe.call_args[1]
        assert call_kwargs.get("language") == "es"


@pytest.mark.asyncio
async def test_transcribe_video_no_language_hint_omits_param():
    """Without a language hint, 'language' key should not appear in Whisper call."""
    from app.services.transcript_service import transcribe_video

    with patch("app.services.transcript_service._whisper_model") as mock_model:
        mock_model.transcribe.return_value = MOCK_WHISPER_RESULT
        await transcribe_video("/fake/video.mp4")
        call_kwargs = mock_model.transcribe.call_args[1]
        assert "language" not in call_kwargs


@pytest.mark.asyncio
async def test_transcribe_video_cleans_temp_file_on_success():
    """Temporary audio file should be removed after a successful transcription."""
    from app.services.transcript_service import transcribe_video

    with patch("app.services.transcript_service.clean_temp_file") as mock_clean:
        await transcribe_video("/fake/video.mp4")
        mock_clean.assert_called_once()


@pytest.mark.asyncio
async def test_transcribe_video_cleans_temp_file_on_error():
    """Temporary audio file should be removed even when an error occurs."""
    from app.services.transcript_service import transcribe_video

    with (
        patch("app.services.transcript_service._whisper_model") as mock_model,
        patch("app.services.transcript_service.clean_temp_file") as mock_clean,
    ):
        mock_model.transcribe.side_effect = RuntimeError("CUDA OOM")
        with pytest.raises(RuntimeError):
            await transcribe_video("/fake/video.mp4")
        mock_clean.assert_called_once()


@pytest.mark.asyncio
async def test_transcribe_video_processing_time_positive():
    """processing_time in the result should be a positive number."""
    from app.services.transcript_service import transcribe_video

    result = await transcribe_video("/fake/video.mp4")
    assert result["processing_time"] >= 0


@pytest.mark.asyncio
async def test_transcribe_video_empty_segments():
    """Service should handle Whisper returning no segments gracefully."""
    from app.services.transcript_service import transcribe_video

    with patch("app.services.transcript_service._whisper_model") as mock_model:
        mock_model.transcribe.return_value = {
            "language": "en",
            "text": "",
            "segments": [],
        }
        result = await transcribe_video("/fake/video.mp4")
        assert result["segments"] == []
        assert result["full_text"] == ""
