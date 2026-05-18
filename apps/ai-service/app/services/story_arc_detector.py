"""Story Arc Detector for Clip Engine V2.

Assigns a "story score" (0–100) to each transcript segment based on:

1. **Arc position bonus** — climax regions (roughly 55–80 % through the
   transcript) receive the highest bonus; setup (0–20 %) and resolution
   (80–100 %) receive lower bonuses.
2. **Keyword bonus** — narrative trigger words signal active story moments
   and add a flat bonus.
3. **Sentence complexity** — longer, richer sentences (multiple clauses)
   tend to carry more narrative weight.

The detector is stateless and deterministic.
"""

from __future__ import annotations

import re
from typing import List, Optional

from loguru import logger


# ---------------------------------------------------------------------------
# Arc keywords
# ---------------------------------------------------------------------------

_SETUP_RE = re.compile(
    r"\b(at first|in the beginning|once|one day|back when|long before|"
    r"it all started|before i|before we|when i was|years ago|"
    r"initially|originally)\b",
    re.IGNORECASE,
)

_BUILDUP_RE = re.compile(
    r"\b(and then|but then|after that|soon|eventually|over time|"
    r"as (time|days|weeks|months) went by|things started|began to|"
    r"slowly|gradually|little by little|more and more)\b",
    re.IGNORECASE,
)

_CLIMAX_RE = re.compile(
    r"\b(suddenly|out of nowhere|all of a sudden|that's when|"
    r"everything changed|it all changed|then it happened|the moment|"
    r"right then|at that exact|boom|and then suddenly|"
    r"nothing prepared me|i couldn'?t believe|that was (the|my) last)\b",
    re.IGNORECASE,
)

_RESOLUTION_RE = re.compile(
    r"\b(in the end|looking back|after all|now i (know|realise|realize|understand)|"
    r"it taught me|the lesson|what i learned|that experience|"
    r"i'?m glad|i survived|we made it|it worked out|the moral)\b",
    re.IGNORECASE,
)

# Bonus weights per keyword category (added to base arc-position score)
_KEYWORD_WEIGHTS: dict[re.Pattern[str], float] = {
    _SETUP_RE: 10.0,
    _BUILDUP_RE: 15.0,
    _CLIMAX_RE: 25.0,
    _RESOLUTION_RE: 12.0,
}

# Clause boundary pattern (comma, semicolon, conjunction) for complexity
_CLAUSE_RE = re.compile(r"[,;]|\b(and|but|because|although|however|therefore|which|who|that)\b", re.IGNORECASE)


class StoryArcDetector:
    """Assign a story score to a segment based on arc position and keywords.

    Parameters
    ----------
    keyword_weights:
        Override the default keyword-to-bonus mapping.
    """

    def __init__(
        self,
        keyword_weights: dict[re.Pattern[str], float] | None = None,
    ) -> None:
        self._kw = keyword_weights if keyword_weights is not None else _KEYWORD_WEIGHTS

    def score(
        self,
        text: str,
        segment_index: int,
        total_segments: int,
    ) -> int:
        """Return a story arc score in [0, 100] for a segment.

        Parameters
        ----------
        text:
            Transcript segment text.
        segment_index:
            Zero-based segment index within the full transcript.
        total_segments:
            Total number of segments.

        Returns
        -------
        int
            Story score clamped to [0, 100].
        """
        base = self._arc_position_score(segment_index, total_segments)
        kw_bonus = self._keyword_bonus(text)
        complexity = self._complexity_bonus(text)

        raw = base + kw_bonus + complexity
        s = max(0, min(100, round(raw)))
        logger.debug(
            "StoryArcDetector: idx={}/{} base={:.1f} kw={:.1f} cx={:.1f} → {}",
            segment_index, total_segments, base, kw_bonus, complexity, s,
        )
        return s

    # ------------------------------------------------------------------
    # Private helpers
    # ------------------------------------------------------------------

    def _arc_position_score(self, idx: int, total: int) -> float:
        """Return a position-based arc score in [0, 75]."""
        if total <= 0:
            return 30.0
        rel = idx / max(total - 1, 1)  # 0.0 … 1.0

        if rel < 0.20:        # setup
            return 30.0
        elif rel < 0.40:      # early buildup
            return 45.0
        elif rel < 0.55:      # late buildup
            return 55.0
        elif rel < 0.80:      # climax zone — highest value
            return 75.0
        else:                 # resolution
            return 35.0

    def _keyword_bonus(self, text: str) -> float:
        """Sum bonuses for all matched keyword categories (capped at 40)."""
        total_bonus = 0.0
        for pattern, bonus in self._kw.items():
            if pattern.search(text):
                total_bonus += bonus
        return min(40.0, total_bonus)

    def _complexity_bonus(self, text: str) -> float:
        """Up to +8 for clause-rich sentences."""
        clauses = len(_CLAUSE_RE.findall(text))
        return min(8.0, clauses * 2.0)
