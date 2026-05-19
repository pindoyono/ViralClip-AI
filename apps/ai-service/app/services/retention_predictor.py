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
from typing import Any, Dict, List, Optional, Sequence

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
        category: str = "general",
        hook_types: Optional[Sequence[str]] = None,
        historical_analytics: Optional[Dict[str, Any]] = None,
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
        learned_s = self._historical_score(
            duration=duration,
            category=category,
            hook_types=hook_types or [],
            historical_analytics=historical_analytics or {},
        )
        confidence = self._historical_confidence(historical_analytics or {})
        blended = raw if learned_s is None else ((1.0 - confidence) * raw) + (confidence * learned_s)
        s = max(0, min(100, round(blended)))
        logger.debug(
            "RetentionPredictor: dur={:.1f}s dur_s={:.1f} dens={:.1f} qa={:.1f} hook={} var={:.1f} learned={} conf={:.2f} → {}",
            duration, duration_s, density_s, qa_s, has_hook, variety_s, learned_s, confidence, s,
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

    def _historical_score(
        self,
        duration: float,
        category: str,
        hook_types: Sequence[str],
        historical_analytics: Dict[str, Any],
    ) -> Optional[float]:
        baseline = self._safe_ratio(historical_analytics.get("baseline_retention"))
        if baseline is None:
            return None

        duration_ret = self._lookup_duration_retention(duration, historical_analytics.get("duration_bucket_retention"))
        category_ret = self._lookup_map_retention(category, historical_analytics.get("category_retention"))
        hook_ret = self._lookup_hook_retention(hook_types, historical_analytics.get("hook_type_retention"))

        duration_ret = baseline if duration_ret is None else duration_ret
        category_ret = baseline if category_ret is None else category_ret
        hook_ret = baseline if hook_ret is None else hook_ret

        learned_ratio = (baseline * 0.35) + (duration_ret * 0.25) + (category_ret * 0.20) + (hook_ret * 0.20)
        return max(0.0, min(100.0, learned_ratio * 100.0))

    def _historical_confidence(self, historical_analytics: Dict[str, Any]) -> float:
        sample_size = historical_analytics.get("sample_size", 0)
        if not isinstance(sample_size, (int, float)) or sample_size <= 0:
            return 0.0
        # Cap historical influence to keep text-signal heuristics dominant.
        return min(0.45, float(sample_size) / 200.0)

    def _lookup_duration_retention(self, duration: float, duration_map: Any) -> Optional[float]:
        if not isinstance(duration_map, dict):
            return None
        bucket = "short" if duration < 30 else ("medium" if duration <= 60 else "long")
        return self._safe_ratio(duration_map.get(bucket))

    def _lookup_map_retention(self, key: str, values: Any) -> Optional[float]:
        if not isinstance(values, dict):
            return None
        lookup = str(key or "").strip().lower()
        if lookup == "":
            return None
        return self._safe_ratio(values.get(lookup))

    def _lookup_hook_retention(self, hook_types: Sequence[str], hook_map: Any) -> Optional[float]:
        if not isinstance(hook_map, dict) or not hook_types:
            return None
        ratios: List[float] = []
        for hook_type in hook_types:
            ratio = self._safe_ratio(hook_map.get(str(hook_type).strip().lower()))
            if ratio is not None:
                ratios.append(ratio)
        if not ratios:
            return None
        return sum(ratios) / len(ratios)

    def _safe_ratio(self, value: Any) -> Optional[float]:
        if isinstance(value, (int, float)):
            return max(0.0, min(1.0, float(value)))
        return None
