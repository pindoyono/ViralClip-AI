"""Speech Pattern Analyzer for Audio-Aware Hook Engine.

Analyses the *rate* of speech derived from transcript segment timing.
No audio file is required – only the word count and duration of each segment.

Detects:
- **fast**          – wps > μ + σ  (hurried, energetic delivery)
- **slow**          – wps < μ − σ  (deliberate, dramatic delivery)
- **normal**        – near the mean
- **sudden_change** – large absolute Δwps from the preceding segment
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import List, Optional, Sequence

import numpy as np
from loguru import logger


# ---------------------------------------------------------------------------
# Public data types
# ---------------------------------------------------------------------------

@dataclass
class SpeechPatternInfo:
    """Speech rate characteristics for one transcript segment."""

    start: float                 # segment start (seconds)
    end: float                   # segment end (seconds)
    words_per_second: float      # word rate for this segment
    speech_rate: str             # "fast" | "slow" | "normal" | "sudden_change"
    rate_deviation: float        # (wps − mean) / std; positive = fast


# ---------------------------------------------------------------------------
# Analyzer
# ---------------------------------------------------------------------------

class SpeechPatternAnalyzer:
    """Compute speech-rate patterns across all transcript segments.

    Parameters
    ----------
    fast_sigma:
        Number of standard deviations above the mean to be "fast".
    slow_sigma:
        Number of standard deviations below the mean to be "slow".
    sudden_change_threshold:
        Absolute Δwps (from the previous segment) that qualifies as a
        sudden change regardless of the global distribution.
    """

    def __init__(
        self,
        fast_sigma: float = 1.0,
        slow_sigma: float = 1.0,
        sudden_change_threshold: float = 2.0,
    ) -> None:
        self._fast_sigma = fast_sigma
        self._slow_sigma = slow_sigma
        self._sudden_change_threshold = sudden_change_threshold

    # ------------------------------------------------------------------
    # Public interface
    # ------------------------------------------------------------------

    def analyze(
        self,
        segments: Sequence[tuple[float, float, str]],
    ) -> List[SpeechPatternInfo]:
        """Analyse speech rate for a list of (start, end, text) tuples.

        Parameters
        ----------
        segments:
            Ordered list of (start_seconds, end_seconds, text) tuples.

        Returns
        -------
        List[SpeechPatternInfo]
            One entry per input segment, in the same order.
        """
        if not segments:
            return []

        wps_values = [self._words_per_second(start, end, text) for start, end, text in segments]
        wps_arr = np.array(wps_values, dtype=float)

        mean_wps = float(np.mean(wps_arr))
        std_wps = float(np.std(wps_arr)) if len(wps_arr) > 1 else 0.0

        results: List[SpeechPatternInfo] = []
        for i, (start, end, _text) in enumerate(segments):
            wps = wps_values[i]
            deviation = (wps - mean_wps) / std_wps if std_wps > 0 else 0.0

            prev_wps: Optional[float] = wps_values[i - 1] if i > 0 else None
            rate = self._classify(wps, mean_wps, std_wps, prev_wps)

            results.append(
                SpeechPatternInfo(
                    start=start,
                    end=end,
                    words_per_second=round(wps, 3),
                    speech_rate=rate,
                    rate_deviation=round(deviation, 3),
                )
            )

        logger.debug(
            "SpeechPatternAnalyzer: {} segments, mean wps={:.2f}, std={:.2f}",
            len(segments), mean_wps, std_wps,
        )
        return results

    # ------------------------------------------------------------------
    # Private helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _words_per_second(start: float, end: float, text: str) -> float:
        duration = max(0.1, end - start)  # avoid division by zero
        word_count = len(text.split())
        return word_count / duration

    def _classify(
        self,
        wps: float,
        mean_wps: float,
        std_wps: float,
        prev_wps: Optional[float],
    ) -> str:
        # Sudden change from preceding segment takes priority
        if prev_wps is not None:
            delta = abs(wps - prev_wps)
            if delta >= self._sudden_change_threshold:
                return "sudden_change"

        if std_wps > 0:
            if wps > mean_wps + self._fast_sigma * std_wps:
                return "fast"
            if wps < mean_wps - self._slow_sigma * std_wps:
                return "slow"

        return "normal"
