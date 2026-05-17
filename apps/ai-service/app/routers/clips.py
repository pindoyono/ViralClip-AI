import time
import os
from fastapi import APIRouter, HTTPException
from loguru import logger
from app.models.schemas import ClipGenerationRequest, ClipGenerationResponse
from app.services.clip_service import identify_viral_segments
from app.services.transcript_service import transcribe_video
from app.utils.ffmpeg_utils import _validate_storage_path
from app.config import settings

router = APIRouter(prefix="/clips", tags=["clips"])


@router.post("", response_model=ClipGenerationResponse)
async def generate_clips(request: ClipGenerationRequest):
    """Identify viral clip segments from a video using AI."""
    try:
        safe_path = _validate_storage_path(request.storage_path, settings.local_storage_path)
    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))

    if not os.path.exists(safe_path):
        raise HTTPException(status_code=404, detail=f"Video file not found: {safe_path}")

    start = time.time()
    try:
        segments = request.segments
        if not segments:
            logger.info(f"No pre-computed segments; transcribing {request.video_id}")
            result = await transcribe_video(safe_path)
            segments = result["segments"]

        clips = await identify_viral_segments(
            transcript_segments=segments,
            content_profile=request.content_profile,
            max_clips=request.max_clips,
            min_duration=request.min_duration,
            max_duration=request.max_duration,
        )

        processing_time = round(time.time() - start, 2)
        return ClipGenerationResponse(
            video_id=request.video_id,
            clips=clips,
            processing_time=processing_time,
        )
    except Exception as e:
        logger.error(f"Clip generation failed for {request.video_id}: {e}")
        raise HTTPException(status_code=500, detail=str(e))
