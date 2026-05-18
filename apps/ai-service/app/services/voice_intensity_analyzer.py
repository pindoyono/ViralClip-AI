"""Voice Intensity Analyzer for Audio-Aware Hook Engine.

Analyses the energy envelope of a mono audio signal to detect:

- **loud**            – sustained high RMS relative to the track baseline
- **quiet**           – sustained low RMS
- **normal**          – near-baseline energy
- **sudden_increase** – a sharp jump in energy within the segment
- **emotional**       – both loud AND energetically variable (high std-dev)

All timestamps are in *seconds* and match the transcript segment time-base.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import List, Optional

import numpy as np
from loguru import logger


# ---------------------------------------------------------------------------
# Public data types
# ---------------------------------------------------------------------------

@dataclass
class IntensityInfo:
    """Voice intensity characteristics for one audio segment."""

    start: float                 # segment start (seconds)
    end: float                   # segment end (seconds)
    rms_db: float                # mean RMS in dBFS for this segment
    rms_relative: float          # segment RMS / baseline RMS  (1.0 = average)
    intensity_level: str         # "loud" | "normal" | "quiet"
    has_sudden_increase: bool    # True if energy spikes within segment
    is_emotional: bool           # True if loud + energetically variable


# ---------------------------------------------------------------------------
# Analyzer
# ---------------------------------------------------------------------------

class VoiceIntensityAnalyzer:
    """Measure voice intensity for a given audio window.

    Parameters
    ----------
    loud_threshold:
        Ratio of segment RMS to baseline above which the segment is "loud".
        Default 1.8 (80 % louder than average).
    quiet_threshold:
        Ratio below which the segment is "quiet".  Default 0.5.
    spike_threshold:
        Frame-to-frame relative increase (Δ RMS / prev RMS) that counts as a
        sudden spike.  Default 1.5 (150 % jump).
    frame_duration:
        Duration of each analysis frame in seconds.
    """

    def __init__(
        self,
        loud_threshold: float = 1.8,
        quiet_threshold: float = 0.5,
        spike_threshold: float = 1.5,
        frame_duration: float = 0.05,
    ) -> None:
        self._loud_threshold = loud_threshold
        self._quiet_threshold = quiet_threshold
        self._spike_threshold = spike_threshold
        self._frame_duration = frame_duration

    # ------------------------------------------------------------------
    # Public interface
    # ------------------------------------------------------------------

    def compute_baseline(self, audio: np.ndarray, sample_rate: int) -> float:
        """Compute the RMS baseline (median frame RMS) for the full track."""
        if audio.ndim != 1:
            audio = audio.mean(axis=1)
        frame_len = max(1, int(self._frame_duration * sample_rate))
        rms_frames = self._frame_rms(audio, frame_len)
        # Use median so loud sections don't skew the baseline
        baseline = float(np.median(rms_frames))
        logger.debug("VoiceIntensityAnalyzer: baseline RMS = {:.4f}", baseline)
        return max(1e-6, baseline)  # avoid division by zero

    def analyze(
        self,
        audio: np.ndarray,
        sample_rate: int,
        segment_start: float,
        segment_end: float,
        baseline_rms: Optional[float] = None,
    ) -> IntensityInfo:
        """Analyse a single transcript segment window.

        Parameters
        ----------
        audio:
            Full track PCM array (1-D, float32, normalised to [−1, 1]).
        sample_rate:
            Sample rate in Hz.
        segment_start:
            Start of the transcript segment in seconds.
        segment_end:
            End of the transcript segment in seconds.
        baseline_rms:
            Pre-computed track baseline.  If *None*, computed on the fly.

        Returns
        -------
        IntensityInfo
        """
        if audio.ndim != 1:
            audio = audio.mean(axis=1)

        if baseline_rms is None:
            baseline_rms = self.compute_baseline(audio, sample_rate)

        # Extract the segment slice
        s_start = max(0, int(segment_start * sample_rate))
        s_end = min(len(audio), int(segment_end * sample_rate))
        chunk = audio[s_start:s_end]

        if len(chunk) == 0:
            return IntensityInfo(
                start=segment_start,
                end=segment_end,
                rms_db=-60.0,
                rms_relative=0.0,
                intensity_level="quiet",
                has_sudden_increase=False,
                is_emotional=False,
            )

        frame_len = max(1, int(self._frame_duration * sample_rate))
        rms_frames = self._frame_rms(chunk, frame_len)

        mean_rms = float(np.mean(rms_frames))
        rms_db = self._to_db(mean_rms)
        rms_relative = mean_rms / baseline_rms

        intensity_level = self._classify_intensity(rms_relative)
        has_sudden_increase = self._detect_spike(rms_frames)
        is_emotional = intensity_level == "loud" and float(np.std(rms_frames)) > 0.3 * mean_rms

        result = IntensityInfo(
            start=segment_start,
            end=segment_end,
            rms_db=round(rms_db, 2),
            rms_relative=round(rms_relative, 3),
            intensity_level=intensity_level,
            has_sudden_increase=has_sudden_increase,
            is_emotional=is_emotional,
        )
        logger.debug(
            "VoiceIntensityAnalyzer [{:.2f}–{:.2f}]: level={} rms_rel={:.2f} spike={} emotional={}",
            segment_start, segment_end,
            result.intensity_level, result.rms_relative,
            result.has_sudden_increase, result.is_emotional,
        )
        return result

    def analyze_segments(
        self,
        audio: np.ndarray,
        sample_rate: int,
        segments: List[tuple[float, float]],
    ) -> List[IntensityInfo]:
        """Analyse a list of (start, end) pairs, sharing one baseline."""
        baseline = self.compute_baseline(audio, sample_rate)
        return [
            self.analyze(audio, sample_rate, start, end, baseline_rms=baseline)
            for start, end in segments
        ]

    # ------------------------------------------------------------------
    # Private helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _frame_rms(chunk: np.ndarray, frame_len: int) -> np.ndarray:
        n_frames = max(1, len(chunk) // frame_len)
        trimmed = chunk[: n_frames * frame_len]
        frames = trimmed.reshape(n_frames, frame_len)
        rms: np.ndarray = np.sqrt(np.mean(frames ** 2, axis=1))
        return rms

    @staticmethod
    def _to_db(rms: float) -> float:
        """Convert linear RMS to dBFS.  Clamps at −60 dBFS."""
        if rms <= 0:
            return -60.0
        return max(-60.0, 20.0 * float(np.log10(rms)))

    def _classify_intensity(self, rms_relative: float) -> str:
        if rms_relative >= self._loud_threshold:
            return "loud"
        if rms_relative <= self._quiet_threshold:
            return "quiet"
        return "normal"

    def _detect_spike(self, rms_frames: np.ndarray) -> bool:
        """Return True if any frame is self._spike_threshold× higher than the previous."""
        if len(rms_frames) < 2:
            return False
        prev = rms_frames[:-1]
        # Avoid division by near-zero
        safe_prev = np.where(prev < 1e-6, 1e-6, prev)
        ratios = rms_frames[1:] / safe_prev
        return bool(np.any(ratios >= self._spike_threshold))
