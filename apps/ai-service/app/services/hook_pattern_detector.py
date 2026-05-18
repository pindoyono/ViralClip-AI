"""Hook pattern detection for Hook Engine V2.

Detects hook patterns across five categories: curiosity, emotion,
storytelling, controversy, and CTA.  Each match records the hook type,
the exact pattern that triggered it, and a confidence multiplier (0–1)
that later feeds the score calculator.
"""

from __future__ import annotations

import re
from dataclasses import dataclass, field
from typing import List

from loguru import logger


# ---------------------------------------------------------------------------
# Pattern registry
# ---------------------------------------------------------------------------

@dataclass(frozen=True)
class _PatternSpec:
    """A compiled pattern and its weight within the category."""
    pattern: re.Pattern[str]
    weight: float  # 0–1; higher = stronger indicator


_CURIOSITY_SPECS: List[_PatternSpec] = [
    _PatternSpec(re.compile(r"\bsecret\b", re.IGNORECASE), 0.90),
    _PatternSpec(re.compile(r"\bunbelievable\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bnobody knows\b", re.IGNORECASE), 0.90),
    _PatternSpec(re.compile(r"\bshocking(ly)?\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bsurprising(ly)?\b", re.IGNORECASE), 0.75),
    _PatternSpec(re.compile(r"\bhidden\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\brevealed?\b", re.IGNORECASE), 0.75),
    _PatternSpec(re.compile(r"\bdiscovered?\b", re.IGNORECASE), 0.70),
    _PatternSpec(re.compile(r"\bthe truth\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\byou won'?t believe\b", re.IGNORECASE), 0.90),
    _PatternSpec(re.compile(r"\bwait for it\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bmind.blowing\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\bnever before\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\bunexpected\b", re.IGNORECASE), 0.70),
    _PatternSpec(re.compile(r"\bimpossible\b", re.IGNORECASE), 0.70),
]

_EMOTION_SPECS: List[_PatternSpec] = [
    _PatternSpec(re.compile(r"\bangr(y|ily|ier|iest)\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\bexcited?\b", re.IGNORECASE), 0.75),
    _PatternSpec(re.compile(r"\bscared?\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\bsurprised?\b", re.IGNORECASE), 0.75),
    _PatternSpec(re.compile(r"\bheart.?breaking\b", re.IGNORECASE), 0.90),
    _PatternSpec(re.compile(r"\binspiring\b", re.IGNORECASE), 0.70),
    _PatternSpec(re.compile(r"\bterrif(ied|ying)\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bdisgusting\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\bamazing\b", re.IGNORECASE), 0.65),
    _PatternSpec(re.compile(r"\bincredible\b", re.IGNORECASE), 0.70),
    _PatternSpec(re.compile(r"\bfrustrat(ed|ing)\b", re.IGNORECASE), 0.75),
    _PatternSpec(re.compile(r"\boverwhelm(ed|ing)\b", re.IGNORECASE), 0.70),
    _PatternSpec(re.compile(r"\bemotional\b", re.IGNORECASE), 0.65),
    _PatternSpec(re.compile(r"\bcried?\b", re.IGNORECASE), 0.75),
    _PatternSpec(re.compile(r"\bbroke (my|his|her|their) heart\b", re.IGNORECASE), 0.90),
]

_STORYTELLING_SPECS: List[_PatternSpec] = [
    _PatternSpec(re.compile(r"\bone day\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bsuddenly\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\bat first\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\bout of nowhere\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bfor years\b", re.IGNORECASE), 0.75),
    _PatternSpec(re.compile(r"\bi remember\b", re.IGNORECASE), 0.70),
    _PatternSpec(re.compile(r"\bback when\b", re.IGNORECASE), 0.70),
    _PatternSpec(re.compile(r"\bit all started\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bthe moment (i|we|he|she|they)\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\bi never thought\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\band then\b", re.IGNORECASE), 0.60),
    _PatternSpec(re.compile(r"\bonce upon\b", re.IGNORECASE), 0.75),
    _PatternSpec(re.compile(r"\bthat's when\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\buntil (one day|suddenly|everything)\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\beverything changed\b", re.IGNORECASE), 0.85),
]

_CONTROVERSY_SPECS: List[_PatternSpec] = [
    _PatternSpec(re.compile(r"\bunpopular opinion\b", re.IGNORECASE), 0.95),
    _PatternSpec(re.compile(r"\bnobody talks about\b", re.IGNORECASE), 0.90),
    _PatternSpec(re.compile(r"\bcontroversial\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\beveryone is wrong\b", re.IGNORECASE), 0.90),
    _PatternSpec(re.compile(r"\bactually no\b", re.IGNORECASE), 0.75),
    _PatternSpec(re.compile(r"\bthey don'?t want (you|us|people)\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bhot take\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bchange my mind\b", re.IGNORECASE), 0.90),
    _PatternSpec(re.compile(r"\bdon'?t agree\b", re.IGNORECASE), 0.70),
    _PatternSpec(re.compile(r"\bfight me\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\bthe (real|hard|ugly) truth\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bam i (the only one|wrong)\b", re.IGNORECASE), 0.80),
]

_CTA_SPECS: List[_PatternSpec] = [
    _PatternSpec(re.compile(r"\bfollow (me|us|along)\b", re.IGNORECASE), 0.90),
    _PatternSpec(re.compile(r"\bsubscribe\b", re.IGNORECASE), 0.90),
    _PatternSpec(re.compile(r"\bleave a comment\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bcomment (below|down|your)\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bshare (this|it)\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\blike (this|and subscribe)\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\bclick (the|that|below)\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bcheck (it|this) out\b", re.IGNORECASE), 0.75),
    _PatternSpec(re.compile(r"\blink in (my )?bio\b", re.IGNORECASE), 0.90),
    _PatternSpec(re.compile(r"\bswipe up\b", re.IGNORECASE), 0.90),
    _PatternSpec(re.compile(r"\btap (the|that|here)\b", re.IGNORECASE), 0.85),
    _PatternSpec(re.compile(r"\bwatch (till|to|until) the end\b", re.IGNORECASE), 0.90),
    _PatternSpec(re.compile(r"\bdon'?t forget to\b", re.IGNORECASE), 0.80),
    _PatternSpec(re.compile(r"\bhit (the|that) (like|bell|subscribe)\b", re.IGNORECASE), 0.85),
]

# Category metadata: base score (0–100) applied before bonuses
CATEGORY_BASE_SCORES: dict[str, int] = {
    "curiosity": 85,
    "emotion": 78,
    "storytelling": 72,
    "controversy": 88,
    "cta": 68,
}

_PATTERN_MAP: dict[str, List[_PatternSpec]] = {
    "curiosity": _CURIOSITY_SPECS,
    "emotion": _EMOTION_SPECS,
    "storytelling": _STORYTELLING_SPECS,
    "controversy": _CONTROVERSY_SPECS,
    "cta": _CTA_SPECS,
}


# ---------------------------------------------------------------------------
# Public data types
# ---------------------------------------------------------------------------

@dataclass
class PatternMatch:
    """A single pattern hit within a transcript segment."""
    hook_type: str          # e.g. "curiosity"
    matched_text: str       # the sub-string that matched
    confidence: float       # 0–1 from _PatternSpec.weight


# ---------------------------------------------------------------------------
# Detector
# ---------------------------------------------------------------------------

class HookPatternDetector:
    """Detects hook patterns in transcript segment text.

    Designed for dependency injection – the pattern map can be overridden
    in tests or specialised subclasses.
    """

    def __init__(self, pattern_map: dict[str, List[_PatternSpec]] | None = None) -> None:
        self._patterns = pattern_map or _PATTERN_MAP

    def detect(self, text: str) -> List[PatternMatch]:
        """Return all pattern matches found in *text*.

        Parameters
        ----------
        text:
            The transcript segment text to analyse.

        Returns
        -------
        List[PatternMatch]
            One entry per (hook_type, matched_text) pair.  A segment may
            match multiple types (e.g. both storytelling and emotion).
        """
        matches: List[PatternMatch] = []
        for hook_type, specs in self._patterns.items():
            for spec in specs:
                for m in spec.pattern.finditer(text):
                    matches.append(
                        PatternMatch(
                            hook_type=hook_type,
                            matched_text=m.group(0),
                            confidence=spec.weight,
                        )
                    )
                    logger.debug(
                        "Pattern hit: type={} text={!r} matched={!r} confidence={:.2f}",
                        hook_type, text[:40], m.group(0), spec.weight,
                    )
        return matches
