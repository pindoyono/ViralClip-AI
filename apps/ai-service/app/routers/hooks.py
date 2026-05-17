from fastapi import APIRouter, HTTPException
from loguru import logger
from app.models.schemas import HookRequest, HookResponse
from app.services.hook_service import generate_hooks

router = APIRouter(prefix="/hooks", tags=["hooks"])


@router.post("", response_model=HookResponse)
async def create_hooks(request: HookRequest):
    """Generate viral hooks from a video transcript."""
    if not request.transcript.strip():
        raise HTTPException(status_code=400, detail="Transcript is required")

    try:
        hooks = await generate_hooks(
            transcript=request.transcript,
            niche=request.niche,
            platform=request.platform,
            tone=request.tone,
            count=request.count,
        )
        return HookResponse(video_id=request.video_id, hooks=hooks)
    except Exception as e:
        logger.error(f"Hook generation failed for {request.video_id}: {e}")
        raise HTTPException(status_code=500, detail=str(e))
