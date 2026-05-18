"""Pause Detector for Audio-Aware Hook Engine.

Detects speech pauses in an audio signal by analysing the RMS energy per
short frame.  Pauses are classified by duration:

- **dramatic** – gap ≥ 2.0 s   (narrative beat, tension)
- **long**      – gap ≥ 1.0 s   (topic boundary, breath)
- **emphasis**  – gap ≥ 0.3 s   (word stress, micro-pause)

All timestamps are in *seconds* and refer to the same time-base as the
transcript segments.
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
class PauseInfo:
    """A detected speech pause between two voiced regions."""

    start: float           # pause start (seconds)
    end: float             # pause end (seconds)
    duration: float        # pause length in seconds
    pause_type: str        # "dramatic" | "long" | "emphasis"


# ---------------------------------------------------------------------------
# Detector
# ---------------------------------------------------------------------------

class PauseDetector:
    """Detect and classify speech pauses from a mono audio array.

    Parameters
    ----------
    silence_threshold:
        RMS level (0–1 linear) below which a frame is considered silent.
        Default 0.01 (~−40 dBFS).
    frame_duration:
        Duration of each RMS analysis frame in seconds.  Smaller values
        give finer resolution at the cost of more computation.
    min_pause_duration:
        Minimum contiguous silence to report as a pause (seconds).
    """

    _DRAMATIC_THRESHOLD: float = 2.0
    _LONG_THRESHOLD: float = 1.0
    _EMPHASIS_THRESHOLD: float = 0.3

    def __init__(
        self,
        silence_threshold: float = 0.01,
        frame_duration: float = 0.02,
        min_pause_duration: float = 0.3,
    ) -> None:
        self._silence_threshold = silence_threshold
        self._frame_duration = frame_duration
        self._min_pause_duration = min_pause_duration

    # ------------------------------------------------------------------
    # Public interface
    # ------------------------------------------------------------------

    def detect(
        self,
        audio: np.ndarray,
        sample_rate: int,
        clip_start: float = 0.0,
        clip_end: Optional[float] = None,
    ) -> List[PauseInfo]:
        """Return all pauses found in *audio* within [clip_start, clip_end].

        Parameters
        ----------
        audio:
            1-D float32 array of PCM samples normalised to [−1, 1].
        sample_rate:
            Sample rate of *audio* in Hz.
        clip_start:
            Start of the region to analyse (seconds).  Defaults to 0.
        clip_end:
            End of the region to analyse (seconds).  Defaults to end of
            the audio array.

        Returns
        -------
        List[PauseInfo]
            Pauses sorted by start time.
        """
        if audio.ndim != 1:
            audio = audio.mean(axis=1)  # downmix to mono

        total_duration = len(audio) / sample_rate
        if clip_end is None:
            clip_end = total_duration

        # Extract slice
        s_start = max(0, int(clip_start * sample_rate))
        s_end = min(len(audio), int(clip_end * sample_rate))
        chunk = audio[s_start:s_end]

        if len(chunk) == 0:
            logger.debug("PauseDetector: empty audio chunk for [{:.2f}, {:.2f}]", clip_start, clip_end)
            return []

        frame_len = max(1, int(self._frame_duration * sample_rate))
        frames = self._frame_rms(chunk, frame_len)
        is_silent = frames < self._silence_threshold

        pauses = self._extract_pause_regions(is_silent, frame_len, sample_rate, clip_start)
        logger.debug(
            "PauseDetector: detected {} pauses in [{:.2f}, {:.2f}]",
            len(pauses), clip_start, clip_end,
        )
        return pauses

    def detect_pre_segment_pause(
        self,
        audio: np.ndarray,
        sample_rate: int,
        prev_end: float,
        segment_start: float,
    ) -> Optional[PauseInfo]:
        """Detect a pause in the gap *before* a transcript segment.

        Analyses the window [prev_end, segment_start].  Returns *None* if
        the gap is below *min_pause_duration*.
        """
        gap = segment_start - prev_end
        if gap < self._min_pause_duration:
            return None

        pauses = self.detect(audio, sample_rate, clip_start=prev_end, clip_end=segment_start)
        if not pauses:
            return None
        # Return the longest pause in the gap
        return max(pauses, key=lambda p: p.duration)

    # ------------------------------------------------------------------
    # Private helpers
    # ------------------------------------------------------------------

    @staticmethod
    def _frame_rms(chunk: np.ndarray, frame_len: int) -> np.ndarray:
        """Compute RMS for consecutive non-overlapping frames."""
        # Pad to multiple of frame_len
        n_frames = max(1, len(chunk) // frame_len)
        trimmed = chunk[: n_frames * frame_len]
        frames = trimmed.reshape(n_frames, frame_len)
        rms: np.ndarray = np.sqrt(np.mean(frames ** 2, axis=1))
        return rms

    def _extract_pause_regions(
        self,
        is_silent: np.ndarray,
        frame_len: int,
        sample_rate: int,
        time_offset: float,
    ) -> List[PauseInfo]:
        """Convert a boolean silence mask into PauseInfo objects."""
        pauses: List[PauseInfo] = []
        in_pause = False
        pause_start_frame = 0

        for i, silent in enumerate(is_silent):
            if silent and not in_pause:
                in_pause = True
                pause_start_frame = i
            elif not silent and in_pause:
                in_pause = False
                pause_end_frame = i
                pause_sec = self._frames_to_pause(
                    pause_start_frame, pause_end_frame, frame_len, sample_rate, time_offset
                )
                if pause_sec is not None:
                    pauses.append(pause_sec)

        # Close any open pause at the end
        if in_pause:
            pause_sec = self._frames_to_pause(
                pause_start_frame, len(is_silent), frame_len, sample_rate, time_offset
            )
            if pause_sec is not None:
                pauses.append(pause_sec)

        return pauses

    def _frames_to_pause(
        self,
        start_frame: int,
        end_frame: int,
        frame_len: int,
        sample_rate: int,
        time_offset: float,
    ) -> Optional[PauseInfo]:
        """Convert frame indices to a PauseInfo, or None if too short."""
        start_sec = time_offset + (start_frame * frame_len) / sample_rate
        end_sec = time_offset + (end_frame * frame_len) / sample_rate
        duration = end_sec - start_sec

        if duration < self._min_pause_duration:
            return None

        if duration >= self._DRAMATIC_THRESHOLD:
            pause_type = "dramatic"
        elif duration >= self._LONG_THRESHOLD:
            pause_type = "long"
        else:
            pause_type = "emphasis"

        return PauseInfo(start=start_sec, end=end_sec, duration=duration, pause_type=pause_type)
