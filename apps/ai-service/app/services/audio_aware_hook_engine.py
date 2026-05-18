"""Audio-Aware Hook Engine.

Combines the text-based :class:`~app.services.hook_engine_v2.HookEngineV2`
with :class:`~app.services.audio_signal_analyzer.AudioSignalAnalyzer` to
produce richer hook detections that incorporate:

- Speech pauses (dramatic / long / emphasis)
- Voice intensity (loud, sudden increases, emotional delivery)
- Speech speed (fast, slow, sudden changes)
- Audio emotion (excitement, anger, surprise, sadness)

Scoring strategy
----------------
1.  Text-based score from HookEngineV2 (0–100).
2.  Audio boost from SegmentAudioSignals.audio_score (0–100), scaled to
    contribute up to +25 additional points: ``boost = audio_score * 0.25``.
3.  Combined score is clamped to [0, 100].

Audio-only hooks
----------------
Segments that do NOT trigger any text pattern but carry a strong audio signal
(audio_score ≥ 40) are surfaced with an audio-derived hook type and a score
proportional to the audio signal: ``score = int(audio_score * 0.85)``.

Fallback
--------
If *audio_path* is *None* or the audio cannot be loaded, the engine
transparently falls back to text-only analysis identical to HookEngineV2.
"""

from __future__ import annotations

from typing import Dict, List, Optional

from loguru import logger

from app.models.schemas import HookDetectionResult, TranscriptSegmentInput
from app.services.audio_signal_analyzer import AudioSignalAnalyzer, SegmentAudioSignals
from app.services.hook_engine_v2 import HookEngineV2
from app.services.hook_pattern_detector import HookPatternDetector
from app.services.hook_score_calculator import HookScoreCalculator


# Minimum audio score for a segment with no text hook to be surfaced
_AUDIO_ONLY_THRESHOLD: int = 40

# Audio score is scaled to contribute at most this many extra points
_AUDIO_BOOST_MAX: float = 25.0


class AudioAwareHookEngine:
    """Detect hook moments using both transcript text and audio signals.

    Parameters
    ----------
    text_engine:
        An instance of :class:`HookEngineV2` for transcript-based detection.
        Can be mocked in tests.
    audio_analyzer:
        An instance of :class:`AudioSignalAnalyzer` for audio-based signals.
        Can be mocked in tests.  Pass *None* to disable audio analysis.
    """

    def __init__(
        self,
        text_engine: HookEngineV2,
        audio_analyzer: Optional[AudioSignalAnalyzer],
    ) -> None:
        self._text_engine = text_engine
        self._audio_analyzer = audio_analyzer

    async def detect_hooks(
        self,
        segments: List[TranscriptSegmentInput],
        audio_path: Optional[str] = None,
        min_score: int = 50,
    ) -> List[HookDetectionResult]:
        """Detect and score hook moments, optionally augmented with audio.

        Parameters
        ----------
        segments:
            Ordered transcript segments with timestamps.
        audio_path:
            Absolute path to an audio or video file.  When *None* (or the
            file cannot be loaded) the method falls back to text-only
            analysis.
        min_score:
            Minimum score threshold (0–100).

        Returns
        -------
        List[HookDetectionResult]
            Hook detections sorted by descending score.
        """
        if not segments:
            logger.warning("AudioAwareHookEngine.detect_hooks called with empty segments")
            return []

        # Step 1 – text-based detection (async)
        text_hooks = await self._text_engine.detect_hooks(segments, min_score=0)

        # Index text hooks by start time for fast lookup
        text_hook_map: Dict[float, HookDetectionResult] = {
            h.start: h for h in text_hooks
        }

        # Step 2 – audio analysis (optional)
        audio_signals: List[SegmentAudioSignals] = []
        if audio_path and self._audio_analyzer is not None:
            audio_signals = self._audio_analyzer.analyze(segments, audio_path)
        else:
            if audio_path:
                logger.warning("AudioAwareHookEngine: no audio analyzer injected; text-only mode")

        if not audio_signals:
            # No audio – filter by min_score and return text hooks
            results = [h for h in text_hooks if h.score >= min_score]
            results.sort(key=lambda r: r.score, reverse=True)
            logger.info(
                "AudioAwareHookEngine (text-only): {} segments → {} hooks",
                len(segments), len(results),
            )
            return results

        # Step 3 – merge text and audio signals
        merged: List[HookDetectionResult] = []
        audio_signal_map: Dict[float, SegmentAudioSignals] = {
            s.start: s for s in audio_signals
        }

        # Process each segment
        for seg in segments:
            text_hook = text_hook_map.get(seg.start)
            audio_sig = audio_signal_map.get(seg.start)

            if text_hook is not None and audio_sig is not None:
                # Boost existing text hook with audio
                boost = round(audio_sig.audio_score * (_AUDIO_BOOST_MAX / 100.0))
                new_score = min(100, text_hook.score + boost)
                merged.append(
                    HookDetectionResult(
                        start=text_hook.start,
                        end=text_hook.end,
                        type=text_hook.type,
                        score=new_score,
                        matched_pattern=text_hook.matched_pattern,
                    )
                )
                logger.debug(
                    "AudioAwareHookEngine [{:.1f}–{:.1f}]: text={} audio_boost={} → {}",
                    seg.start, seg.end, text_hook.score, boost, new_score,
                )

            elif text_hook is not None:
                # Text hook with no audio signal – keep as-is
                merged.append(text_hook)

            elif audio_sig is not None and audio_sig.audio_score >= _AUDIO_ONLY_THRESHOLD:
                # Audio-only hook – surface if signal is strong enough
                hook_type = audio_sig.audio_hook_type or "emotion"
                audio_only_score = min(100, int(audio_sig.audio_score * 0.85))
                if audio_only_score >= min_score:
                    merged.append(
                        HookDetectionResult(
                            start=seg.start,
                            end=seg.end,
                            type=hook_type,
                            score=audio_only_score,
                            matched_pattern=f"audio:{audio_sig.audio_emotion}",
                        )
                    )
                    logger.debug(
                        "AudioAwareHookEngine [{:.1f}–{:.1f}]: audio-only hook type={} score={}",
                        seg.start, seg.end, hook_type, audio_only_score,
                    )

        # Filter by min_score and sort
        results = [h for h in merged if h.score >= min_score]
        results.sort(key=lambda r: r.score, reverse=True)

        logger.info(
            "AudioAwareHookEngine: {} segments → {} hooks (min_score={})",
            len(segments), len(results), min_score,
        )
        return results


# ---------------------------------------------------------------------------
# Factory
# ---------------------------------------------------------------------------

def build_audio_aware_hook_engine() -> AudioAwareHookEngine:
    """Return a default-configured :class:`AudioAwareHookEngine`."""
    from app.services.audio_signal_analyzer import build_audio_signal_analyzer

    return AudioAwareHookEngine(
        text_engine=HookEngineV2(
            detector=HookPatternDetector(),
            calculator=HookScoreCalculator(),
        ),
        audio_analyzer=build_audio_signal_analyzer(),
    )
