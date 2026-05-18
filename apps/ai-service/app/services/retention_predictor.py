"""Retention Predictor for Clip Engine V2.

Predicts how likely viewers are to keep watching through a clip window
on a scale of 0–100.

Five signals are combined:

1. **Duration fit** — windows whose length matches the profile's target
   duration get a higher score; very short or very long windows are penalised.
2. **Word density** — words-per-second; too sparse (boring) or too dense
   (overwhelming) lowers retention.
3. **Question & exclamation ratio** — rhetorical questions and exclamations
   maintain attention.
4. **Hook presence** — whether a hook detection overlaps with this window.
5. **Sentence variation** — short + long sentences together signal a dynamic
   delivery style.
"""

from __future__ import annotations

import re
from typing import List, Optional

from loguru import logger


_QUESTION_RE = re.compile(r"\?")
_EXCLAMATION_RE = re.compile(r"!")
_SENTENCE_RE = re.compile(r"[.!?]+")
_WORD_RE = re.compile(r"\b[a-zA-Z]{2,}\b")


class RetentionPredictor:
    """Predict viewer retention for a clip window.

    Parameters
    ----------
    ideal_words_per_second:
        Target speech rate; deviations penalise the score (default 2.5 w/s).
    """

    def __init__(self, ideal_words_per_second: float = 2.5) -> None:
        self._ideal_wps = ideal_words_per_second

    def score(
        self,
        text: str,
        duration: float,
        min_duration: float,
        max_duration: float,
        has_hook: bool = False,
    ) -> int:
        """Return a retention score in [0, 100] for a clip window.

        Parameters
        ----------
        text:
            Concatenated transcript text for the window.
        duration:
            Duration of the window in seconds.
        min_duration:
            Minimum acceptable duration for this profile.
        max_duration:
            Maximum acceptable duration for this profile.
        has_hook:
            Whether a V2 hook detection overlaps with this window.

        Returns
        -------
        int
            Retention score clamped to [0, 100].
        """
        if duration <= 0:
            return 0

        duration_s = self._duration_score(duration, min_duration, max_duration)
        density_s  = self._density_score(text, duration)
        qa_s       = self._question_exclamation_score(text)
        hook_s     = 15.0 if has_hook else 0.0
        variety_s  = self._sentence_variety_score(text)

        raw = duration_s + density_s + qa_s + hook_s + variety_s
        s = max(0, min(100, round(raw)))
        logger.debug(
            "RetentionPredictor: dur={:.1f}s dur_s={:.1f} dens={:.1f} qa={:.1f} hook={} var={:.1f} → {}",
            duration, duration_s, density_s, qa_s, has_hook, variety_s, s,
        )
        return s

    # ------------------------------------------------------------------
    # Private signals
    # ------------------------------------------------------------------

    def _duration_score(
        self, duration: float, min_dur: float, max_dur: float
    ) -> float:
        """Up to 40 points; peaks at the midpoint of [min, max]."""
        target = (min_dur + max_dur) / 2
        max_deviation = (max_dur - min_dur) / 2
        if max_deviation <= 0:
            return 40.0
        deviation = abs(duration - target)
        fraction = max(0.0, 1.0 - deviation / max_deviation)
        return 40.0 * fraction

    def _density_score(self, text: str, duration: float) -> float:
        """Up to 25 points; peaks at _ideal_wps words/second."""
        word_count = len(_WORD_RE.findall(text))
        wps = word_count / duration
        # Gaussian-like penalty: score decreases as wps deviates from ideal
        diff = abs(wps - self._ideal_wps)
        score = 25.0 * max(0.0, 1.0 - diff / self._ideal_wps)
        return score

    def _question_exclamation_score(self, text: str) -> float:
        """Up to 15 points for questions/exclamations (engagement cues)."""
        hits = len(_QUESTION_RE.findall(text)) + len(_EXCLAMATION_RE.findall(text))
        return min(15.0, hits * 5.0)

    def _sentence_variety_score(self, text: str) -> float:
        """Up to 10 points when the clip contains sentences of mixed length."""
        sentences = [s.strip() for s in _SENTENCE_RE.split(text) if s.strip()]
        if len(sentences) < 2:
            return 0.0
        lengths = [len(s.split()) for s in sentences]
        min_len = min(lengths)
        max_len = max(lengths)
        variety = (max_len - min_len) / max(max_len, 1)
        return min(10.0, variety * 10.0)
