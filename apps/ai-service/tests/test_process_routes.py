"""Tests for the /process/* pipeline endpoints."""
import json
import os
import pytest
from unittest.mock import AsyncMock, MagicMock, patch

from fastapi.testclient import TestClient


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _make_mock_transcript_result(full_text: str = "Hello world") -> dict:
    return {
        "language": "en",
        "duration": 60.0,
        "segments": [],
        "full_text": full_text,
        "processing_time": 1.0,
    }


def _make_mock_clip_segment():
    from app.models.schemas import ClipSegment
    return ClipSegment(
        start_time=5.0,
        end_time=35.0,
        duration=30.0,
        viral_score=0.88,
        rationale="Great hook",
        hook_text="You won't believe this",
        suggested_title="Amazing Clip",
        hashtags=["#viral"],
        suggested_for=["tiktok"],
    )


# ---------------------------------------------------------------------------
# POST /process/transcript
# ---------------------------------------------------------------------------

class TestProcessTranscript:
    def test_missing_video_file_returns_404(self, tmp_path):
        """Non-existent storage_path should return 404."""
        with (
            patch("app.routers.process._validate_storage_path", return_value=str(tmp_path / "missing.mp4")),
        ):
            from main import app
            with TestClient(app) as c:
                resp = c.post("/process/transcript", json={
                    "video_id": "vid-1",
                    "storage_path": "missing.mp4",
                })
        assert resp.status_code == 404

    def test_path_traversal_returns_400(self):
        """Path traversal attempt should return 400."""
        from main import app
        with TestClient(app) as c:
            resp = c.post("/process/transcript", json={
                "video_id": "vid-1",
                "storage_path": "../../etc/passwd",
            })
        assert resp.status_code == 400

    def test_success_returns_transcript_and_caches(self, tmp_path):
        """Successful transcription should return transcript and cache to disk."""
        video_file = tmp_path / "test.mp4"
        video_file.write_bytes(b"fake video content")
        transcript_dir = tmp_path / "transcripts"
        transcript_dir.mkdir()

        mock_result = _make_mock_transcript_result("Test transcript text")

        with (
            patch("app.routers.process.transcribe_video", new=AsyncMock(return_value=mock_result)),
            patch("app.routers.process._validate_storage_path", return_value=str(video_file)),
            patch("app.routers.process._transcript_cache_path", return_value=str(tmp_path / "transcripts" / "vid-t1.json")),
        ):
            from main import app
            with TestClient(app) as c:
                resp = c.post("/process/transcript", json={
                    "video_id": "vid-t1",
                    "storage_path": "test.mp4",
                })

        assert resp.status_code == 200
        body = resp.json()
        assert body["video_id"] == "vid-t1"
        assert body["language"] == "en"
        assert body["duration"] == 60.0
        assert body["full_text"] == "Test transcript text"
        assert "transcript_path" in body

    def test_missing_video_id_returns_422(self):
        """Missing required field should return 422."""
        from main import app
        with TestClient(app) as c:
            resp = c.post("/process/transcript", json={"storage_path": "test.mp4"})
        assert resp.status_code == 422


# ---------------------------------------------------------------------------
# POST /process/clips
# ---------------------------------------------------------------------------

class TestProcessClips:
    def test_missing_video_file_returns_404(self, tmp_path):
        """Non-existent storage_path should return 404."""
        with (
            patch("app.routers.process._validate_storage_path", return_value=str(tmp_path / "missing.mp4")),
        ):
            from main import app
            with TestClient(app) as c:
                resp = c.post("/process/clips", json={
                    "video_id": "vid-2",
                    "storage_path": "missing.mp4",
                })
        assert resp.status_code == 404

    def test_path_traversal_returns_400(self):
        """Path traversal should return 400."""
        from main import app
        with TestClient(app) as c:
            resp = c.post("/process/clips", json={
                "video_id": "vid-2",
                "storage_path": "../../../etc/passwd",
            })
        assert resp.status_code == 400

    def test_success_with_cached_transcript(self, tmp_path):
        """Should use cached transcript and return extracted clips."""
        video_file = tmp_path / "test.mp4"
        video_file.write_bytes(b"fake video content")

        # Write a fake transcript cache
        transcript_dir = tmp_path / "transcripts"
        transcript_dir.mkdir()
        cache_file = transcript_dir / "vid-c1.json"
        cache_data = {
            "video_id": "vid-c1",
            "language": "en",
            "duration": 60.0,
            "full_text": "Test",
            "segments": [{"start": 0.0, "end": 30.0, "text": "Hello", "confidence": 1.0}],
        }
        cache_file.write_text(json.dumps(cache_data))

        clip_seg = _make_mock_clip_segment()
        fake_clip_path = str(tmp_path / "clips" / "vid-c1" / "clip_000.mp4")

        with (
            patch("app.routers.process._validate_storage_path", return_value=str(video_file)),
            patch("app.routers.process._transcript_cache_path", return_value=str(cache_file)),
            patch("app.routers.process._clips_dir", return_value=str(tmp_path / "clips" / "vid-c1")),
            patch("app.routers.process._manifest_path", return_value=str(tmp_path / "clips" / "vid-c1_manifest.json")),
            patch("app.routers.process.identify_viral_segments", new=AsyncMock(return_value=[clip_seg])),
            patch("app.routers.process.extract_clip", return_value=fake_clip_path),
        ):
            os.makedirs(str(tmp_path / "clips" / "vid-c1"), exist_ok=True)
            from main import app
            with TestClient(app) as c:
                resp = c.post("/process/clips", json={
                    "video_id": "vid-c1",
                    "storage_path": "test.mp4",
                })

        assert resp.status_code == 200
        body = resp.json()
        assert body["video_id"] == "vid-c1"
        assert isinstance(body["clips"], list)
        assert len(body["clips"]) == 1
        assert body["clips"][0]["viral_score"] == pytest.approx(0.88)
        assert "manifest_path" in body

    def test_missing_video_id_returns_422(self):
        """Missing required field should return 422."""
        from main import app
        with TestClient(app) as c:
            resp = c.post("/process/clips", json={"storage_path": "test.mp4"})
        assert resp.status_code == 422


# ---------------------------------------------------------------------------
# POST /process/subtitles
# ---------------------------------------------------------------------------

class TestProcessSubtitles:
    def test_missing_manifest_returns_404(self, tmp_path):
        """No clip manifest on disk should return 404."""
        with (
            patch("app.routers.process._manifest_path", return_value=str(tmp_path / "nonexistent.json")),
        ):
            from main import app
            with TestClient(app) as c:
                resp = c.post("/process/subtitles", json={
                    "video_id": "vid-3",
                    "storage_path": "test.mp4",
                })
        assert resp.status_code == 404

    def test_success_with_manifest(self, tmp_path):
        """Should burn subtitles for each clip in the manifest."""
        # Write manifest
        clips_dir = tmp_path / "clips"
        clips_dir.mkdir()
        clip_file = clips_dir / "clip_000.mp4"
        clip_file.write_bytes(b"fake clip")

        manifest = [
            {
                "index": 0,
                "storage_path": str(clip_file),
                "start_time": 0.0,
                "end_time": 30.0,
                "duration": 30.0,
                "viral_score": 0.8,
                "rationale": "good",
                "hook_text": "hook",
                "suggested_title": "title",
                "hashtags": [],
                "suggested_for": ["tiktok"],
            }
        ]
        manifest_file = tmp_path / "manifest.json"
        manifest_file.write_text(json.dumps(manifest))

        # Write transcript cache with a segment that overlaps
        transcript_cache = tmp_path / "transcript.json"
        transcript_cache.write_text(json.dumps({
            "segments": [{"start": 5.0, "end": 20.0, "text": "Hello world", "confidence": 1.0}]
        }))

        with (
            patch("app.routers.process._manifest_path", return_value=str(manifest_file)),
            patch("app.routers.process._transcript_cache_path", return_value=str(transcript_cache)),
            patch("app.routers.process.burn_subtitles", new=AsyncMock(return_value={"output_path": str(clip_file), "subtitle_path": "sub.srt"})),
        ):
            from main import app
            with TestClient(app) as c:
                resp = c.post("/process/subtitles", json={
                    "video_id": "vid-3",
                    "storage_path": "test.mp4",
                })

        assert resp.status_code == 200
        body = resp.json()
        assert body["video_id"] == "vid-3"
        assert body["clips_processed"] == 1

    def test_missing_video_id_returns_422(self):
        """Missing required field should return 422."""
        from main import app
        with TestClient(app) as c:
            resp = c.post("/process/subtitles", json={"storage_path": "test.mp4"})
        assert resp.status_code == 422


# ---------------------------------------------------------------------------
# POST /process/video
# ---------------------------------------------------------------------------

class TestProcessVideo:
    def test_missing_video_file_returns_404(self, tmp_path):
        """Non-existent storage_path should return 404."""
        with (
            patch("app.routers.process._validate_storage_path", return_value=str(tmp_path / "missing.mp4")),
        ):
            from main import app
            with TestClient(app) as c:
                resp = c.post("/process/video", json={
                    "video_id": "vid-4",
                    "storage_path": "missing.mp4",
                })
        assert resp.status_code == 404

    def test_path_traversal_returns_400(self):
        """Path traversal should return 400."""
        from main import app
        with TestClient(app) as c:
            resp = c.post("/process/video", json={
                "video_id": "vid-4",
                "storage_path": "../../etc/passwd",
            })
        assert resp.status_code == 400

    def test_success_returns_video_metadata(self, tmp_path):
        """Should return video metadata and generate thumbnail."""
        video_file = tmp_path / "test.mp4"
        video_file.write_bytes(b"fake video content")
        thumb_path = str(tmp_path / "thumbnails" / "vid-v1.jpg")

        mock_info = {
            "duration": 120.5,
            "width": 1920,
            "height": 1080,
            "fps": 30.0,
            "has_audio": True,
            "size": 1024000,
            "bitrate": 2000000,
            "video_codec": "h264",
            "audio_codec": "aac",
        }

        with (
            patch("app.routers.process._validate_storage_path", return_value=str(video_file)),
            patch("app.routers.process.get_video_info", return_value=mock_info),
            patch("app.routers.process.generate_thumbnail", return_value=thumb_path),
            patch("app.routers.process._thumbnail_path", return_value=thumb_path),
        ):
            from main import app
            with TestClient(app) as c:
                resp = c.post("/process/video", json={
                    "video_id": "vid-v1",
                    "storage_path": "test.mp4",
                })

        assert resp.status_code == 200
        body = resp.json()
        assert body["video_id"] == "vid-v1"
        assert body["duration"] == pytest.approx(120.5)
        assert body["width"] == 1920
        assert body["height"] == 1080
        assert body["fps"] == pytest.approx(30.0)
        assert body["has_audio"] is True
        assert body["thumbnail_path"] == thumb_path

    def test_missing_video_id_returns_422(self):
        """Missing required field should return 422."""
        from main import app
        with TestClient(app) as c:
            resp = c.post("/process/video", json={"storage_path": "test.mp4"})
        assert resp.status_code == 422
