"""Integration tests for FastAPI routes using TestClient."""
import json
import os
import pytest
from unittest.mock import AsyncMock, MagicMock, patch
from fastapi.testclient import TestClient


@pytest.fixture(scope="module")
def client():
    """Create a FastAPI TestClient with all services mocked."""
    # Patch external dependencies before importing the app
    with (
        patch("app.services.transcript_service._whisper_model", MagicMock()),
        patch("app.services.clip_service._client", MagicMock()),
        patch("app.services.hook_service._client", MagicMock()),
        patch("app.services.metadata_service._client", MagicMock()),
    ):
        from main import app
        with TestClient(app) as c:
            yield c


# ---- Health endpoints ----

def test_health_endpoint(client):
    """GET /health should return 200 with status ok."""
    resp = client.get("/health")
    assert resp.status_code == 200
    body = resp.json()
    assert body["status"] == "ok"
    assert "version" in body


def test_ready_endpoint(client):
    """GET /ready should return 200 with status ready."""
    resp = client.get("/ready")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ready"


# ---- Transcript endpoint ----

def test_transcript_endpoint_file_not_found(client):
    """POST /api/v1/transcript should 404 when file does not exist."""
    payload = {
        "video_id": "vid-123",
        "storage_path": "nonexistent/video.mp4",
    }
    resp = client.post("/api/v1/transcript", json=payload)
    assert resp.status_code in (400, 404)


def test_transcript_endpoint_invalid_path_traversal(client):
    """POST /api/v1/transcript should reject path traversal attempts."""
    payload = {
        "video_id": "vid-123",
        "storage_path": "../../etc/passwd",
    }
    resp = client.post("/api/v1/transcript", json=payload)
    assert resp.status_code == 400


def test_transcript_endpoint_missing_video_id(client):
    """POST /api/v1/transcript without video_id should return 422."""
    resp = client.post("/api/v1/transcript", json={"storage_path": "video.mp4"})
    assert resp.status_code == 422


def test_transcript_endpoint_success(tmp_path):
    """POST /api/v1/transcript should return transcript when file exists."""
    video_file = tmp_path / "test.mp4"
    video_file.write_bytes(b"fake video content")

    mock_transcript_result = {
        "language": "en",
        "duration": 10.0,
        "segments": [],
        "full_text": "Hello world",
        "processing_time": 1.5,
    }

    with (
        patch("app.services.transcript_service.transcribe_video", new=AsyncMock(return_value=mock_transcript_result)),
        patch("app.utils.ffmpeg_utils._validate_storage_path", return_value=str(video_file)),
    ):
        from main import app
        with TestClient(app) as c:
            resp = c.post("/api/v1/transcript", json={
                "video_id": "vid-test",
                "storage_path": "test.mp4",
            })

    assert resp.status_code == 200
    body = resp.json()
    assert body["video_id"] == "vid-test"
    assert body["language"] == "en"


# ---- Clips endpoint ----

def test_clips_endpoint_file_not_found(client):
    """POST /api/v1/clips should 404 when file does not exist."""
    payload = {
        "video_id": "vid-456",
        "storage_path": "missing/video.mp4",
    }
    resp = client.post("/api/v1/clips", json=payload)
    assert resp.status_code in (400, 404)


def test_clips_endpoint_invalid_path(client):
    """POST /api/v1/clips should reject path traversal."""
    payload = {
        "video_id": "vid-456",
        "storage_path": "../../../etc/shadow",
    }
    resp = client.post("/api/v1/clips", json=payload)
    assert resp.status_code == 400


def test_clips_endpoint_missing_fields(client):
    """POST /api/v1/clips without required fields should return 422."""
    resp = client.post("/api/v1/clips", json={})
    assert resp.status_code == 422


def test_clips_endpoint_success(tmp_path):
    """POST /api/v1/clips should return clip segments."""
    from app.models.schemas import ClipSegment

    video_file = tmp_path / "test.mp4"
    video_file.write_bytes(b"fake video content")

    mock_clips = [
        ClipSegment(
            start_time=5.0,
            end_time=45.0,
            duration=40.0,
            viral_score=0.88,
            rationale="Great hook",
            hook_text="You won't believe this",
            suggested_title="Amazing Clip",
            hashtags=["viral"],
            suggested_for=["tiktok"],
        )
    ]

    with (
        patch("app.services.clip_service.identify_viral_segments", new=AsyncMock(return_value=mock_clips)),
        patch("app.services.transcript_service.transcribe_video", new=AsyncMock(return_value={
            "language": "en", "duration": 60.0, "segments": [], "full_text": "test", "processing_time": 1.0
        })),
        patch("app.utils.ffmpeg_utils._validate_storage_path", return_value=str(video_file)),
    ):
        from main import app
        with TestClient(app) as c:
            resp = c.post("/api/v1/clips", json={
                "video_id": "vid-test",
                "storage_path": "test.mp4",
            })

    assert resp.status_code == 200
    body = resp.json()
    assert body["video_id"] == "vid-test"
    assert isinstance(body["clips"], list)
    assert len(body["clips"]) == 1


# ---- Hooks endpoint ----

def test_hooks_endpoint_success():
    """POST /api/v1/hooks should return generated hooks."""
    mock_hooks = [
        {"text": "Amazing hook", "type": "statement", "viral_score": 0.9, "rationale": "Strong opening"},
    ]

    with patch("app.services.hook_service.generate_hooks", new=AsyncMock(return_value=mock_hooks)):
        from main import app
        with TestClient(app) as c:
            resp = c.post("/api/v1/hooks", json={
                "video_id": "vid-hook-test",
                "transcript": "This is an amazing story about success.",
                "count": 1,
            })

    assert resp.status_code == 200
    body = resp.json()
    assert body["video_id"] == "vid-hook-test"
    assert isinstance(body["hooks"], list)


def test_hooks_endpoint_missing_transcript(client):
    """POST /api/v1/hooks without transcript should return 422."""
    resp = client.post("/api/v1/hooks", json={"video_id": "vid-123"})
    assert resp.status_code == 422


# ---- Metadata endpoint ----

def test_metadata_endpoint_success():
    """POST /api/v1/metadata should return generated metadata."""
    mock_meta = {
        "title": "Test Title",
        "description": "Test description",
        "hashtags": ["fitness", "health"],
        "keywords": ["workout"],
        "category": "Health",
        "optimal_post_times": ["7PM EST"],
    }

    with patch("app.services.metadata_service.generate_metadata", new=AsyncMock(return_value=mock_meta)):
        from main import app
        with TestClient(app) as c:
            resp = c.post("/api/v1/metadata", json={
                "video_id": "vid-meta-test",
                "transcript": "Great workout tips.",
                "platform": "tiktok",
            })

    assert resp.status_code == 200
    body = resp.json()
    assert body["video_id"] == "vid-meta-test"
    assert body["title"] == "Test Title"


def test_metadata_endpoint_missing_platform(client):
    """POST /api/v1/metadata without platform should return 422."""
    resp = client.post("/api/v1/metadata", json={
        "video_id": "vid-123",
        "transcript": "test",
    })
    assert resp.status_code == 422
