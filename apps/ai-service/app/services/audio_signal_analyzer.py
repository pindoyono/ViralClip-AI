"""Audio Signal Analyzer for Audio-Aware Hook Engine.

Orchestrates :class:`PauseDetector`, :class:`VoiceIntensityAnalyzer`, and
:class:`SpeechPatternAnalyzer` to produce per-segment audio signals.

Audio loading uses *ffmpeg* (already a project dependency) via a subprocess
call that decodes to 16 kHz mono PCM.  The class degrades gracefully when the
audio file is absent or ffmpeg fails – callers receive an empty signal list.

Emotion label heuristic (rule-based, no ML model required)
-----------------------------------------------------------
    excitement  – fast speech + loud voice
    anger       – loud voice + sudden volume increase
    surprise    – sudden volume increase + not slow speech
    sadness     – slow speech + quiet voice
    neutral     – none of the above
"""

from __future__ import annotations

import subprocess
from dataclasses import dataclass
from typing import List, Optional, Sequence

import numpy as np
from loguru import logger

from app.models.schemas import TranscriptSegmentInput
from app.services.pause_detector import PauseDetector, PauseInfo
from app.services.speech_pattern_analyzer import SpeechPatternAnalyzer, SpeechPatternInfo
from app.services.voice_intensity_analyzer import VoiceIntensityAnalyzer, IntensityInfo


# ---------------------------------------------------------------------------
# Public data types
# ---------------------------------------------------------------------------

@dataclass
class SegmentAudioSignals:
    """Aggregated audio signals for one transcript segment."""

    start: float
    end: float

    # Pause signals
    pre_pause: Optional[PauseInfo]      # pause immediately before this segment

    # Intensity signals
    intensity: IntensityInfo

    # Speech pattern signals
    speech_pattern: SpeechPatternInfo

    # Derived emotion label
    audio_emotion: str                  # "excitement" | "anger" | "surprise" | "sadness" | "neutral"

    # Combined audio score (0–100)
    audio_score: int

    # Suggested hook type driven by audio (None when audio signal is weak)
    audio_hook_type: Optional[str]


# ---------------------------------------------------------------------------
# Analyzer
# ---------------------------------------------------------------------------

_SAMPLE_RATE = 16_000   # Hz – matches Whisper's default input rate


class AudioSignalAnalyzer:
    """Load an audio/video file and compute per-segment audio signals.

    Parameters
    ----------
    pause_detector:
        Injected :class:`PauseDetector` instance.
    intensity_analyzer:
        Injected :class:`VoiceIntensityAnalyzer` instance.
    speech_analyzer:
        Injected :class:`SpeechPatternAnalyzer` instance.
    sample_rate:
        PCM sample rate to request from ffmpeg.
    """

    def __init__(
        self,
        pause_detector: PauseDetector,
        intensity_analyzer: VoiceIntensityAnalyzer,
        speech_analyzer: SpeechPatternAnalyzer,
        sample_rate: int = _SAMPLE_RATE,
    ) -> None:
        self._pause_detector = pause_detector
        self._intensity_analyzer = intensity_analyzer
        self._speech_analyzer = speech_analyzer
        self._sample_rate = sample_rate

    # ------------------------------------------------------------------
    # Public interface
    # ------------------------------------------------------------------

    def analyze(
        self,
        segments: List[TranscriptSegmentInput],
        audio_path: str,
    ) -> List[SegmentAudioSignals]:
        """Return audio signals for every transcript segment.

        Parameters
        ----------
        segments:
            Ordered transcript segments.  Must not be empty.
        audio_path:
            Absolute path to an audio or video file that ffmpeg can read.

        Returns
        -------
        List[SegmentAudioSignals]
            One entry per segment, in the same order.  Returns an empty
            list (and logs a warning) if the file cannot be loaded.
        """
        if not segments:
            return []

        audio = self._load_audio(audio_path)
        if audio is None:
            logger.warning("AudioSignalAnalyzer: could not load audio from {!r}", audio_path)
            return []

        baseline = self._intensity_analyzer.compute_baseline(audio, self._sample_rate)

        # Prepare speech-pattern input
        sp_input = [(seg.start, seg.end, seg.text) for seg in segments]
        speech_patterns = self._speech_analyzer.analyze(sp_input)

        results: List[SegmentAudioSignals] = []
        for i, seg in enumerate(segments):
            # Pause detection: gap between previous segment end and this start
            prev_end = segments[i - 1].end if i > 0 else None
            pre_pause: Optional[PauseInfo] = None
            if prev_end is not None and seg.start > prev_end:
                pre_pause = self._pause_detector.detect_pre_segment_pause(
                    audio, self._sample_rate, prev_end, seg.start
                )

            # Intensity analysis for the segment window
            intensity = self._intensity_analyzer.analyze(
                audio, self._sample_rate, seg.start, seg.end, baseline_rms=baseline
            )

            sp = speech_patterns[i]
            emotion = self._infer_emotion(intensity, sp)
            score = self._compute_audio_score(pre_pause, intensity, sp)
            hook_type = self._suggest_hook_type(pre_pause, intensity, sp, emotion)

            results.append(
                SegmentAudioSignals(
                    start=seg.start,
                    end=seg.end,
                    pre_pause=pre_pause,
                    intensity=intensity,
                    speech_pattern=sp,
                    audio_emotion=emotion,
                    audio_score=score,
                    audio_hook_type=hook_type,
                )
            )

        logger.info(
            "AudioSignalAnalyzer: processed {} segments from {!r}",
            len(results), audio_path,
        )
        return results

    # ------------------------------------------------------------------
    # Private helpers
    # ------------------------------------------------------------------

    def _load_audio(self, path: str) -> Optional[np.ndarray]:
        """Decode *path* to a 1-D float32 PCM array via ffmpeg.

        Returns *None* on any failure (file not found, decode error, etc.).
        """
        cmd = [
            "ffmpeg",
            "-hide_banner", "-loglevel", "error",
            "-i", path,
            "-vn",                          # drop video
            "-acodec", "pcm_s16le",         # 16-bit signed LE PCM
            "-ar", str(self._sample_rate),  # resample
            "-ac", "1",                     # mono
            "-f", "s16le",
            "pipe:1",                       # write raw PCM to stdout
        ]
        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                timeout=120,
            )
            if result.returncode != 0:
                logger.warning(
                    "AudioSignalAnalyzer: ffmpeg exit {} for {!r}: {}",
                    result.returncode, path, result.stderr.decode(errors="replace")[:200],
                )
                return None
            samples = np.frombuffer(result.stdout, dtype=np.int16).astype(np.float32)
            if len(samples) == 0:
                logger.warning("AudioSignalAnalyzer: ffmpeg produced no samples for {!r}", path)
                return None
            return samples / 32768.0  # normalise to [−1, 1]
        except FileNotFoundError:
            logger.error("AudioSignalAnalyzer: ffmpeg not found in PATH")
            return None
        except subprocess.TimeoutExpired:
            logger.error("AudioSignalAnalyzer: ffmpeg timed out for {!r}", path)
            return None
        except Exception as exc:
            logger.error("AudioSignalAnalyzer: unexpected error loading {!r}: {}", path, exc)
            return None

    @staticmethod
    def _infer_emotion(intensity: IntensityInfo, sp: SpeechPatternInfo) -> str:
        """Map audio features to a high-level emotion label."""
        loud = intensity.intensity_level == "loud"
        quiet = intensity.intensity_level == "quiet"
        spike = intensity.has_sudden_increase
        fast = sp.speech_rate == "fast"
        slow = sp.speech_rate == "slow"

        if loud and fast:
            return "excitement"
        if loud and spike:
            return "anger"
        if spike and not slow:
            return "surprise"
        if quiet and slow:
            return "sadness"
        return "neutral"

    @staticmethod
    def _compute_audio_score(
        pre_pause: Optional[PauseInfo],
        intensity: IntensityInfo,
        sp: SpeechPatternInfo,
    ) -> int:
        """Compute a 0–100 audio signal score for the segment."""
        score = 0.0

        # Pause contribution (up to 25 pts)
        if pre_pause is not None:
            if pre_pause.pause_type == "dramatic":
                score += 25.0
            elif pre_pause.pause_type == "long":
                score += 15.0
            else:  # emphasis
                score += 8.0

        # Intensity contribution (up to 25 pts)
        if intensity.intensity_level == "loud":
            score += 20.0
        if intensity.has_sudden_increase:
            score += 15.0
        if intensity.is_emotional:
            score += 10.0

        # Speech pattern contribution (up to 20 pts)
        if sp.speech_rate == "sudden_change":
            score += 20.0
        elif sp.speech_rate == "fast":
            score += 10.0
        elif sp.speech_rate == "slow":
            score += 8.0

        return max(0, min(100, round(score)))

    @staticmethod
    def _suggest_hook_type(
        pre_pause: Optional[PauseInfo],
        intensity: IntensityInfo,
        sp: SpeechPatternInfo,
        emotion: str,
    ) -> Optional[str]:
        """Suggest the most appropriate hook type based on audio signals.

        Returns *None* when the audio signal is too weak to drive a hook.
        """
        loud = intensity.intensity_level == "loud"
        spike = intensity.has_sudden_increase
        dramatic_pause = pre_pause is not None and pre_pause.pause_type == "dramatic"

        if dramatic_pause or sp.speech_rate == "slow":
            return "storytelling"
        if emotion in ("anger", "sadness") or (loud and intensity.is_emotional):
            return "emotion"
        if emotion in ("excitement", "surprise") or sp.speech_rate in ("fast", "sudden_change"):
            return "curiosity"
        if spike and loud:
            return "controversy"
        return None


# ---------------------------------------------------------------------------
# Factory
# ---------------------------------------------------------------------------

def build_audio_signal_analyzer() -> AudioSignalAnalyzer:
    """Return a default-configured :class:`AudioSignalAnalyzer`."""
    return AudioSignalAnalyzer(
        pause_detector=PauseDetector(),
        intensity_analyzer=VoiceIntensityAnalyzer(),
        speech_analyzer=SpeechPatternAnalyzer(),
    )
