"""Hook Detection Engine V2 router.

Exposes a single endpoint:

    POST /api/v1/hooks/v2/detect

which accepts a list of transcript segments (with timestamps) and returns
detected hook moments classified by type with a 0–100 strength score.
"""

from fastapi import APIRouter, HTTPException
from loguru import logger

from app.models.schemas import HookDetectionRequest, HookDetectionResponse
from app.services.hook_engine_v2 import build_hook_engine_v2

router = APIRouter(prefix="/hooks/v2", tags=["hooks-v2"])

# One shared engine instance per process (stateless, thread-safe)
_engine = build_hook_engine_v2()


@router.post("/detect", response_model=HookDetectionResponse)
async def detect_hooks_v2(request: HookDetectionRequest) -> HookDetectionResponse:
    """Detect and score hook moments in a transcript.

    Analyses each transcript segment for five hook categories:
    curiosity, emotion, storytelling, controversy, and CTA.

    Scores incorporate sentence patterns, position in the transcript,
    emphasis words, speech pauses, and repeated words.

    Returns only segments whose score meets `min_score` (default 50).
    """
    logger.info(
        "HookV2 detect: video_id={} segments={} min_score={}",
        request.video_id, len(request.segments), request.min_score,
    )

    try:
        hooks = await _engine.detect_hooks(
            segments=request.segments,
            min_score=request.min_score,
        )
    except Exception as exc:
        logger.error("HookV2 detection failed for {}: {}", request.video_id, exc)
        raise HTTPException(status_code=500, detail=str(exc)) from exc

    return HookDetectionResponse(
        video_id=request.video_id,
        hooks=hooks,
        total=len(hooks),
    )
