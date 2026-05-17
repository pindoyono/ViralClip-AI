import os
import time
import whisper
from typing import Optional, List, Dict, Any
from loguru import logger
from app.config import settings
from app.models.schemas import TranscriptSegment
from app.utils.ffmpeg_utils import extract_audio, get_video_info
from app.utils.file_utils import generate_temp_path, clean_temp_file

_whisper_model = None


def _get_model():
    global _whisper_model
    if _whisper_model is None:
        logger.info(f"Loading Whisper model: {settings.whisper_model}")
        _whisper_model = whisper.load_model(settings.whisper_model, device=settings.whisper_device)
        logger.info("Whisper model loaded")
    return _whisper_model


async def transcribe_video(
    video_path: str,
    language: Optional[str] = None,
) -> Dict[str, Any]:
    """Transcribe a video file using Whisper."""
    start_time = time.time()
    audio_path = None

    try:
        # Extract audio track
        audio_path = generate_temp_path(settings.local_storage_path + "/tmp", ".wav")
        extract_audio(video_path, audio_path)

        # Get video duration
        info = get_video_info(video_path)
        duration = info["duration"]

        # Transcribe with Whisper
        model = _get_model()
        logger.info(f"Transcribing video: {video_path}")

        transcribe_kwargs = {"verbose": False, "task": "transcribe"}
        if language:
            transcribe_kwargs["language"] = language

        result = model.transcribe(audio_path, **transcribe_kwargs)

        segments: List[TranscriptSegment] = []
        for seg in result.get("segments", []):
            segments.append(
                TranscriptSegment(
                    start=round(seg["start"], 3),
                    end=round(seg["end"], 3),
                    text=seg["text"].strip(),
                    confidence=round(seg.get("no_speech_prob", 1.0), 4),
                )
            )

        elapsed = round(time.time() - start_time, 2)
        logger.info(f"Transcription completed in {elapsed}s, {len(segments)} segments")

        return {
            "language": result.get("language", "en"),
            "duration": duration,
            "segments": segments,
            "full_text": result.get("text", "").strip(),
            "processing_time": elapsed,
        }

    finally:
        clean_temp_file(audio_path)
