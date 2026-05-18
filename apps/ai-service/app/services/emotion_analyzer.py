"""Emotion Analyzer for Clip Engine V2.

Scores transcript segment text for emotional intensity on a scale of 0–100.
Uses weighted regex patterns grouped by emotion family and intensity tier.

Design goals:
- Stateless & CPU-only (no network/ML)
- Fully injectable for tests
- Consistent with HookPatternDetector pattern style
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import List

from loguru import logger


# ---------------------------------------------------------------------------
# Pattern specifications
# ---------------------------------------------------------------------------

@dataclass(frozen=True)
class _EmotionSpec:
    pattern: re.Pattern[str]
    weight: float  # contribution to raw emotion score (0–1)


# High-intensity emotion patterns (weight 0.85–1.0)
_HIGH_SPECS: List[_EmotionSpec] = [
    _EmotionSpec(re.compile(r"\bterrif(ied|ying)\b", re.IGNORECASE), 0.95),
    _EmotionSpec(re.compile(r"\bheartbreak(ing)?\b", re.IGNORECASE), 0.95),
    _EmotionSpec(re.compile(r"\bdestroyed?\b", re.IGNORECASE), 0.90),
    _EmotionSpec(re.compile(r"\bshattered?\b", re.IGNORECASE), 0.90),
    _EmotionSpec(re.compile(r"\bdevastated?\b", re.IGNORECASE), 0.92),
    _EmotionSpec(re.compile(r"\becstatic\b", re.IGNORECASE), 0.90),
    _EmotionSpec(re.compile(r"\beuphoric\b", re.IGNORECASE), 0.88),
    _EmotionSpec(re.compile(r"\benraged?\b", re.IGNORECASE), 0.92),
    _EmotionSpec(re.compile(r"\bfurious(ly)?\b", re.IGNORECASE), 0.90),
    _EmotionSpec(re.compile(r"\bsobbing\b", re.IGNORECASE), 0.88),
    _EmotionSpec(re.compile(r"\bcried (my|his|her|their) eyes\b", re.IGNORECASE), 0.90),
    _EmotionSpec(re.compile(r"\bpanic(ked|king)?\b", re.IGNORECASE), 0.88),
    _EmotionSpec(re.compile(r"\bscreamed?\b", re.IGNORECASE), 0.85),
    _EmotionSpec(re.compile(r"\bwept\b", re.IGNORECASE), 0.85),
]

# Medium-intensity emotion patterns (weight 0.60–0.84)
_MEDIUM_SPECS: List[_EmotionSpec] = [
    _EmotionSpec(re.compile(r"\bangr(y|ily)?\b", re.IGNORECASE), 0.78),
    _EmotionSpec(re.compile(r"\bupset\b", re.IGNORECASE), 0.70),
    _EmotionSpec(re.compile(r"\bscared?\b", re.IGNORECASE), 0.75),
    _EmotionSpec(re.compile(r"\bfrightened\b", re.IGNORECASE), 0.75),
    _EmotionSpec(re.compile(r"\bdisappointed\b", re.IGNORECASE), 0.72),
    _EmotionSpec(re.compile(r"\bexcited?\b", re.IGNORECASE), 0.70),
    _EmotionSpec(re.compile(r"\bsurprised?\b", re.IGNORECASE), 0.68),
    _EmotionSpec(re.compile(r"\bshocked?\b", re.IGNORECASE), 0.75),
    _EmotionSpec(re.compile(r"\bjealous\b", re.IGNORECASE), 0.70),
    _EmotionSpec(re.compile(r"\bhumiliated?\b", re.IGNORECASE), 0.80),
    _EmotionSpec(re.compile(r"\bashamed?\b", re.IGNORECASE), 0.72),
    _EmotionSpec(re.compile(r"\bguilty\b", re.IGNORECASE), 0.68),
    _EmotionSpec(re.compile(r"\bblessed\b", re.IGNORECASE), 0.65),
    _EmotionSpec(re.compile(r"\bgrateful\b", re.IGNORECASE), 0.62),
    _EmotionSpec(re.compile(r"\bomg\b", re.IGNORECASE), 0.70),
    _EmotionSpec(re.compile(r"\boh my god\b", re.IGNORECASE), 0.75),
    _EmotionSpec(re.compile(r"\bno way\b", re.IGNORECASE), 0.68),
    _EmotionSpec(re.compile(r"\bi can'?t believe\b", re.IGNORECASE), 0.72),
]

# Low-intensity emotion patterns (weight 0.30–0.59)
_LOW_SPECS: List[_EmotionSpec] = [
    _EmotionSpec(re.compile(r"\bsad(ly)?\b", re.IGNORECASE), 0.55),
    _EmotionSpec(re.compile(r"\bhappy\b", re.IGNORECASE), 0.50),
    _EmotionSpec(re.compile(r"\bworried?\b", re.IGNORECASE), 0.52),
    _EmotionSpec(re.compile(r"\bnervous(ly)?\b", re.IGNORECASE), 0.50),
    _EmotionSpec(re.compile(r"\bconfused?\b", re.IGNORECASE), 0.45),
    _EmotionSpec(re.compile(r"\blove(d|s)?\b", re.IGNORECASE), 0.50),
    _EmotionSpec(re.compile(r"\bhate(d|s)?\b", re.IGNORECASE), 0.55),
    _EmotionSpec(re.compile(r"\bfrustrat(ed|ing)\b", re.IGNORECASE), 0.58),
    _EmotionSpec(re.compile(r"\bamazing\b", re.IGNORECASE), 0.45),
    _EmotionSpec(re.compile(r"\bincredible\b", re.IGNORECASE), 0.48),
]

_ALL_SPECS: List[_EmotionSpec] = _HIGH_SPECS + _MEDIUM_SPECS + _LOW_SPECS

# Amplifier words (multiply raw score by this factor when present)
_AMPLIFIERS: list[re.Pattern[str]] = [
    re.compile(r"\bso\b", re.IGNORECASE),
    re.compile(r"\bextremely\b", re.IGNORECASE),
    re.compile(r"\bincredibly\b", re.IGNORECASE),
    re.compile(r"\babsolutely\b", re.IGNORECASE),
    re.compile(r"\btotally\b", re.IGNORECASE),
    re.compile(r"\bcompletely\b", re.IGNORECASE),
    re.compile(r"\breally\b", re.IGNORECASE),
    re.compile(r"\binsanely\b", re.IGNORECASE),
    re.compile(r"\blegitimately\b", re.IGNORECASE),
]


class EmotionAnalyzer:
    """Score transcript segment text for emotional intensity.

    Parameters
    ----------
    specs:
        Override the default pattern list (useful in tests).
    amplifiers:
        Override the default amplifier pattern list.
    """

    def __init__(
        self,
        specs: List[_EmotionSpec] | None = None,
        amplifiers: list[re.Pattern[str]] | None = None,
    ) -> None:
        self._specs = specs if specs is not None else _ALL_SPECS
        self._amplifiers = amplifiers if amplifiers is not None else _AMPLIFIERS

    def score(self, text: str) -> int:
        """Return an emotion intensity score in [0, 100] for *text*.

        Multiple emotion hits are combined additively and capped at 100.
        Amplifier words multiply the raw score by 1.20.

        Parameters
        ----------
        text:
            Transcript segment text.

        Returns
        -------
        int
            Emotion score in [0, 100].
        """
        raw = 0.0
        for spec in self._specs:
            if spec.pattern.search(text):
                raw += spec.weight * 100

        if raw == 0.0:
            return 0

        # Amplifier bonus
        amplifier_hits = sum(
            1 for amp in self._amplifiers if amp.search(text)
        )
        if amplifier_hits > 0:
            raw *= 1.0 + min(0.40, amplifier_hits * 0.10)

        score = max(0, min(100, round(raw)))
        logger.debug("EmotionAnalyzer: text={!r:.40} → {}", text, score)
        return score
