"""Dynamic Clip Engine V2.

Generates clip candidates from a transcript using hook scores, emotion scores,
story arc scores, and retention prediction.  Clip duration is constrained by
a content-profile type.

Profile duration rules (seconds):
    gaming     → 15 – 45
    comedy     → 10 – 30
    education  → 30 – 60
    politics   → 30 – 90
    podcast    → 45 – 90
    general    → 15 – 90   (default fallback)

Algorithm:
    1. Score every segment for emotion and story arc.
    2. Map existing hook detections onto segment indices (best hook score
       overlapping each segment).
    3. Identify "anchor" segments: hook_score > 0 OR emotion_score ≥ 60.
    4. For each anchor, build the smallest forward-growing window that
       satisfies min_duration.  Expand up to max_duration.
    5. Score each window using ClipScoreCalculatorV2.
    6. Deduplicate overlapping windows (keep the higher-scoring one).
    7. Sort by score descending; return top `max_clips`.

Output is a list of ClipV2Result with HH:MM:SS start/end strings.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any, Dict, List, Optional

from loguru import logger

from app.models.schemas import HookDetectionResult, TranscriptSegmentInput
from app.services.clip_score_calculator_v2 import ClipScoreCalculatorV2
from app.services.emotion_analyzer import EmotionAnalyzer
from app.services.retention_predictor import RetentionPredictor
from app.services.story_arc_detector import StoryArcDetector


# ---------------------------------------------------------------------------
# Profile duration rules
# ---------------------------------------------------------------------------

PROFILE_DURATION_RULES: dict[str, tuple[float, float]] = {
    "gaming":    (15.0,  45.0),
    "comedy":    (10.0,  30.0),
    "education": (30.0,  60.0),
    "politics":  (30.0,  90.0),
    "podcast":   (45.0,  90.0),
    "general":   (15.0,  90.0),
}


def _seconds_to_hms(seconds: float) -> str:
    """Convert float seconds to 'HH:MM:SS' string."""
    total = max(0, int(round(seconds)))
    h = total // 3600
    m = (total % 3600) // 60
    s = total % 60
    return f"{h:02d}:{m:02d}:{s:02d}"


# ---------------------------------------------------------------------------
# Output type
# ---------------------------------------------------------------------------

@dataclass
class ClipV2Result:
    """A single detected clip candidate."""
    start: str          # "HH:MM:SS"
    end: str            # "HH:MM:SS"
    start_seconds: float
    end_seconds: float
    score: int          # 0–100 composite
    hook_score: float
    emotion_score: float
    story_score: float
    retention_score: float
    profile_type: str


# ---------------------------------------------------------------------------
# Engine
# ---------------------------------------------------------------------------

class ClipEngineV2:
    """Generate clip candidates from a transcript.

    Parameters
    ----------
    emotion_analyzer:
        :class:`EmotionAnalyzer` instance.
    story_arc_detector:
        :class:`StoryArcDetector` instance.
    retention_predictor:
        :class:`RetentionPredictor` instance.
    score_calculator:
        :class:`ClipScoreCalculatorV2` instance.
    """

    def __init__(
        self,
        emotion_analyzer: EmotionAnalyzer,
        story_arc_detector: StoryArcDetector,
        retention_predictor: RetentionPredictor,
        score_calculator: ClipScoreCalculatorV2,
    ) -> None:
        self._emotion = emotion_analyzer
        self._story = story_arc_detector
        self._retention = retention_predictor
        self._score = score_calculator

    async def generate_clips(
        self,
        segments: List[TranscriptSegmentInput],
        hook_detections: List[HookDetectionResult],
        profile_type: str = "general",
        min_clip_score: int = 50,
        max_clips: int = 10,
        historical_analytics: Optional[Dict[str, Any]] = None,
    ) -> List[ClipV2Result]:
        """Generate scored clip candidates from transcript segments.

        Parameters
        ----------
        segments:
            Transcript segments (ordered by start time).
        hook_detections:
            V2 hook detections for the same video.
        profile_type:
            One of gaming / comedy / education / politics / podcast / general.
        min_clip_score:
            Only return clips whose composite score meets this threshold.
        max_clips:
            Maximum number of clips to return (sorted by score desc).

        Returns
        -------
        List[ClipV2Result]
            Clip candidates sorted by score descending.
        """
        if not segments:
            logger.warning("ClipEngineV2.generate_clips: empty segments list")
            return []

        profile_type = profile_type.lower()
        min_dur, max_dur = PROFILE_DURATION_RULES.get(profile_type, PROFILE_DURATION_RULES["general"])
        total = len(segments)

        # ------------------------------------------------------------------
        # Step 1 – per-segment scores
        # ------------------------------------------------------------------
        emotion_scores = [self._emotion.score(seg.text) for seg in segments]
        story_scores   = [
            self._story.score(seg.text, idx, total)
            for idx, seg in enumerate(segments)
        ]

        # ------------------------------------------------------------------
        # Step 2 – map hook detections onto segment indices
        #          (best hook score overlapping each segment)
        # ------------------------------------------------------------------
        hook_score_map: dict[int, float] = {}
        hook_type_map: dict[int, set[str]] = {}
        for det in hook_detections:
            for idx, seg in enumerate(segments):
                if seg.start < det.end and seg.end > det.start:
                    existing = hook_score_map.get(idx, 0.0)
                    hook_score_map[idx] = max(existing, float(det.score))
                    if idx not in hook_type_map:
                        hook_type_map[idx] = set()
                    hook_type_map[idx].add(det.type.lower())

        # ------------------------------------------------------------------
        # Step 3 – identify anchor segments
        # ------------------------------------------------------------------
        anchors: list[int] = [
            idx for idx in range(total)
            if hook_score_map.get(idx, 0.0) > 0 or emotion_scores[idx] >= 60
        ]
        if not anchors:
            # Fall back to the top-3 emotion peaks
            sorted_by_emotion = sorted(range(total), key=lambda i: emotion_scores[i], reverse=True)
            anchors = sorted_by_emotion[:3]

        logger.debug("ClipEngineV2: {} anchors for profile={}", len(anchors), profile_type)

        # ------------------------------------------------------------------
        # Step 4 – build windows from each anchor
        # ------------------------------------------------------------------
        candidates: list[ClipV2Result] = []
        for anchor_idx in anchors:
            result = self._build_window(
                segments=segments,
                anchor_idx=anchor_idx,
                min_dur=min_dur,
                max_dur=max_dur,
                emotion_scores=emotion_scores,
                story_scores=story_scores,
                hook_score_map=hook_score_map,
                hook_type_map=hook_type_map,
                profile_type=profile_type,
                historical_analytics=historical_analytics,
            )
            if result is not None:
                candidates.append(result)

        # ------------------------------------------------------------------
        # Step 5 – deduplicate overlapping windows
        # ------------------------------------------------------------------
        candidates = self._deduplicate(candidates)

        # ------------------------------------------------------------------
        # Step 6 – filter by min_clip_score, sort, limit
        # ------------------------------------------------------------------
        candidates = [c for c in candidates if c.score >= min_clip_score]
        candidates.sort(key=lambda c: c.score, reverse=True)
        result_clips = candidates[:max_clips]

        logger.info(
            "ClipEngineV2: profile={} anchors={} clips={}",
            profile_type, len(anchors), len(result_clips),
        )
        return result_clips

    # ------------------------------------------------------------------
    # Private helpers
    # ------------------------------------------------------------------

    def _build_window(
        self,
        segments: List[TranscriptSegmentInput],
        anchor_idx: int,
        min_dur: float,
        max_dur: float,
        emotion_scores: list[int],
        story_scores: list[int],
        hook_score_map: dict[int, float],
        hook_type_map: dict[int, set[str]],
        profile_type: str,
        historical_analytics: Optional[Dict[str, Any]],
    ) -> Optional[ClipV2Result]:
        """Try to build a valid clip window starting at anchor_idx.

        The window grows forward until min_duration is satisfied.  Expansion
        stops when max_duration would be exceeded.
        """
        total = len(segments)
        start_seg = anchor_idx
        end_seg = anchor_idx  # inclusive

        # Grow forward until min_dur is satisfied
        while end_seg < total - 1:
            dur = segments[end_seg].end - segments[start_seg].start
            if dur >= min_dur:
                break
            end_seg += 1

        # Check that we have enough duration
        dur = segments[end_seg].end - segments[start_seg].start
        if dur < min_dur:
            return None  # not enough content

        # Trim if over max_dur
        while dur > max_dur and end_seg > start_seg:
            end_seg -= 1
            dur = segments[end_seg].end - segments[start_seg].start

        if dur < min_dur or dur > max_dur:
            return None

        # Collect indices in window
        window_indices = list(range(start_seg, end_seg + 1))

        # Aggregate scores across window
        window_text = " ".join(segments[i].text for i in window_indices)
        avg_emotion = sum(emotion_scores[i] for i in window_indices) / len(window_indices)
        avg_story   = sum(story_scores[i]   for i in window_indices) / len(window_indices)
        best_hook   = max((hook_score_map.get(i, 0.0) for i in window_indices), default=0.0)
        has_hook    = best_hook > 0
        hook_types = sorted(
            {
                hook_type
                for i in window_indices
                for hook_type in hook_type_map.get(i, set())
            }
        )

        min_d, max_d = PROFILE_DURATION_RULES.get(profile_type, PROFILE_DURATION_RULES["general"])
        retention = self._retention.score(
            text=window_text,
            duration=dur,
            min_duration=min_d,
            max_duration=max_d,
            has_hook=has_hook,
            category=profile_type,
            hook_types=hook_types,
            historical_analytics=historical_analytics,
        )

        composite = self._score.calculate(
            hook_score=best_hook,
            emotion_score=avg_emotion,
            story_score=avg_story,
            retention_score=retention,
        )

        start_s = segments[start_seg].start
        end_s   = segments[end_seg].end

        return ClipV2Result(
            start=_seconds_to_hms(start_s),
            end=_seconds_to_hms(end_s),
            start_seconds=start_s,
            end_seconds=end_s,
            score=composite,
            hook_score=best_hook,
            emotion_score=avg_emotion,
            story_score=avg_story,
            retention_score=float(retention),
            profile_type=profile_type,
        )

    def _deduplicate(self, candidates: list[ClipV2Result]) -> list[ClipV2Result]:
        """Remove overlapping windows; keep the higher-scoring one.

        Two windows overlap if their time ranges share > 50 % of the shorter
        window's duration.
        """
        # Sort by score desc so the first occurrence is always the better one
        candidates.sort(key=lambda c: c.score, reverse=True)
        kept: list[ClipV2Result] = []
        for candidate in candidates:
            overlap = False
            for existing in kept:
                overlap_start = max(candidate.start_seconds, existing.start_seconds)
                overlap_end   = min(candidate.end_seconds,   existing.end_seconds)
                overlap_dur   = max(0.0, overlap_end - overlap_start)
                candidate_dur = candidate.end_seconds - candidate.start_seconds
                if candidate_dur > 0 and overlap_dur / candidate_dur > 0.50:
                    overlap = True
                    break
            if not overlap:
                kept.append(candidate)
        return kept


# ---------------------------------------------------------------------------
# Factory
# ---------------------------------------------------------------------------

def build_clip_engine_v2() -> ClipEngineV2:
    """Return a default-configured ClipEngineV2."""
    return ClipEngineV2(
        emotion_analyzer=EmotionAnalyzer(),
        story_arc_detector=StoryArcDetector(),
        retention_predictor=RetentionPredictor(),
        score_calculator=ClipScoreCalculatorV2(),
    )
