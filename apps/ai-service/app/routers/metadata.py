from fastapi import APIRouter, HTTPException
from loguru import logger
from app.models.schemas import MetadataRequest, MetadataResponse
from app.services.metadata_service import generate_metadata

router = APIRouter(prefix="/metadata", tags=["metadata"])


@router.post("", response_model=MetadataResponse)
async def create_metadata(request: MetadataRequest):
    """Generate platform-optimized metadata for a clip."""
    if not request.transcript.strip():
        raise HTTPException(status_code=400, detail="Transcript is required")

    try:
        result = await generate_metadata(
            transcript=request.transcript,
            platform=request.platform,
            niche=request.niche,
            tone=request.tone,
        )
        return MetadataResponse(video_id=request.video_id, **result)
    except Exception as e:
        logger.error(f"Metadata generation failed for {request.video_id}: {e}")
        raise HTTPException(status_code=500, detail=str(e))
