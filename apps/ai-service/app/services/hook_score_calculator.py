"""Hook score calculation for Hook Engine V2.

Combines six independent signals into a final integer score in [0, 100]:

1. **Base score** – derived from the highest-confidence pattern match for
   the dominant category (see CATEGORY_BASE_SCORES).
2. **Position bonus** – segments early in the transcript tend to be stronger
   hooks; gives up to +15 points for the first 20 % of the content.
3. **Emphasis bonus** – intensifiers such as "very", "extremely", "totally",
   etc. amplify hook strength; up to +10 points.
4. **Pattern-count bonus** – multiple pattern hits signal richer hook
   potential; up to +10 points (+5 per additional match, capped).
5. **Pause bonus** – a speech pause (gap between the previous segment's end
   and this segment's start) suggests a natural attention reset; up to +8.
6. **Repetition bonus** – repeated content words within the segment indicate
   emphasis; up to +7 points.
"""

from __future__ import annotations

import re
from typing import List, Optional

from loguru import logger

from app.services.hook_pattern_detector import PatternMatch, CATEGORY_BASE_SCORES


# ---------------------------------------------------------------------------
# Emphasis word list
# ---------------------------------------------------------------------------

_EMPHASIS_WORDS: frozenset[str] = frozenset({
    "very", "extremely", "absolutely", "totally", "completely", "incredibly",
    "unbelievably", "massively", "insanely", "literally", "actually",
    "seriously", "genuinely", "truly", "really", "so", "such",
    "beyond", "way", "super", "ultra",
})

_WORD_RE = re.compile(r"\b[a-z]{3,}\b")  # words of at least 3 chars


class HookScoreCalculator:
    """Computes a 0–100 integer score for a single transcript segment.

    Parameters
    ----------
    pause_threshold:
        Gap in seconds between previous segment end and current segment
        start that counts as a speech pause (default 0.5 s).
    emphasis_words:
        Override the default set of emphasis/intensifier words.
    """

    def __init__(
        self,
        pause_threshold: float = 0.5,
        emphasis_words: frozenset[str] | None = None,
    ) -> None:
        self._pause_threshold = pause_threshold
        self._emphasis_words = emphasis_words if emphasis_words is not None else _EMPHASIS_WORDS

    # ------------------------------------------------------------------
    # Public interface
    # ------------------------------------------------------------------

    def calculate(
        self,
        text: str,
        patterns: List[PatternMatch],
        segment_index: int,
        total_segments: int,
        prev_segment_end: Optional[float] = None,
        segment_start: Optional[float] = None,
    ) -> int:
        """Return a score in [0, 100] for a transcript segment.

        Parameters
        ----------
        text:
            Raw text of the transcript segment.
        patterns:
            Pattern matches returned by ``HookPatternDetector.detect``.
        segment_index:
            Zero-based index of this segment within the full transcript.
        total_segments:
            Total number of segments in the full transcript.
        prev_segment_end:
            End timestamp (seconds) of the preceding segment, or *None* if
            this is the first segment.
        segment_start:
            Start timestamp (seconds) of this segment, used to compute the
            speech-pause signal.

        Returns
        -------
        int
            Score clamped to [0, 100].
        """
        if not patterns:
            return 0

        base = self._base_score(patterns)
        position = self._position_bonus(segment_index, total_segments)
        emphasis = self._emphasis_bonus(text)
        pattern_count = self._pattern_count_bonus(patterns)
        pause = self._pause_bonus(prev_segment_end, segment_start)
        repetition = self._repetition_bonus(text)

        raw = base + position + emphasis + pattern_count + pause + repetition
        score = max(0, min(100, round(raw)))

        logger.debug(
            "Score breakdown: base={} pos={} emph={} pat={} pause={} rep={} → {}",
            base, position, emphasis, pattern_count, pause, repetition, score,
        )
        return score

    # ------------------------------------------------------------------
    # Individual signal calculators
    # ------------------------------------------------------------------

    def _base_score(self, patterns: List[PatternMatch]) -> float:
        """Weighted base score from the best matching pattern."""
        best: float = 0.0
        for m in patterns:
            category_base = CATEGORY_BASE_SCORES.get(m.hook_type, 70)
            weighted = category_base * m.confidence
            if weighted > best:
                best = weighted
        return best

    def _position_bonus(self, segment_index: int, total_segments: int) -> float:
        """Up to +15 for segments in the first 20 % of the transcript."""
        if total_segments <= 0:
            return 0.0
        relative_pos = segment_index / total_segments
        if relative_pos <= 0.20:
            return 15.0
        if relative_pos <= 0.40:
            return 8.0
        if relative_pos <= 0.60:
            return 4.0
        return 0.0

    def _emphasis_bonus(self, text: str) -> float:
        """Up to +10 based on emphasis / intensifier word count."""
        words = {w.lower() for w in _WORD_RE.findall(text)}
        hits = len(words & self._emphasis_words)
        return min(10.0, hits * 3.0)

    def _pattern_count_bonus(self, patterns: List[PatternMatch]) -> float:
        """Up to +10 for multiple distinct patterns; +5 per extra match."""
        extra = max(0, len(patterns) - 1)
        return min(10.0, extra * 5.0)

    def _pause_bonus(
        self,
        prev_end: Optional[float],
        current_start: Optional[float],
    ) -> float:
        """Up to +8 for a speech pause before this segment."""
        if prev_end is None or current_start is None:
            return 0.0
        gap = current_start - prev_end
        if gap >= 1.5:
            return 8.0
        if gap >= self._pause_threshold:
            return 4.0
        return 0.0

    def _repetition_bonus(self, text: str) -> float:
        """Up to +7 when content words repeat within the segment."""
        words = [w.lower() for w in _WORD_RE.findall(text)]
        if not words:
            return 0.0
        # Count words that appear more than once (exclude stop-words implicitly
        # via the 3-char minimum in _WORD_RE)
        from collections import Counter
        repeated = sum(1 for _, cnt in Counter(words).items() if cnt >= 2)
        return min(7.0, repeated * 2.5)
