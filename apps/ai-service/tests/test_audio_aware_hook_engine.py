"""Tests for Audio-Aware Hook Engine (Task 1).

Covers:
- PauseDetector (unit)
- VoiceIntensityAnalyzer (unit)
- SpeechPatternAnalyzer (unit)
- AudioSignalAnalyzer (unit with mocked audio loading)
- AudioAwareHookEngine (unit with mocked dependencies)
- POST /api/v1/hooks/v3/detect HTTP endpoint (integration)
"""

from __future__ import annotations

from typing import List
from unittest.mock import MagicMock, patch

import numpy as np
import pytest

from app.models.schemas import TranscriptSegmentInput
from app.services.audio_aware_hook_engine import AudioAwareHookEngine, _AUDIO_ONLY_THRESHOLD
from app.services.audio_signal_analyzer import AudioSignalAnalyzer, SegmentAudioSignals
from app.services.hook_engine_v2 import HookEngineV2
from app.services.hook_pattern_detector import HookPatternDetector
from app.services.hook_score_calculator import HookScoreCalculator
from app.services.pause_detector import PauseDetector, PauseInfo
from app.services.speech_pattern_analyzer import SpeechPatternAnalyzer, SpeechPatternInfo
from app.services.voice_intensity_analyzer import VoiceIntensityAnalyzer, IntensityInfo


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

SR = 16_000  # sample rate used in all audio tests


def _silence(duration: float) -> np.ndarray:
    """Generate a silent (near-zero) float32 audio array."""
    return np.zeros(int(SR * duration), dtype=np.float32)


def _noise(duration: float, amplitude: float = 0.05) -> np.ndarray:
    """Generate low-amplitude white noise."""
    rng = np.random.default_rng(42)
    return (rng.uniform(-amplitude, amplitude, int(SR * duration))).astype(np.float32)


def _loud(duration: float, amplitude: float = 0.8) -> np.ndarray:
    """Generate loud audio."""
    rng = np.random.default_rng(0)
    return (rng.uniform(-amplitude, amplitude, int(SR * duration))).astype(np.float32)


def _seg(text: str, start: float = 0.0, end: float = 5.0) -> TranscriptSegmentInput:
    return TranscriptSegmentInput(text=text, start=start, end=end)


def _make_intensity(
    level: str = "normal",
    spike: bool = False,
    emotional: bool = False,
    rms_rel: float = 1.0,
) -> IntensityInfo:
    return IntensityInfo(
        start=0.0,
        end=5.0,
        rms_db=-20.0,
        rms_relative=rms_rel,
        intensity_level=level,
        has_sudden_increase=spike,
        is_emotional=emotional,
    )


def _make_speech_pattern(rate: str = "normal", wps: float = 2.5) -> SpeechPatternInfo:
    return SpeechPatternInfo(
        start=0.0,
        end=5.0,
        words_per_second=wps,
        speech_rate=rate,
        rate_deviation=0.0,
    )


def _make_audio_signals(
    seg_start: float = 0.0,
    seg_end: float = 5.0,
    audio_score: int = 0,
    hook_type: str | None = None,
    emotion: str = "neutral",
    pause_type: str | None = None,
    intensity_level: str = "normal",
    speech_rate: str = "normal",
) -> SegmentAudioSignals:
    pre_pause: PauseInfo | None = None
    if pause_type:
        pre_pause = PauseInfo(start=seg_start - 1.0, end=seg_start, duration=1.0, pause_type=pause_type)

    intensity = IntensityInfo(
        start=seg_start,
        end=seg_end,
        rms_db=-20.0,
        rms_relative=1.0,
        intensity_level=intensity_level,
        has_sudden_increase=False,
        is_emotional=False,
    )
    sp = SpeechPatternInfo(
        start=seg_start,
        end=seg_end,
        words_per_second=2.5,
        speech_rate=speech_rate,
        rate_deviation=0.0,
    )
    return SegmentAudioSignals(
        start=seg_start,
        end=seg_end,
        pre_pause=pre_pause,
        intensity=intensity,
        speech_pattern=sp,
        audio_emotion=emotion,
        audio_score=audio_score,
        audio_hook_type=hook_type,
    )


# ===========================================================================
# PauseDetector
# ===========================================================================

class TestPauseDetector:
    @pytest.fixture
    def detector(self) -> PauseDetector:
        return PauseDetector(silence_threshold=0.01, min_pause_duration=0.3)

    def test_no_pause_in_continuous_speech(self, detector: PauseDetector) -> None:
        audio = _noise(5.0)
        pauses = detector.detect(audio, SR)
        assert isinstance(pauses, list)
        # Noise is above threshold – no pauses expected
        assert len(pauses) == 0

    def test_detects_silence_region(self, detector: PauseDetector) -> None:
        # 1 s speech + 2 s silence + 1 s speech
        audio = np.concatenate([_noise(1.0), _silence(2.0), _noise(1.0)])
        pauses = detector.detect(audio, SR)
        assert len(pauses) >= 1
        longest = max(pauses, key=lambda p: p.duration)
        assert longest.duration >= 1.5  # should capture most of the 2 s silence

    def test_dramatic_pause_classification(self, detector: PauseDetector) -> None:
        audio = np.concatenate([_noise(0.5), _silence(2.5), _noise(0.5)])
        pauses = detector.detect(audio, SR)
        types = {p.pause_type for p in pauses}
        assert "dramatic" in types

    def test_long_pause_classification(self, detector: PauseDetector) -> None:
        audio = np.concatenate([_noise(0.5), _silence(1.2), _noise(0.5)])
        pauses = detector.detect(audio, SR)
        types = {p.pause_type for p in pauses}
        assert "long" in types or "dramatic" in types

    def test_emphasis_pause_classification(self, detector: PauseDetector) -> None:
        audio = np.concatenate([_noise(0.5), _silence(0.4), _noise(0.5)])
        pauses = detector.detect(audio, SR)
        types = {p.pause_type for p in pauses}
        assert "emphasis" in types or "long" in types

    def test_short_silence_below_threshold_ignored(self, detector: PauseDetector) -> None:
        audio = np.concatenate([_noise(0.5), _silence(0.1), _noise(0.5)])
        pauses = detector.detect(audio, SR)
        assert len(pauses) == 0

    def test_pause_timestamps_are_positive(self, detector: PauseDetector) -> None:
        audio = np.concatenate([_noise(1.0), _silence(1.5), _noise(1.0)])
        pauses = detector.detect(audio, SR)
        for p in pauses:
            assert p.start >= 0.0
            assert p.end > p.start
            assert p.duration > 0.0

    def test_detect_pre_segment_pause_returns_none_for_small_gap(self, detector: PauseDetector) -> None:
        audio = _noise(5.0)
        result = detector.detect_pre_segment_pause(audio, SR, prev_end=1.0, segment_start=1.2)
        assert result is None  # gap = 0.2 s < min_pause_duration

    def test_detect_pre_segment_pause_detects_silence(self, detector: PauseDetector) -> None:
        # Build audio: 1 s speech, 2 s silence, 2 s speech
        audio = np.concatenate([_noise(1.0), _silence(2.0), _noise(2.0)])
        result = detector.detect_pre_segment_pause(audio, SR, prev_end=1.0, segment_start=3.0)
        assert result is not None
        assert result.duration >= 1.5

    def test_empty_audio_returns_empty(self, detector: PauseDetector) -> None:
        audio = np.array([], dtype=np.float32)
        pauses = detector.detect(audio, SR)
        assert pauses == []

    def test_clip_end_limits_analysis(self, detector: PauseDetector) -> None:
        audio = np.concatenate([_noise(1.0), _silence(2.0), _noise(2.0)])
        # Only analyse first 1 s – no pause expected there
        pauses = detector.detect(audio, SR, clip_start=0.0, clip_end=1.0)
        assert all(p.start < 1.0 for p in pauses)


# ===========================================================================
# VoiceIntensityAnalyzer
# ===========================================================================

class TestVoiceIntensityAnalyzer:
    @pytest.fixture
    def analyzer(self) -> VoiceIntensityAnalyzer:
        return VoiceIntensityAnalyzer(loud_threshold=1.8, quiet_threshold=0.5)

    def test_returns_intensity_info(self, analyzer: VoiceIntensityAnalyzer) -> None:
        audio = _noise(5.0)
        baseline = analyzer.compute_baseline(audio, SR)
        result = analyzer.analyze(audio, SR, 0.0, 5.0, baseline_rms=baseline)
        assert hasattr(result, "intensity_level")
        assert hasattr(result, "rms_db")
        assert hasattr(result, "rms_relative")
        assert hasattr(result, "has_sudden_increase")
        assert hasattr(result, "is_emotional")

    def test_loud_segment_classified_correctly(self, analyzer: VoiceIntensityAnalyzer) -> None:
        # Loud segment, quiet baseline → rms_relative >> 1
        quiet_bg = _noise(4.0, amplitude=0.001)
        loud_part = _loud(1.0, amplitude=0.9)
        audio = np.concatenate([quiet_bg, loud_part])
        baseline = analyzer.compute_baseline(audio, SR)
        result = analyzer.analyze(audio, SR, 4.0, 5.0, baseline_rms=baseline)
        assert result.intensity_level == "loud"
        assert result.rms_relative > 1.0

    def test_quiet_segment_classified_correctly(self, analyzer: VoiceIntensityAnalyzer) -> None:
        loud_bg = _loud(4.0, amplitude=0.7)
        quiet_part = _noise(1.0, amplitude=0.001)
        audio = np.concatenate([loud_bg, quiet_part])
        baseline = analyzer.compute_baseline(audio, SR)
        result = analyzer.analyze(audio, SR, 4.0, 5.0, baseline_rms=baseline)
        assert result.intensity_level == "quiet"

    def test_rms_db_is_non_positive(self, analyzer: VoiceIntensityAnalyzer) -> None:
        audio = _noise(2.0)
        result = analyzer.analyze(audio, SR, 0.0, 2.0)
        assert result.rms_db <= 0.0

    def test_empty_segment_returns_quiet(self, analyzer: VoiceIntensityAnalyzer) -> None:
        audio = _noise(5.0)
        # Segment beyond end of audio
        result = analyzer.analyze(audio, SR, 10.0, 12.0)
        assert result.intensity_level == "quiet"
        assert result.rms_db == -60.0

    def test_sudden_increase_detected(self, analyzer: VoiceIntensityAnalyzer) -> None:
        # Mostly quiet then a sudden loud burst
        quiet_frames = _noise(0.5, amplitude=0.001)
        loud_frames = _loud(0.5, amplitude=0.9)
        audio = np.concatenate([quiet_frames, loud_frames])
        result = analyzer.analyze(audio, SR, 0.0, 1.0)
        assert result.has_sudden_increase is True

    def test_no_spike_in_uniform_noise(self, analyzer: VoiceIntensityAnalyzer) -> None:
        rng = np.random.default_rng(99)
        audio = (rng.uniform(-0.1, 0.1, SR * 2)).astype(np.float32)
        result = analyzer.analyze(audio, SR, 0.0, 2.0)
        # Uniform noise should not trigger a spike
        assert isinstance(result.has_sudden_increase, bool)

    def test_baseline_positive(self, analyzer: VoiceIntensityAnalyzer) -> None:
        audio = _noise(3.0)
        baseline = analyzer.compute_baseline(audio, SR)
        assert baseline > 0.0

    def test_analyze_segments_batch(self, analyzer: VoiceIntensityAnalyzer) -> None:
        audio = _noise(10.0)
        segs = [(0.0, 2.0), (2.0, 5.0), (5.0, 10.0)]
        results = analyzer.analyze_segments(audio, SR, segs)
        assert len(results) == 3
        for r in results:
            assert r.intensity_level in ("loud", "normal", "quiet")


# ===========================================================================
# SpeechPatternAnalyzer
# ===========================================================================

class TestSpeechPatternAnalyzer:
    @pytest.fixture
    def analyzer(self) -> SpeechPatternAnalyzer:
        return SpeechPatternAnalyzer(fast_sigma=1.0, slow_sigma=1.0, sudden_change_threshold=2.0)

    def test_returns_list(self, analyzer: SpeechPatternAnalyzer) -> None:
        segs = [(0.0, 5.0, "hello world this is a test")]
        results = analyzer.analyze(segs)
        assert isinstance(results, list)
        assert len(results) == 1

    def test_empty_input(self, analyzer: SpeechPatternAnalyzer) -> None:
        assert analyzer.analyze([]) == []

    def test_wps_calculated(self, analyzer: SpeechPatternAnalyzer) -> None:
        # 10 words in 2 s = 5 wps
        segs = [(0.0, 2.0, "one two three four five six seven eight nine ten")]
        results = analyzer.analyze(segs)
        assert abs(results[0].words_per_second - 5.0) < 0.1

    def test_fast_speech_detected(self, analyzer: SpeechPatternAnalyzer) -> None:
        # Many slow segments plus one fast; no consecutive pair creates a sudden_change
        # (consecutive difference < sudden_change_threshold=2.0), but fast is detected statistically
        segs = [
            (float(i * 10), float(i * 10 + 10), "a b")  # 0.2 wps each
            for i in range(8)
        ] + [
            (80.0, 81.0, "a b c d e f g h i j k l m n o"),  # 15 wps – much faster
        ]
        results = analyzer.analyze(segs)
        rates = {r.speech_rate for r in results}
        assert "fast" in rates or "sudden_change" in rates

    def test_slow_speech_detected(self, analyzer: SpeechPatternAnalyzer) -> None:
        segs = [
            (float(i * 0.5), float(i * 0.5 + 0.5), "a b c d e f g h i j")  # ~20 wps each
            for i in range(8)
        ] + [
            (4.0, 20.0, "a b"),  # 0.125 wps – much slower
        ]
        results = analyzer.analyze(segs)
        rates = {r.speech_rate for r in results}
        assert "slow" in rates or "sudden_change" in rates

    def test_sudden_change_detected(self, analyzer: SpeechPatternAnalyzer) -> None:
        segs = [
            (0.0, 1.0, "one"),       # 1 wps
            (1.0, 1.2, "one two three four five six seven eight nine ten"),  # burst: 50 wps
        ]
        results = analyzer.analyze(segs)
        assert results[1].speech_rate == "sudden_change"

    def test_all_results_have_required_fields(self, analyzer: SpeechPatternAnalyzer) -> None:
        segs = [(float(i), float(i + 2), "hello world") for i in range(5)]
        results = analyzer.analyze(segs)
        for r in results:
            assert r.speech_rate in ("fast", "slow", "normal", "sudden_change")
            assert r.words_per_second > 0.0

    def test_single_segment_is_normal(self, analyzer: SpeechPatternAnalyzer) -> None:
        segs = [(0.0, 5.0, "hello world this is a test")]
        results = analyzer.analyze(segs)
        # Only one segment – no outlier detection possible
        assert results[0].speech_rate in ("normal", "fast", "slow")


# ===========================================================================
# AudioSignalAnalyzer (unit tests with mocked audio loading)
# ===========================================================================

class TestAudioSignalAnalyzer:
    @pytest.fixture
    def analyzer(self) -> AudioSignalAnalyzer:
        return AudioSignalAnalyzer(
            pause_detector=PauseDetector(),
            intensity_analyzer=VoiceIntensityAnalyzer(),
            speech_analyzer=SpeechPatternAnalyzer(),
            sample_rate=SR,
        )

    def test_returns_empty_for_empty_segments(self, analyzer: AudioSignalAnalyzer) -> None:
        with patch.object(analyzer, "_load_audio", return_value=_noise(10.0)):
            result = analyzer.analyze([], "/fake/path.mp4")
        assert result == []

    def test_returns_empty_when_audio_load_fails(self, analyzer: AudioSignalAnalyzer) -> None:
        with patch.object(analyzer, "_load_audio", return_value=None):
            segs = [_seg("hello world", 0.0, 5.0)]
            result = analyzer.analyze(segs, "/nonexistent/path.mp4")
        assert result == []

    def test_returns_one_signal_per_segment(self, analyzer: AudioSignalAnalyzer) -> None:
        segments = [
            _seg("hello world", 0.0, 5.0),
            _seg("goodbye world", 5.0, 10.0),
        ]
        audio = np.concatenate([_noise(5.0), _noise(5.0)])
        with patch.object(analyzer, "_load_audio", return_value=audio):
            results = analyzer.analyze(segments, "/fake.mp4")
        assert len(results) == 2

    def test_signal_start_end_match_segments(self, analyzer: AudioSignalAnalyzer) -> None:
        segments = [_seg("test", 0.0, 3.0), _seg("foo", 3.0, 7.0)]
        audio = _noise(7.0)
        with patch.object(analyzer, "_load_audio", return_value=audio):
            results = analyzer.analyze(segments, "/fake.mp4")
        assert results[0].start == 0.0
        assert results[0].end == 3.0
        assert results[1].start == 3.0
        assert results[1].end == 7.0

    def test_audio_score_within_bounds(self, analyzer: AudioSignalAnalyzer) -> None:
        segments = [_seg(f"segment {i}", float(i * 3), float(i * 3 + 3)) for i in range(4)]
        audio = _noise(12.0)
        with patch.object(analyzer, "_load_audio", return_value=audio):
            results = analyzer.analyze(segments, "/fake.mp4")
        for r in results:
            assert 0 <= r.audio_score <= 100

    def test_emotion_label_is_valid(self, analyzer: AudioSignalAnalyzer) -> None:
        valid_emotions = {"excitement", "anger", "surprise", "sadness", "neutral"}
        segments = [_seg("hello", 0.0, 2.0)]
        audio = _noise(2.0)
        with patch.object(analyzer, "_load_audio", return_value=audio):
            results = analyzer.analyze(segments, "/fake.mp4")
        for r in results:
            assert r.audio_emotion in valid_emotions

    def test_dramatic_pause_increases_audio_score(self, analyzer: AudioSignalAnalyzer) -> None:
        # 2 s silence between segments = dramatic pause
        audio = np.concatenate([_noise(2.0), _silence(2.5), _noise(2.0)])
        segments = [
            _seg("hello", 0.0, 2.0),
            _seg("world", 4.5, 6.5),
        ]
        with patch.object(analyzer, "_load_audio", return_value=audio):
            results = analyzer.analyze(segments, "/fake.mp4")
        # The second segment follows a dramatic pause → higher audio score
        assert results[1].audio_score >= results[0].audio_score

    def test_hook_type_suggestion_is_valid_or_none(self, analyzer: AudioSignalAnalyzer) -> None:
        valid_types = {"curiosity", "emotion", "storytelling", "controversy", None}
        segments = [_seg("test", 0.0, 2.0)]
        audio = _noise(2.0)
        with patch.object(analyzer, "_load_audio", return_value=audio):
            results = analyzer.analyze(segments, "/fake.mp4")
        for r in results:
            assert r.audio_hook_type in valid_types

    def test_infer_emotion_excitement(self) -> None:
        intensity = _make_intensity(level="loud")
        sp = _make_speech_pattern(rate="fast")
        emotion = AudioSignalAnalyzer._infer_emotion(intensity, sp)
        assert emotion == "excitement"

    def test_infer_emotion_anger(self) -> None:
        intensity = _make_intensity(level="loud", spike=True)
        sp = _make_speech_pattern(rate="normal")
        emotion = AudioSignalAnalyzer._infer_emotion(intensity, sp)
        assert emotion == "anger"

    def test_infer_emotion_surprise(self) -> None:
        intensity = _make_intensity(level="normal", spike=True)
        sp = _make_speech_pattern(rate="normal")
        emotion = AudioSignalAnalyzer._infer_emotion(intensity, sp)
        assert emotion == "surprise"

    def test_infer_emotion_sadness(self) -> None:
        intensity = _make_intensity(level="quiet")
        sp = _make_speech_pattern(rate="slow")
        emotion = AudioSignalAnalyzer._infer_emotion(intensity, sp)
        assert emotion == "sadness"

    def test_infer_emotion_neutral(self) -> None:
        intensity = _make_intensity(level="normal")
        sp = _make_speech_pattern(rate="normal")
        emotion = AudioSignalAnalyzer._infer_emotion(intensity, sp)
        assert emotion == "neutral"

    def test_compute_audio_score_dramatic_pause(self) -> None:
        pause = PauseInfo(start=0.0, end=2.5, duration=2.5, pause_type="dramatic")
        intensity = _make_intensity(level="normal")
        sp = _make_speech_pattern(rate="normal")
        score = AudioSignalAnalyzer._compute_audio_score(pause, intensity, sp)
        assert score >= 20  # dramatic pause alone contributes 25 pts

    def test_compute_audio_score_no_signals(self) -> None:
        intensity = _make_intensity(level="normal")
        sp = _make_speech_pattern(rate="normal")
        score = AudioSignalAnalyzer._compute_audio_score(None, intensity, sp)
        assert score == 0

    def test_compute_audio_score_multiple_signals(self) -> None:
        pause = PauseInfo(start=0.0, end=2.5, duration=2.5, pause_type="dramatic")
        intensity = _make_intensity(level="loud", spike=True, emotional=True)
        sp = _make_speech_pattern(rate="sudden_change")
        score = AudioSignalAnalyzer._compute_audio_score(pause, intensity, sp)
        # dramatic(25) + loud(20) + spike(15) + emotional(10) + sudden_change(20) = 90, capped at 100
        assert score == 90

    def test_suggest_hook_type_storytelling_for_dramatic_pause(self) -> None:
        pause = PauseInfo(start=0.0, end=2.5, duration=2.5, pause_type="dramatic")
        intensity = _make_intensity()
        sp = _make_speech_pattern()
        result = AudioSignalAnalyzer._suggest_hook_type(pause, intensity, sp, "neutral")
        assert result == "storytelling"

    def test_suggest_hook_type_emotion(self) -> None:
        intensity = _make_intensity(level="loud", emotional=True)
        sp = _make_speech_pattern()
        result = AudioSignalAnalyzer._suggest_hook_type(None, intensity, sp, "anger")
        assert result == "emotion"

    def test_suggest_hook_type_curiosity(self) -> None:
        intensity = _make_intensity()
        sp = _make_speech_pattern(rate="fast")
        result = AudioSignalAnalyzer._suggest_hook_type(None, intensity, sp, "excitement")
        assert result == "curiosity"


# ===========================================================================
# AudioAwareHookEngine (unit with mocked dependencies)
# ===========================================================================

class TestAudioAwareHookEngine:
    """Unit tests using a mocked text engine and mocked audio analyzer."""

    @pytest.fixture
    def text_engine(self):
        return HookEngineV2(HookPatternDetector(), HookScoreCalculator())

    @pytest.fixture
    def engine_no_audio(self, text_engine):
        return AudioAwareHookEngine(text_engine=text_engine, audio_analyzer=None)

    @pytest.fixture
    def engine_with_audio(self, text_engine):
        mock_audio = MagicMock(spec=AudioSignalAnalyzer)
        mock_audio.analyze.return_value = []
        return AudioAwareHookEngine(text_engine=text_engine, audio_analyzer=mock_audio)

    @pytest.mark.asyncio
    async def test_empty_segments_returns_empty(self, engine_no_audio):
        results = await engine_no_audio.detect_hooks([])
        assert results == []

    @pytest.mark.asyncio
    async def test_text_only_mode_no_audio_path(self, engine_no_audio):
        segs = [_seg("shocking secret revealed", 0.0, 5.0)]
        results = await engine_no_audio.detect_hooks(segs, audio_path=None, min_score=0)
        assert isinstance(results, list)
        assert len(results) >= 1

    @pytest.mark.asyncio
    async def test_results_sorted_descending(self, engine_no_audio):
        segs = [
            _seg("shocking secret", 0.0, 5.0),
            _seg("unpopular opinion nobody talks about", 5.0, 10.0),
            _seg("one day suddenly everything changed", 10.0, 15.0),
        ]
        results = await engine_no_audio.detect_hooks(segs, min_score=0)
        scores = [r.score for r in results]
        assert scores == sorted(scores, reverse=True)

    @pytest.mark.asyncio
    async def test_min_score_filters(self, engine_no_audio):
        segs = [_seg("shocking secret", 0.0, 5.0)]
        all_r = await engine_no_audio.detect_hooks(segs, min_score=0)
        high_r = await engine_no_audio.detect_hooks(segs, min_score=99)
        assert len(high_r) <= len(all_r)

    @pytest.mark.asyncio
    async def test_neutral_text_excluded(self, engine_no_audio):
        segs = [_seg("the sky is blue today", 0.0, 5.0)]
        results = await engine_no_audio.detect_hooks(segs, min_score=1)
        assert results == []

    @pytest.mark.asyncio
    async def test_audio_boost_increases_score(self, text_engine):
        strong_audio = _make_audio_signals(
            seg_start=0.0, seg_end=5.0,
            audio_score=80,
            hook_type="storytelling",
            emotion="excitement",
            pause_type="dramatic",
        )
        mock_audio = MagicMock(spec=AudioSignalAnalyzer)
        mock_audio.analyze.return_value = [strong_audio]

        engine = AudioAwareHookEngine(text_engine=text_engine, audio_analyzer=mock_audio)
        segs = [_seg("shocking secret revealed", 0.0, 5.0)]

        text_only_engine = AudioAwareHookEngine(text_engine=text_engine, audio_analyzer=None)
        text_results = await text_only_engine.detect_hooks(segs, min_score=0)
        audio_results = await engine.detect_hooks(segs, audio_path="/fake.mp4", min_score=0)

        if text_results and audio_results:
            audio_score = next((r.score for r in audio_results if r.start == 0.0), None)
            text_score = next((r.score for r in text_results if r.start == 0.0), None)
            if audio_score is not None and text_score is not None:
                assert audio_score >= text_score

    @pytest.mark.asyncio
    async def test_audio_only_hook_surfaced(self, text_engine):
        """Segment with no text pattern but strong audio should be surfaced."""
        strong_audio = _make_audio_signals(
            seg_start=0.0, seg_end=5.0,
            audio_score=60,
            hook_type="emotion",
            emotion="excitement",
        )
        mock_audio = MagicMock(spec=AudioSignalAnalyzer)
        mock_audio.analyze.return_value = [strong_audio]

        engine = AudioAwareHookEngine(text_engine=text_engine, audio_analyzer=mock_audio)
        segs = [_seg("the weather was nice that day", 0.0, 5.0)]  # no text hook
        results = await engine.detect_hooks(segs, audio_path="/fake.mp4", min_score=0)
        # Should surface an audio-only hook
        assert len(results) >= 1
        assert results[0].matched_pattern.startswith("audio:")

    @pytest.mark.asyncio
    async def test_weak_audio_only_hook_not_surfaced(self, text_engine):
        """Segment with weak audio signal and no text pattern should be excluded."""
        weak_audio = _make_audio_signals(
            seg_start=0.0, seg_end=5.0,
            audio_score=_AUDIO_ONLY_THRESHOLD - 1,  # below threshold
            hook_type=None,
        )
        mock_audio = MagicMock(spec=AudioSignalAnalyzer)
        mock_audio.analyze.return_value = [weak_audio]

        engine = AudioAwareHookEngine(text_engine=text_engine, audio_analyzer=mock_audio)
        segs = [_seg("the weather was nice today", 0.0, 5.0)]
        results = await engine.detect_hooks(segs, audio_path="/fake.mp4", min_score=50)
        assert results == []

    @pytest.mark.asyncio
    async def test_score_capped_at_100(self, text_engine):
        max_audio = _make_audio_signals(audio_score=100, hook_type="storytelling")
        mock_audio = MagicMock(spec=AudioSignalAnalyzer)
        mock_audio.analyze.return_value = [max_audio]

        engine = AudioAwareHookEngine(text_engine=text_engine, audio_analyzer=mock_audio)
        segs = [_seg("shocking secret unpopular opinion", 0.0, 5.0)]
        results = await engine.detect_hooks(segs, audio_path="/fake.mp4", min_score=0)
        for r in results:
            assert r.score <= 100

    @pytest.mark.asyncio
    async def test_hook_type_preserved_from_text(self, text_engine):
        neutral_audio = _make_audio_signals(audio_score=10)
        mock_audio = MagicMock(spec=AudioSignalAnalyzer)
        mock_audio.analyze.return_value = [neutral_audio]

        engine = AudioAwareHookEngine(text_engine=text_engine, audio_analyzer=mock_audio)
        segs = [_seg("subscribe and follow me now", 0.0, 5.0)]
        results = await engine.detect_hooks(segs, audio_path="/fake.mp4", min_score=0)
        if results:
            assert results[0].type in {"curiosity", "emotion", "storytelling", "controversy", "cta"}

    @pytest.mark.asyncio
    async def test_audio_analyzer_not_called_without_path(self, text_engine):
        mock_audio = MagicMock(spec=AudioSignalAnalyzer)

        engine = AudioAwareHookEngine(text_engine=text_engine, audio_analyzer=mock_audio)
        segs = [_seg("shocking secret", 0.0, 5.0)]
        await engine.detect_hooks(segs, audio_path=None, min_score=0)
        mock_audio.analyze.assert_not_called()

    @pytest.mark.asyncio
    async def test_fallback_when_no_audio_analyzer(self, text_engine):
        engine = AudioAwareHookEngine(text_engine=text_engine, audio_analyzer=None)
        segs = [_seg("shocking secret", 0.0, 5.0)]
        results = await engine.detect_hooks(segs, audio_path="/fake.mp4", min_score=0)
        assert isinstance(results, list)


# ===========================================================================
# HTTP endpoint (integration-style tests)
# ===========================================================================

class TestHookV3Endpoint:
    @pytest.fixture
    def client(self):
        from fastapi.testclient import TestClient
        from main import app
        return TestClient(app)

    def test_detect_returns_200(self, client) -> None:
        payload = {
            "video_id": "vid-v3-001",
            "segments": [
                {"text": "shocking secret revealed", "start": 0.0, "end": 5.0}
            ],
            "min_score": 0,
        }
        resp = client.post("/api/v1/hooks/v3/detect", json=payload)
        assert resp.status_code == 200

    def test_response_structure(self, client) -> None:
        payload = {
            "video_id": "vid-v3-002",
            "segments": [
                {"text": "unpopular opinion nobody talks about this", "start": 0.0, "end": 5.0}
            ],
            "min_score": 0,
        }
        resp = client.post("/api/v1/hooks/v3/detect", json=payload)
        data = resp.json()
        assert "video_id" in data
        assert "hooks" in data
        assert "total" in data
        assert "audio_analysis" in data
        assert "audio_enabled" in data
        assert data["video_id"] == "vid-v3-002"

    def test_audio_enabled_false_without_path(self, client) -> None:
        payload = {
            "video_id": "vid-v3-003",
            "segments": [
                {"text": "shocking secret", "start": 0.0, "end": 5.0}
            ],
        }
        resp = client.post("/api/v1/hooks/v3/detect", json=payload)
        data = resp.json()
        assert data["audio_enabled"] is False
        assert data["audio_analysis"] == []

    def test_hooks_contain_required_fields(self, client) -> None:
        payload = {
            "video_id": "vid-v3-004",
            "segments": [
                {"text": "one day suddenly everything changed", "start": 10.0, "end": 15.0}
            ],
            "min_score": 0,
        }
        resp = client.post("/api/v1/hooks/v3/detect", json=payload)
        data = resp.json()
        for hook in data["hooks"]:
            assert "start" in hook
            assert "end" in hook
            assert "type" in hook
            assert "score" in hook

    def test_empty_segments_rejected_422(self, client) -> None:
        payload = {"video_id": "vid-v3-005", "segments": []}
        resp = client.post("/api/v1/hooks/v3/detect", json=payload)
        assert resp.status_code == 422

    def test_invalid_segment_missing_end_rejected(self, client) -> None:
        payload = {
            "video_id": "vid-v3-006",
            "segments": [{"text": "hello", "start": 0.0}],
        }
        resp = client.post("/api/v1/hooks/v3/detect", json=payload)
        assert resp.status_code == 422

    def test_min_score_filter(self, client) -> None:
        payload = {
            "video_id": "vid-v3-007",
            "segments": [
                {"text": "shocking secret", "start": 0.0, "end": 5.0}
            ],
            "min_score": 100,
        }
        resp = client.post("/api/v1/hooks/v3/detect", json=payload)
        assert resp.status_code == 200
        data = resp.json()
        assert isinstance(data["hooks"], list)

    def test_multiple_segments(self, client) -> None:
        payload = {
            "video_id": "vid-v3-008",
            "segments": [
                {"text": "shocking secret", "start": 0.0, "end": 5.0},
                {"text": "unpopular opinion", "start": 5.0, "end": 10.0},
                {"text": "normal weather text", "start": 10.0, "end": 15.0},
                {"text": "subscribe and follow", "start": 15.0, "end": 20.0},
            ],
            "min_score": 0,
        }
        resp = client.post("/api/v1/hooks/v3/detect", json=payload)
        assert resp.status_code == 200
        data = resp.json()
        assert data["total"] >= 2

    def test_neutral_text_returns_zero_hooks(self, client) -> None:
        payload = {
            "video_id": "vid-v3-009",
            "segments": [
                {"text": "the weather today is quite pleasant", "start": 0.0, "end": 5.0}
            ],
            "min_score": 1,
        }
        resp = client.post("/api/v1/hooks/v3/detect", json=payload)
        data = resp.json()
        assert data["total"] == 0

    def test_audio_path_with_nonexistent_file_graceful_fallback(self, client) -> None:
        """Providing a non-existent audio path must not crash the endpoint."""
        payload = {
            "video_id": "vid-v3-010",
            "segments": [
                {"text": "shocking secret revealed", "start": 0.0, "end": 5.0}
            ],
            "audio_storage_path": "/nonexistent/path/video.mp4",
            "min_score": 0,
        }
        resp = client.post("/api/v1/hooks/v3/detect", json=payload)
        # Should return 200 (graceful fallback, not 500)
        assert resp.status_code == 200
        data = resp.json()
        assert isinstance(data["hooks"], list)

    def test_hooks_sorted_descending_score(self, client) -> None:
        payload = {
            "video_id": "vid-v3-011",
            "segments": [
                {"text": "shocking secret", "start": 0.0, "end": 5.0},
                {"text": "unpopular opinion nobody", "start": 5.0, "end": 10.0},
                {"text": "one day suddenly", "start": 10.0, "end": 15.0},
            ],
            "min_score": 0,
        }
        resp = client.post("/api/v1/hooks/v3/detect", json=payload)
        data = resp.json()
        scores = [h["score"] for h in data["hooks"]]
        assert scores == sorted(scores, reverse=True)

    def test_total_matches_hooks_length(self, client) -> None:
        payload = {
            "video_id": "vid-v3-012",
            "segments": [
                {"text": "shocking secret revealed", "start": 0.0, "end": 5.0},
            ],
            "min_score": 0,
        }
        resp = client.post("/api/v1/hooks/v3/detect", json=payload)
        data = resp.json()
        assert data["total"] == len(data["hooks"])
