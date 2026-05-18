"""Audio-Aware Hook Detection Engine V3 router.

Exposes a single endpoint:

    POST /api/v1/hooks/v3/detect

which accepts a list of transcript segments (with timestamps) and an optional
path to an audio/video file, then returns hook moments whose scores have been
enriched by audio signals (speech pauses, voice intensity, speech speed, and
audio emotion).

When *audio_storage_path* is omitted the endpoint falls back to text-only
analysis that is functionally identical to the V2 engine.
"""

from __future__ import annotations

from typing import List, Optional

from fastapi import APIRouter, HTTPException
from loguru import logger

from app.models.schemas import (
    AudioAwareHookDetectionRequest,
    AudioAwareHookDetectionResponse,
    AudioEmotionLabel,
    HookDetectionResult,
    IntensityLevel,
    IntensitySignal,
    PauseSignal,
    PauseType,
    SegmentAudioAnalysis,
    SpeechPatternSignal,
    SpeechRate,
    TranscriptSegmentInput,
)
from app.services.audio_aware_hook_engine import build_audio_aware_hook_engine
from app.services.audio_signal_analyzer import SegmentAudioSignals

router = APIRouter(prefix="/hooks/v3", tags=["hooks-v3"])

# One shared engine instance per process (stateless, thread-safe)
_engine = build_audio_aware_hook_engine()


def _to_segment_audio_analysis(s: SegmentAudioSignals) -> SegmentAudioAnalysis:
    """Convert an internal SegmentAudioSignals dataclass to the public schema."""
    pre_pause: Optional[PauseSignal] = None
    if s.pre_pause is not None:
        pre_pause = PauseSignal(
            start=s.pre_pause.start,
            end=s.pre_pause.end,
            duration=s.pre_pause.duration,
            pause_type=PauseType(s.pre_pause.pause_type),
        )

    intensity = IntensitySignal(
        rms_db=s.intensity.rms_db,
        rms_relative=s.intensity.rms_relative,
        intensity_level=IntensityLevel(s.intensity.intensity_level),
        has_sudden_increase=s.intensity.has_sudden_increase,
        is_emotional=s.intensity.is_emotional,
    )

    speech_pattern = SpeechPatternSignal(
        words_per_second=s.speech_pattern.words_per_second,
        speech_rate=SpeechRate(s.speech_pattern.speech_rate),
        rate_deviation=s.speech_pattern.rate_deviation,
    )

    return SegmentAudioAnalysis(
        start=s.start,
        end=s.end,
        pre_pause=pre_pause,
        intensity=intensity,
        speech_pattern=speech_pattern,
        audio_emotion=AudioEmotionLabel(s.audio_emotion),
        audio_score=s.audio_score,
        audio_hook_type=s.audio_hook_type,
    )


@router.post("/detect", response_model=AudioAwareHookDetectionResponse)
async def detect_hooks_v3(request: AudioAwareHookDetectionRequest) -> AudioAwareHookDetectionResponse:
    """Detect and score hook moments using transcript text and audio signals.

    Analyses each transcript segment for five hook categories:
    curiosity, emotion, storytelling, controversy, and CTA.

    When *audio_storage_path* is provided, scores are additionally boosted by:
    - Speech pauses (dramatic, long, emphasis) before each segment
    - Voice intensity (loud delivery, sudden volume increases, emotional tone)
    - Speech rate (fast/slow/sudden speed changes)
    - Inferred audio emotion (excitement, anger, surprise, sadness)

    Returns only segments whose combined score meets `min_score` (default 50).
    """
    logger.info(
        "HookV3 detect: video_id={} segments={} audio={} min_score={}",
        request.video_id,
        len(request.segments),
        request.audio_storage_path or "none",
        request.min_score,
    )

    try:
        hooks = await _engine.detect_hooks(
            segments=request.segments,
            audio_path=request.audio_storage_path,
            min_score=request.min_score,
        )
    except Exception as exc:
        logger.error("HookV3 detection failed for {}: {}", request.video_id, exc)
        raise HTTPException(status_code=500, detail=str(exc)) from exc

    # Collect audio analysis for the response (if audio was used)
    audio_analysis: List[SegmentAudioAnalysis] = []
    audio_enabled = False

    if request.audio_storage_path and _engine._audio_analyzer is not None:
        try:
            raw_signals = _engine._audio_analyzer.analyze(
                request.segments, request.audio_storage_path
            )
            if raw_signals:
                audio_enabled = True
                audio_analysis = [_to_segment_audio_analysis(s) for s in raw_signals]
        except Exception as exc:
            # Non-fatal: log and continue without audio analysis detail
            logger.warning("HookV3: could not collect audio analysis detail: {}", exc)

    return AudioAwareHookDetectionResponse(
        video_id=request.video_id,
        hooks=hooks,
        total=len(hooks),
        audio_analysis=audio_analysis,
        audio_enabled=audio_enabled,
    )
