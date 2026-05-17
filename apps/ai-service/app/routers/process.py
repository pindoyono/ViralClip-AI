"""Pipeline processing endpoints consumed by the Go worker queue consumers.

Each endpoint corresponds to one stage in the video processing pipeline:
  1. /process/transcript  — transcribe + cache result
  2. /process/clips       — identify viral segments, extract clip files, return manifest
  3. /process/subtitles   — burn subtitles into every extracted clip
  4. /process/video       — probe video metadata and generate a thumbnail
"""
import json
import os
import time

from fastapi import APIRouter, HTTPException
from loguru import logger

from app.config import settings
from app.models.schemas import (
    ExtractedClip,
    ProcessClipsRequest,
    ProcessClipsResponse,
    ProcessSubtitlesRequest,
    ProcessSubtitlesResponse,
    ProcessTranscriptRequest,
    ProcessTranscriptResponse,
    ProcessVideoRequest,
    ProcessVideoResponse,
    SubtitleStyle,
    TranscriptSegment,
)
from app.services.clip_service import identify_viral_segments
from app.services.subtitle_service import burn_subtitles
from app.services.transcript_service import transcribe_video
from app.utils.ffmpeg_utils import (
    _validate_storage_path,
    extract_clip,
    generate_thumbnail,
    get_video_info,
)
from app.utils.file_utils import ensure_dir

router = APIRouter(prefix="/process", tags=["process"])

# Paths are relative to local_storage_path
_TRANSCRIPTS_SUBDIR = "transcripts"
_CLIPS_SUBDIR = "clips"
_THUMBNAILS_SUBDIR = "thumbnails"


def _transcript_cache_path(video_id: str) -> str:
    d = os.path.join(settings.local_storage_path, _TRANSCRIPTS_SUBDIR)
    ensure_dir(d)
    return os.path.join(d, f"{video_id}.json")


def _clips_dir(video_id: str) -> str:
    d = os.path.join(settings.local_storage_path, _CLIPS_SUBDIR, video_id)
    ensure_dir(d)
    return d


def _manifest_path(video_id: str) -> str:
    d = os.path.join(settings.local_storage_path, _CLIPS_SUBDIR)
    ensure_dir(d)
    return os.path.join(d, f"{video_id}_manifest.json")


def _thumbnail_path(video_id: str) -> str:
    d = os.path.join(settings.local_storage_path, _THUMBNAILS_SUBDIR)
    ensure_dir(d)
    return os.path.join(d, f"{video_id}.jpg")


# ---------------------------------------------------------------------------
# POST /process/transcript
# ---------------------------------------------------------------------------

@router.post("/transcript", response_model=ProcessTranscriptResponse)
async def process_transcript(request: ProcessTranscriptRequest):
    """Transcribe a video and cache the result to disk for downstream steps."""
    try:
        safe_path = _validate_storage_path(request.storage_path, settings.local_storage_path)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    if not os.path.exists(safe_path):
        raise HTTPException(status_code=404, detail=f"Video file not found: {safe_path}")

    try:
        result = await transcribe_video(safe_path, language=request.language)
    except Exception as e:
        logger.error(f"Transcription failed for {request.video_id}: {e}")
        raise HTTPException(status_code=500, detail=str(e))

    # Cache transcript to disk so /process/clips can read it without re-running Whisper.
    cache_path = _transcript_cache_path(request.video_id)
    try:
        cache_data = {
            "video_id": request.video_id,
            "language": result["language"],
            "duration": result["duration"],
            "full_text": result["full_text"],
            "segments": [s.model_dump() for s in result["segments"]],
        }
        with open(cache_path, "w", encoding="utf-8") as f:
            json.dump(cache_data, f)
        logger.info(f"Transcript cached at {cache_path}")
    except Exception as e:
        logger.warning(f"Failed to cache transcript for {request.video_id}: {e}")

    return ProcessTranscriptResponse(
        video_id=request.video_id,
        language=result["language"],
        duration=result["duration"],
        segments=result["segments"],
        full_text=result["full_text"],
        transcript_path=cache_path,
    )


# ---------------------------------------------------------------------------
# POST /process/clips
# ---------------------------------------------------------------------------

@router.post("/clips", response_model=ProcessClipsResponse)
async def process_clips(request: ProcessClipsRequest):
    """Identify viral segments and extract clip files from the source video."""
    try:
        safe_path = _validate_storage_path(request.storage_path, settings.local_storage_path)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    if not os.path.exists(safe_path):
        raise HTTPException(status_code=404, detail=f"Video file not found: {safe_path}")

    start = time.time()

    # Load transcript from cache; re-transcribe if missing.
    segments: list[TranscriptSegment] = []
    cache_path = _transcript_cache_path(request.video_id)
    if os.path.exists(cache_path):
        try:
            with open(cache_path, encoding="utf-8") as f:
                cached = json.load(f)
            segments = [TranscriptSegment(**s) for s in cached.get("segments", [])]
            logger.info(f"Loaded {len(segments)} cached segments for {request.video_id}")
        except Exception as e:
            logger.warning(f"Failed to read transcript cache: {e}; re-transcribing")

    if not segments:
        logger.info(f"No cached transcript; transcribing {request.video_id}")
        try:
            transcript_result = await transcribe_video(safe_path)
            segments = transcript_result["segments"]
        except Exception as e:
            logger.error(f"Transcription failed for {request.video_id}: {e}")
            raise HTTPException(status_code=500, detail=str(e))

    # Identify viral segments with GPT-4.
    try:
        clip_segments = await identify_viral_segments(
            transcript_segments=segments,
            content_profile=request.content_profile,
            max_clips=request.max_clips,
            min_duration=request.min_duration,
            max_duration=request.max_duration,
        )
    except Exception as e:
        logger.error(f"Clip identification failed for {request.video_id}: {e}")
        raise HTTPException(status_code=500, detail=str(e))

    # Extract each clip as an individual mp4 file.
    clips_output_dir = _clips_dir(request.video_id)
    extracted: list[ExtractedClip] = []

    for i, seg in enumerate(clip_segments):
        clip_filename = f"clip_{i:03d}.mp4"
        clip_path = os.path.join(clips_output_dir, clip_filename)
        try:
            extract_clip(safe_path, clip_path, seg.start_time, seg.end_time)
        except Exception as e:
            logger.warning(f"Failed to extract clip {i} for {request.video_id}: {e}; skipping")
            continue

        extracted.append(
            ExtractedClip(
                index=i,
                storage_path=clip_path,
                start_time=seg.start_time,
                end_time=seg.end_time,
                duration=seg.duration,
                viral_score=seg.viral_score,
                rationale=seg.rationale,
                hook_text=seg.hook_text,
                suggested_title=seg.suggested_title,
                hashtags=seg.hashtags,
                suggested_for=seg.suggested_for,
            )
        )

    # Persist manifest for the subtitle step.
    manifest_path = _manifest_path(request.video_id)
    try:
        manifest_data = [c.model_dump() for c in extracted]
        with open(manifest_path, "w", encoding="utf-8") as f:
            json.dump(manifest_data, f)
        logger.info(f"Clip manifest saved: {manifest_path} ({len(extracted)} clips)")
    except Exception as e:
        logger.warning(f"Failed to save clip manifest for {request.video_id}: {e}")

    processing_time = round(time.time() - start, 2)
    logger.info(f"process/clips done for {request.video_id}: {len(extracted)} clips in {processing_time}s")

    return ProcessClipsResponse(
        video_id=request.video_id,
        clips=extracted,
        manifest_path=manifest_path,
        processing_time=processing_time,
    )


# ---------------------------------------------------------------------------
# POST /process/subtitles
# ---------------------------------------------------------------------------

@router.post("/subtitles", response_model=ProcessSubtitlesResponse)
async def process_subtitles(request: ProcessSubtitlesRequest):
    """Burn subtitles into every clip listed in the video's clip manifest."""
    # Load clip manifest.
    manifest_path = _manifest_path(request.video_id)
    if not os.path.exists(manifest_path):
        raise HTTPException(
            status_code=404,
            detail=f"Clip manifest not found for video {request.video_id}; run /process/clips first",
        )

    try:
        with open(manifest_path, encoding="utf-8") as f:
            manifest: list[dict] = json.load(f)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to read clip manifest: {e}")

    # Load transcript segments for subtitle timing.
    cache_path = _transcript_cache_path(request.video_id)
    transcript_segments: list[TranscriptSegment] = []
    if os.path.exists(cache_path):
        try:
            with open(cache_path, encoding="utf-8") as f:
                cached = json.load(f)
            transcript_segments = [TranscriptSegment(**s) for s in cached.get("segments", [])]
        except Exception as e:
            logger.warning(f"Failed to load transcript for subtitle step: {e}")

    processed = 0
    for clip_data in manifest:
        clip_path = clip_data.get("storage_path", "")
        if not clip_path or not os.path.exists(clip_path):
            logger.warning(f"Clip file not found, skipping: {clip_path}")
            continue

        # Filter segments to the clip's time window.
        start_t = clip_data.get("start_time", 0.0)
        end_t = clip_data.get("end_time", 0.0)
        clip_segs = [
            TranscriptSegment(
                start=max(0.0, s.start - start_t),
                end=min(end_t - start_t, s.end - start_t),
                text=s.text,
                confidence=s.confidence,
            )
            for s in transcript_segments
            if s.start < end_t and s.end > start_t
        ]

        if not clip_segs:
            logger.debug(f"No subtitle segments for clip {clip_data.get('index')}; skipping")
            continue

        try:
            await burn_subtitles(
                clip_path=clip_path,
                transcript_segments=clip_segs,
                style=request.style,
                font_size=request.font_size,
                primary_color=request.primary_color,
                outline_color=request.outline_color,
            )
            processed += 1
        except Exception as e:
            logger.warning(f"Subtitle burn failed for clip {clip_data.get('index')}: {e}")

    logger.info(f"process/subtitles done for {request.video_id}: {processed}/{len(manifest)} clips processed")
    return ProcessSubtitlesResponse(video_id=request.video_id, clips_processed=processed)


# ---------------------------------------------------------------------------
# POST /process/video
# ---------------------------------------------------------------------------

@router.post("/video", response_model=ProcessVideoResponse)
async def process_video(request: ProcessVideoRequest):
    """Extract video metadata and generate a thumbnail for the source video."""
    try:
        safe_path = _validate_storage_path(request.storage_path, settings.local_storage_path)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    if not os.path.exists(safe_path):
        raise HTTPException(status_code=404, detail=f"Video file not found: {safe_path}")

    try:
        info = get_video_info(safe_path)
    except Exception as e:
        logger.error(f"Failed to probe video {request.video_id}: {e}")
        raise HTTPException(status_code=500, detail=str(e))

    thumb_path = _thumbnail_path(request.video_id)
    try:
        generate_thumbnail(safe_path, thumb_path)
        logger.info(f"Thumbnail generated: {thumb_path}")
    except Exception as e:
        logger.warning(f"Thumbnail generation failed for {request.video_id}: {e}")
        thumb_path = ""

    return ProcessVideoResponse(
        video_id=request.video_id,
        duration=info["duration"],
        width=info["width"],
        height=info["height"],
        fps=info["fps"],
        has_audio=info["has_audio"],
        thumbnail_path=thumb_path,
    )
