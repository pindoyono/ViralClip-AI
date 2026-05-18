"""Hook Engine V2 – orchestrates pattern detection and score calculation.

Usage
-----
    from app.services.hook_engine_v2 import HookEngineV2
    from app.services.hook_pattern_detector import HookPatternDetector
    from app.services.hook_score_calculator import HookScoreCalculator
    from app.models.schemas import TranscriptSegmentInput

    engine = HookEngineV2(HookPatternDetector(), HookScoreCalculator())
    results = engine.detect_hooks(segments, min_score=50)

The engine is intentionally synchronous internally – all pattern matching
and scoring is CPU-bound and completes in microseconds per segment.  The
public ``detect_hooks`` method is *async* to fit naturally into FastAPI
route handlers and allow future AI-augmented scoring to be added without
breaking the interface.
"""

from __future__ import annotations

from typing import List, Optional

from loguru import logger

from app.models.schemas import HookDetectionResult, TranscriptSegmentInput
from app.services.hook_pattern_detector import HookPatternDetector
from app.services.hook_score_calculator import HookScoreCalculator


class HookEngineV2:
    """Analyse transcript segments and identify hook moments.

    Parameters
    ----------
    detector:
        An instance of :class:`HookPatternDetector`.  Can be mocked in tests.
    calculator:
        An instance of :class:`HookScoreCalculator`.  Can be mocked in tests.
    """

    def __init__(
        self,
        detector: HookPatternDetector,
        calculator: HookScoreCalculator,
    ) -> None:
        self._detector = detector
        self._calculator = calculator

    async def detect_hooks(
        self,
        segments: List[TranscriptSegmentInput],
        min_score: int = 50,
    ) -> List[HookDetectionResult]:
        """Detect and score hook moments across all transcript segments.

        Parameters
        ----------
        segments:
            List of transcript segments with timestamps and text.
        min_score:
            Minimum score threshold (0–100).  Segments scoring below this
            value are excluded from the result.

        Returns
        -------
        List[HookDetectionResult]
            Hook detections sorted by descending score.
        """
        if not segments:
            logger.warning("HookEngineV2.detect_hooks called with empty segments list")
            return []

        total = len(segments)
        results: List[HookDetectionResult] = []

        for idx, seg in enumerate(segments):
            text = seg.text.strip()
            if not text:
                continue

            patterns = self._detector.detect(text)
            if not patterns:
                continue

            # Find the dominant (highest-confidence) hook type
            best_pattern = max(patterns, key=lambda p: p.confidence)
            hook_type = best_pattern.hook_type
            matched_text = best_pattern.matched_text

            prev_end: Optional[float] = segments[idx - 1].end if idx > 0 else None

            score = self._calculator.calculate(
                text=text,
                patterns=patterns,
                segment_index=idx,
                total_segments=total,
                prev_segment_end=prev_end,
                segment_start=seg.start,
            )

            if score < min_score:
                logger.debug(
                    "Segment [{:.1f}–{:.1f}] scored {} (< min_score {}), skipping",
                    seg.start, seg.end, score, min_score,
                )
                continue

            results.append(
                HookDetectionResult(
                    start=seg.start,
                    end=seg.end,
                    type=hook_type,
                    score=score,
                    matched_pattern=matched_text,
                )
            )

        results.sort(key=lambda r: r.score, reverse=True)
        logger.info(
            "HookEngineV2: analysed {} segments, found {} hooks (min_score={})",
            total, len(results), min_score,
        )
        return results


# ---------------------------------------------------------------------------
# Module-level singleton factory (for use in router)
# ---------------------------------------------------------------------------

def build_hook_engine_v2() -> HookEngineV2:
    """Return a default-configured :class:`HookEngineV2` instance."""
    return HookEngineV2(
        detector=HookPatternDetector(),
        calculator=HookScoreCalculator(),
    )
