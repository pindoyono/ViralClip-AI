from fastapi import APIRouter, HTTPException
from loguru import logger
from app.models.schemas import TranscriptRequest, TranscriptResponse
from app.services.transcript_service import transcribe_video
import os

router = APIRouter(prefix="/transcript", tags=["transcript"])


@router.post("", response_model=TranscriptResponse)
async def transcribe(request: TranscriptRequest):
    """Transcribe a video file and return timestamped segments."""
    if not os.path.exists(request.storage_path):
        raise HTTPException(status_code=404, detail=f"Video file not found: {request.storage_path}")

    try:
        result = await transcribe_video(request.storage_path, language=request.language)
        return TranscriptResponse(
            video_id=request.video_id,
            language=result["language"],
            duration=result["duration"],
            segments=result["segments"],
            full_text=result["full_text"],
        )
    except Exception as e:
        logger.error(f"Transcription failed for {request.video_id}: {e}")
        raise HTTPException(status_code=500, detail=str(e))
