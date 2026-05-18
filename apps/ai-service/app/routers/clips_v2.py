"""Clip Engine V2 router.

Exposes:

    POST /api/v1/clips/v2/generate

Accepts a transcript (as pre-segmented list) plus optional hook detections
and a content profile type, then returns clip candidates scored by the V2
composite formula:

    ClipScore = Hook×50% + Emotion×20% + Story×20% + Retention×10%
"""

from fastapi import APIRouter, HTTPException
from loguru import logger

from app.models.schemas import ClipGenerateV2Request, ClipGenerateV2Response, ClipV2ResultSchema
from app.services.clip_engine_v2 import build_clip_engine_v2

router = APIRouter(prefix="/clips/v2", tags=["clips-v2"])

# Shared stateless engine instance (thread-safe)
_engine = build_clip_engine_v2()


@router.post("/generate", response_model=ClipGenerateV2Response)
async def generate_clips_v2(request: ClipGenerateV2Request) -> ClipGenerateV2Response:
    """Generate clip candidates using the V2 Dynamic Clip Engine.

    Analyses transcript segments with hook scores, emotion signals, story arc
    detection, and retention prediction to produce profile-aware clip windows.

    **Profile duration rules:**
    - gaming: 15–45 s
    - comedy: 10–30 s
    - education: 30–60 s
    - politics: 30–90 s
    - podcast: 45–90 s
    - general: 15–90 s (default)
    """
    logger.info(
        "ClipV2 generate: video_id={} profile={} segments={} hooks={} max_clips={}",
        request.video_id,
        request.profile_type,
        len(request.segments),
        len(request.hook_detections),
        request.max_clips,
    )

    try:
        clips = await _engine.generate_clips(
            segments=request.segments,
            hook_detections=request.hook_detections,
            profile_type=request.profile_type.value,
            min_clip_score=request.min_clip_score,
            max_clips=request.max_clips,
        )
    except Exception as exc:
        logger.error("ClipV2 generation failed for {}: {}", request.video_id, exc)
        raise HTTPException(status_code=500, detail=str(exc)) from exc

    result_clips = [
        ClipV2ResultSchema(
            start=c.start,
            end=c.end,
            start_seconds=c.start_seconds,
            end_seconds=c.end_seconds,
            score=c.score,
            hook_score=c.hook_score,
            emotion_score=c.emotion_score,
            story_score=c.story_score,
            retention_score=c.retention_score,
            profile_type=c.profile_type,
        )
        for c in clips
    ]

    return ClipGenerateV2Response(
        video_id=request.video_id,
        profile_type=request.profile_type.value,
        clips=result_clips,
        total=len(result_clips),
    )
