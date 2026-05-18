"""Clip Score Calculator V2.

Implements the scoring formula from the specification:

    ClipScore = (HookScore × 50%) + (EmotionScore × 20%)
              + (StoryScore × 20%) + (RetentionPrediction × 10%)

All component scores are expected in [0, 100].
The returned score is an integer in [0, 100].
"""

from __future__ import annotations

from loguru import logger


_HOOK_WEIGHT      = 0.50
_EMOTION_WEIGHT   = 0.20
_STORY_WEIGHT     = 0.20
_RETENTION_WEIGHT = 0.10


class ClipScoreCalculatorV2:
    """Combine four signals into a single clip score.

    Parameters
    ----------
    hook_weight, emotion_weight, story_weight, retention_weight:
        Override the default weights (must sum to 1.0; not validated
        to keep the class open/flexible for experimentation).
    """

    def __init__(
        self,
        hook_weight:      float = _HOOK_WEIGHT,
        emotion_weight:   float = _EMOTION_WEIGHT,
        story_weight:     float = _STORY_WEIGHT,
        retention_weight: float = _RETENTION_WEIGHT,
    ) -> None:
        self._hw = hook_weight
        self._ew = emotion_weight
        self._sw = story_weight
        self._rw = retention_weight

    def calculate(
        self,
        hook_score:       float,
        emotion_score:    float,
        story_score:      float,
        retention_score:  float,
    ) -> int:
        """Return a weighted composite score in [0, 100].

        Parameters
        ----------
        hook_score:
            V2 hook detection score for the window (0–100).  Pass 0 when
            no hook was detected.
        emotion_score:
            Average emotion score across the window's segments (0–100).
        story_score:
            Average story arc score across the window's segments (0–100).
        retention_score:
            Predicted retention score for the window (0–100).

        Returns
        -------
        int
            Composite score clamped to [0, 100].
        """
        raw = (
            self._hw * hook_score
            + self._ew * emotion_score
            + self._sw * story_score
            + self._rw * retention_score
        )
        score = max(0, min(100, round(raw)))
        logger.debug(
            "ClipScoreV2: hook={:.1f} emotion={:.1f} story={:.1f} retention={:.1f} → {}",
            hook_score, emotion_score, story_score, retention_score, score,
        )
        return score
