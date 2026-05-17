from fastapi import APIRouter, HTTPException
from loguru import logger
from app.models.schemas import SubtitleRequest, SubtitleResponse
from app.services.subtitle_service import burn_subtitles
import os

router = APIRouter(prefix="/subtitles", tags=["subtitles"])


@router.post("", response_model=SubtitleResponse)
async def add_subtitles(request: SubtitleRequest):
    """Burn subtitles into a video clip."""
    if not os.path.exists(request.clip_storage_path):
        raise HTTPException(status_code=404, detail=f"Clip file not found: {request.clip_storage_path}")
    if not request.transcript_segments:
        raise HTTPException(status_code=400, detail="Transcript segments are required")

    try:
        result = await burn_subtitles(
            clip_path=request.clip_storage_path,
            transcript_segments=request.transcript_segments,
            style=request.style,
            font_size=request.font_size,
            primary_color=request.primary_color,
            outline_color=request.outline_color,
        )
        return SubtitleResponse(
            video_id=request.video_id,
            output_path=result["output_path"],
            subtitle_path=result["subtitle_path"],
        )
    except Exception as e:
        logger.error(f"Subtitle burning failed for {request.video_id}: {e}")
        raise HTTPException(status_code=500, detail=str(e))
