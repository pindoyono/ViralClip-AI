"""Tests for Dynamic Clip Engine V2.

Covers EmotionAnalyzer, StoryArcDetector, RetentionPredictor,
ClipScoreCalculatorV2, ClipEngineV2, and the HTTP endpoint.
"""

from __future__ import annotations

import pytest
from typing import List

from app.models.schemas import HookDetectionResult, TranscriptSegmentInput
from app.services.emotion_analyzer import EmotionAnalyzer
from app.services.story_arc_detector import StoryArcDetector
from app.services.retention_predictor import RetentionPredictor
from app.services.clip_score_calculator_v2 import ClipScoreCalculatorV2
from app.services.clip_engine_v2 import (
    ClipEngineV2,
    ClipV2Result,
    PROFILE_DURATION_RULES,
    _seconds_to_hms,
    build_clip_engine_v2,
)


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def seg(text: str, start: float, end: float) -> TranscriptSegmentInput:
    return TranscriptSegmentInput(text=text, start=start, end=end)


def hook(start: float, end: float, score: int = 80, h_type: str = "storytelling") -> HookDetectionResult:
    return HookDetectionResult(start=start, end=end, type=h_type, score=score, matched_pattern="test")


def make_segments(n: int, seg_duration: float = 5.0) -> List[TranscriptSegmentInput]:
    """Create n neutral segments of equal duration."""
    return [seg(f"Segment {i} content text here.", i * seg_duration, (i + 1) * seg_duration) for i in range(n)]


def make_engine() -> ClipEngineV2:
    return build_clip_engine_v2()


# ===========================================================================
# _seconds_to_hms helper
# ===========================================================================

class TestSecondsToHms:
    def test_zero(self):
        assert _seconds_to_hms(0) == "00:00:00"

    def test_one_minute(self):
        assert _seconds_to_hms(60) == "00:01:00"

    def test_one_hour(self):
        assert _seconds_to_hms(3600) == "01:00:00"

    def test_mixed(self):
        assert _seconds_to_hms(3661) == "01:01:01"

    def test_two_minutes_twenty(self):
        assert _seconds_to_hms(140) == "00:02:20"

    def test_two_minutes_fifty(self):
        assert _seconds_to_hms(170) == "00:02:50"

    def test_rounding(self):
        # 90.4 rounds to 90 → 00:01:30
        assert _seconds_to_hms(90.4) == "00:01:30"

    def test_negative_clamped_to_zero(self):
        assert _seconds_to_hms(-5) == "00:00:00"


# ===========================================================================
# EmotionAnalyzer
# ===========================================================================

class TestEmotionAnalyzer:
    @pytest.fixture
    def analyzer(self) -> EmotionAnalyzer:
        return EmotionAnalyzer()

    def test_returns_int(self, analyzer):
        assert isinstance(analyzer.score("I was so angry"), int)

    def test_score_in_bounds(self, analyzer):
        for text in ["normal sentence", "I was terrified", "oh my god that was scary"]:
            assert 0 <= analyzer.score(text) <= 100

    def test_neutral_text_scores_zero(self, analyzer):
        assert analyzer.score("The weather forecast shows clouds.") == 0

    def test_high_emotion_scores_higher(self, analyzer):
        low  = analyzer.score("I was a bit worried")
        high = analyzer.score("I was completely terrified and devastated")
        assert high > low

    def test_amplifiers_increase_score(self, analyzer):
        base = analyzer.score("I was scared")
        amplified = analyzer.score("I was so incredibly scared")
        assert amplified >= base

    def test_multiple_emotions_accumulate(self, analyzer):
        single   = analyzer.score("I was angry")
        multiple = analyzer.score("I was angry and terrified and heartbroken")
        assert multiple > single

    def test_capped_at_100(self, analyzer):
        very_emotional = " ".join(["terrified devastated sobbing screaming panicked"] * 5)
        assert analyzer.score(very_emotional) == 100

    def test_case_insensitive(self, analyzer):
        lower = analyzer.score("terrified")
        upper = analyzer.score("TERRIFIED")
        assert lower == upper

    def test_omg_triggers_emotion(self, analyzer):
        assert analyzer.score("omg I can't believe that") > 0

    def test_high_tier_patterns(self, analyzer):
        assert analyzer.score("I was utterly devastated") > 50

    def test_medium_tier_patterns(self, analyzer):
        assert analyzer.score("I was surprised") > 0

    def test_low_tier_patterns(self, analyzer):
        assert analyzer.score("I was happy") > 0

    def test_empty_string_returns_zero(self, analyzer):
        assert analyzer.score("") == 0


# ===========================================================================
# StoryArcDetector
# ===========================================================================

class TestStoryArcDetector:
    @pytest.fixture
    def detector(self) -> StoryArcDetector:
        return StoryArcDetector()

    def test_returns_int(self, detector):
        assert isinstance(detector.score("one day it happened", 0, 10), int)

    def test_score_in_bounds(self, detector):
        for idx in range(10):
            assert 0 <= detector.score("some text here", idx, 10) <= 100

    def test_climax_position_scores_higher(self, detector):
        # 60% through transcript → climax zone
        climax_idx = 6
        early_idx  = 0
        climax_score = detector.score("some text", climax_idx, 10)
        early_score  = detector.score("some text", early_idx,  10)
        assert climax_score > early_score

    def test_climax_keyword_bonus(self, detector):
        base    = detector.score("some neutral text", 3, 10)
        climax  = detector.score("suddenly everything changed out of nowhere", 3, 10)
        assert climax > base

    def test_setup_keyword_detected(self, detector):
        base   = detector.score("normal text", 0, 10)
        setup  = detector.score("at first I thought one day it would work", 0, 10)
        assert setup > base

    def test_resolution_keyword_detected(self, detector):
        base       = detector.score("normal text", 9, 10)
        resolution = detector.score("in the end I learned what I needed to know", 9, 10)
        assert resolution > base

    def test_clause_complexity_bonus(self, detector):
        simple  = detector.score("I went there", 5, 10)
        complex_ = detector.score("I went there, but then I realised that although it was hard, I succeeded", 5, 10)
        assert complex_ > simple

    def test_single_segment_no_crash(self, detector):
        # total_segments = 1 should not crash
        score = detector.score("only segment", 0, 1)
        assert 0 <= score <= 100

    def test_zero_total_no_crash(self, detector):
        score = detector.score("text", 0, 0)
        assert 0 <= score <= 100

    def test_buildup_keyword_detected(self, detector):
        base    = detector.score("normal text", 3, 10)
        buildup = detector.score("and then things started to slowly change more and more", 3, 10)
        assert buildup > base


# ===========================================================================
# RetentionPredictor
# ===========================================================================

class TestRetentionPredictor:
    @pytest.fixture
    def predictor(self) -> RetentionPredictor:
        return RetentionPredictor()

    def test_returns_int(self, predictor):
        assert isinstance(predictor.score("Some interesting content here!", 30.0, 15.0, 60.0), int)

    def test_score_in_bounds(self, predictor):
        assert 0 <= predictor.score("Some content.", 30.0, 15.0, 60.0) <= 100

    def test_zero_duration_returns_zero(self, predictor):
        assert predictor.score("text", 0.0, 15.0, 60.0) == 0

    def test_ideal_duration_scores_highest(self, predictor):
        # target = (15+45)/2 = 30s for gaming
        at_target = predictor.score("content", 30.0, 15.0, 45.0)
        off_target = predictor.score("content", 44.0, 15.0, 45.0)
        assert at_target >= off_target

    def test_hook_presence_increases_score(self, predictor):
        no_hook   = predictor.score("content", 30.0, 15.0, 60.0, has_hook=False)
        with_hook = predictor.score("content", 30.0, 15.0, 60.0, has_hook=True)
        assert with_hook > no_hook

    def test_questions_increase_score(self, predictor):
        no_q   = predictor.score("This is a statement.", 30.0, 15.0, 60.0)
        with_q = predictor.score("Did you know this? Can you believe it? What happened?", 30.0, 15.0, 60.0)
        assert with_q > no_q

    def test_exclamations_increase_score(self, predictor):
        no_ex   = predictor.score("Normal text here.", 30.0, 15.0, 60.0)
        with_ex = predictor.score("Wow! Amazing! Incredible!", 30.0, 15.0, 60.0)
        assert with_ex >= no_ex

    def test_sentence_variety_increases_score(self, predictor):
        # Mix of short and long sentences → higher variety score
        uniform = predictor.score("Short. Short. Short. Short.", 30.0, 15.0, 60.0)
        varied  = predictor.score("Short. This is a much longer sentence with more words and clauses.", 30.0, 15.0, 60.0)
        assert varied >= uniform

    def test_score_capped_at_100(self, predictor):
        long_text = "Amazing! " * 20 + "Incredible? " * 20
        score = predictor.score(long_text, 30.0, 15.0, 60.0, has_hook=True)
        assert score <= 100

    def test_historical_analytics_influence_score(self, predictor):
        baseline = predictor.score(
            "Did you know this happened? Amazing!",
            35.0,
            15.0,
            60.0,
            has_hook=True,
            category="gaming",
            hook_types=["storytelling"],
        )
        with_history = predictor.score(
            "Did you know this happened? Amazing!",
            35.0,
            15.0,
            60.0,
            has_hook=True,
            category="gaming",
            hook_types=["storytelling"],
            historical_analytics={
                "sample_size": 180,
                "baseline_retention": 0.75,
                "duration_bucket_retention": {"medium": 0.82},
                "category_retention": {"gaming": 0.88},
                "hook_type_retention": {"storytelling": 0.9},
            },
        )
        assert with_history > baseline

    def test_historical_analytics_supports_hook_type_matching(self, predictor):
        storytelling_score = predictor.score(
            "Interesting story with a twist!",
            35.0,
            15.0,
            60.0,
            has_hook=True,
            category="education",
            hook_types=["storytelling"],
            historical_analytics={
                "sample_size": 120,
                "baseline_retention": 0.55,
                "duration_bucket_retention": {"medium": 0.55},
                "category_retention": {"education": 0.55},
                "hook_type_retention": {"storytelling": 0.85, "cta": 0.45},
            },
        )
        cta_score = predictor.score(
            "Interesting story with a twist!",
            35.0,
            15.0,
            60.0,
            has_hook=True,
            category="education",
            hook_types=["cta"],
            historical_analytics={
                "sample_size": 120,
                "baseline_retention": 0.55,
                "duration_bucket_retention": {"medium": 0.55},
                "category_retention": {"education": 0.55},
                "hook_type_retention": {"storytelling": 0.85, "cta": 0.45},
            },
        )
        assert storytelling_score > cta_score


# ===========================================================================
# ClipScoreCalculatorV2
# ===========================================================================

class TestClipScoreCalculatorV2:
    @pytest.fixture
    def calc(self) -> ClipScoreCalculatorV2:
        return ClipScoreCalculatorV2()

    def test_returns_int(self, calc):
        assert isinstance(calc.calculate(80.0, 60.0, 70.0, 50.0), int)

    def test_score_in_bounds(self, calc):
        assert 0 <= calc.calculate(80.0, 60.0, 70.0, 50.0) <= 100

    def test_formula_correctness(self, calc):
        # 80*0.5 + 60*0.2 + 70*0.2 + 50*0.1 = 40+12+14+5 = 71
        assert calc.calculate(80.0, 60.0, 70.0, 50.0) == 71

    def test_zero_inputs_returns_zero(self, calc):
        assert calc.calculate(0.0, 0.0, 0.0, 0.0) == 0

    def test_max_inputs_returns_100(self, calc):
        assert calc.calculate(100.0, 100.0, 100.0, 100.0) == 100

    def test_hook_dominates(self, calc):
        # High hook should dominate (50% weight)
        high_hook = calc.calculate(100.0, 0.0, 0.0, 0.0)
        high_retention = calc.calculate(0.0, 0.0, 0.0, 100.0)
        assert high_hook > high_retention

    def test_capped_at_100(self, calc):
        assert calc.calculate(200.0, 200.0, 200.0, 200.0) == 100

    def test_clamped_at_0(self, calc):
        assert calc.calculate(-100.0, -100.0, -100.0, -100.0) == 0

    def test_custom_weights(self):
        calc = ClipScoreCalculatorV2(
            hook_weight=0.25,
            emotion_weight=0.25,
            story_weight=0.25,
            retention_weight=0.25,
        )
        # 80*0.25 + 60*0.25 + 70*0.25 + 50*0.25 = 20+15+17.5+12.5 = 65
        assert calc.calculate(80.0, 60.0, 70.0, 50.0) == 65


# ===========================================================================
# ClipEngineV2
# ===========================================================================

class TestClipEngineV2:
    @pytest.fixture
    def engine(self) -> ClipEngineV2:
        return make_engine()

    @pytest.mark.asyncio
    async def test_returns_list(self, engine):
        segments = make_segments(20, 5.0)
        result = await engine.generate_clips(segments, [])
        assert isinstance(result, list)

    @pytest.mark.asyncio
    async def test_empty_segments_returns_empty(self, engine):
        result = await engine.generate_clips([], [])
        assert result == []

    @pytest.mark.asyncio
    async def test_clip_has_required_fields(self, engine):
        segments = make_segments(20, 5.0)
        segs_with_emotion = [
            seg("I was completely terrified and devastated suddenly", 0.0, 5.0),
        ] + make_segments(19, 5.0)
        result = await engine.generate_clips(segs_with_emotion, [], profile_type="general", min_clip_score=0)
        if result:
            c = result[0]
            assert hasattr(c, "start")
            assert hasattr(c, "end")
            assert hasattr(c, "score")
            assert hasattr(c, "start_seconds")
            assert hasattr(c, "end_seconds")
            assert hasattr(c, "hook_score")
            assert hasattr(c, "emotion_score")
            assert hasattr(c, "story_score")
            assert hasattr(c, "retention_score")
            assert hasattr(c, "profile_type")

    @pytest.mark.asyncio
    async def test_start_end_are_hms_strings(self, engine):
        segments = [
            seg("I was terrified suddenly everything changed", 140.0, 145.0),
        ] + make_segments(10, 10.0)
        result = await engine.generate_clips(segments, [], min_clip_score=0)
        if result:
            import re
            hms_re = re.compile(r"^\d{2}:\d{2}:\d{2}$")
            assert hms_re.match(result[0].start), f"bad start format: {result[0].start}"
            assert hms_re.match(result[0].end),   f"bad end format: {result[0].end}"

    @pytest.mark.asyncio
    async def test_score_in_bounds(self, engine):
        segments = make_segments(30, 5.0)
        result = await engine.generate_clips(segments, [], min_clip_score=0)
        for c in result:
            assert 0 <= c.score <= 100

    @pytest.mark.asyncio
    async def test_sorted_by_score_desc(self, engine):
        segments = make_segments(30, 5.0)
        hooks = [hook(0.0, 5.0, score=90), hook(50.0, 55.0, score=60)]
        result = await engine.generate_clips(segments, hooks, min_clip_score=0)
        scores = [c.score for c in result]
        assert scores == sorted(scores, reverse=True)

    @pytest.mark.asyncio
    async def test_max_clips_respected(self, engine):
        segments = make_segments(50, 5.0)
        result = await engine.generate_clips(segments, [], min_clip_score=0, max_clips=3)
        assert len(result) <= 3

    @pytest.mark.asyncio
    async def test_min_clip_score_filters(self, engine):
        segments = make_segments(20, 5.0)
        all_clips  = await engine.generate_clips(segments, [], min_clip_score=0)
        high_clips = await engine.generate_clips(segments, [], min_clip_score=99)
        assert len(high_clips) <= len(all_clips)

    @pytest.mark.asyncio
    async def test_gaming_profile_duration(self, engine):
        # 20 segments × 5s = 100s; gaming clips should be 15–45s
        segments = make_segments(20, 5.0)
        # Add emotional anchor
        segments[0] = seg("I was completely terrified suddenly everything changed", 0.0, 5.0)
        result = await engine.generate_clips(
            segments, [], profile_type="gaming", min_clip_score=0
        )
        min_d, max_d = PROFILE_DURATION_RULES["gaming"]
        for c in result:
            duration = c.end_seconds - c.start_seconds
            assert min_d <= duration <= max_d, f"gaming clip out of range: {duration}s"

    @pytest.mark.asyncio
    async def test_podcast_profile_duration(self, engine):
        # Need longer segments for podcast (45-90s)
        segments = make_segments(20, 10.0)  # 20×10s = 200s
        segments[0] = seg("I was completely devastated suddenly everything changed omg", 0.0, 10.0)
        result = await engine.generate_clips(
            segments, [], profile_type="podcast", min_clip_score=0
        )
        min_d, max_d = PROFILE_DURATION_RULES["podcast"]
        for c in result:
            duration = c.end_seconds - c.start_seconds
            assert min_d <= duration <= max_d, f"podcast clip out of range: {duration}s"

    @pytest.mark.asyncio
    async def test_comedy_profile_duration(self, engine):
        segments = make_segments(20, 5.0)
        segments[0] = seg("omg that was hilarious I can't believe it", 0.0, 5.0)
        result = await engine.generate_clips(
            segments, [], profile_type="comedy", min_clip_score=0
        )
        min_d, max_d = PROFILE_DURATION_RULES["comedy"]
        for c in result:
            duration = c.end_seconds - c.start_seconds
            assert min_d <= duration <= max_d, f"comedy clip out of range: {duration}s"

    @pytest.mark.asyncio
    async def test_hook_detection_boosts_score(self, engine):
        segments = make_segments(20, 5.0)
        without_hooks = await engine.generate_clips(segments, [], min_clip_score=0, max_clips=1)
        with_hooks = await engine.generate_clips(
            segments,
            [hook(0.0, 5.0, score=95)],
            min_clip_score=0,
            max_clips=1,
        )
        if without_hooks and with_hooks:
            assert with_hooks[0].score >= without_hooks[0].score

    @pytest.mark.asyncio
    async def test_no_overlapping_clips(self, engine):
        """Deduplication should prevent substantially overlapping clips."""
        segments = make_segments(30, 5.0)
        hooks_list = [hook(0.0, 5.0, score=90), hook(0.0, 4.0, score=85)]
        result = await engine.generate_clips(segments, hooks_list, min_clip_score=0, max_clips=10)
        # Check no two clips share > 50% overlap
        for i, a in enumerate(result):
            for j, b in enumerate(result):
                if i >= j:
                    continue
                overlap = max(0.0, min(a.end_seconds, b.end_seconds) - max(a.start_seconds, b.start_seconds))
                dur_a = a.end_seconds - a.start_seconds
                if dur_a > 0:
                    assert overlap / dur_a <= 0.50, f"clips {i} and {j} overlap too much"

    @pytest.mark.asyncio
    async def test_profile_type_stored_in_result(self, engine):
        segments = make_segments(20, 5.0)
        segments[0] = seg("terrified and devastated", 0.0, 5.0)
        result = await engine.generate_clips(segments, [], profile_type="education", min_clip_score=0)
        if result:
            assert result[0].profile_type == "education"

    @pytest.mark.asyncio
    async def test_unknown_profile_falls_back_to_general(self, engine):
        segments = make_segments(20, 5.0)
        segments[0] = seg("terrified", 0.0, 5.0)
        result = await engine.generate_clips(segments, [], profile_type="unknown", min_clip_score=0)
        assert isinstance(result, list)  # no crash

    @pytest.mark.asyncio
    async def test_politics_profile_duration(self, engine):
        segments = make_segments(20, 10.0)  # 200s total
        segments[0] = seg("terrified and devastated suddenly everything changed", 0.0, 10.0)
        result = await engine.generate_clips(
            segments, [], profile_type="politics", min_clip_score=0
        )
        min_d, max_d = PROFILE_DURATION_RULES["politics"]
        for c in result:
            duration = c.end_seconds - c.start_seconds
            assert min_d <= duration <= max_d


# ===========================================================================
# HTTP Endpoint (integration-style)
# ===========================================================================

class TestClipV2Endpoint:
    @pytest.fixture
    def client(self):
        from fastapi.testclient import TestClient
        from main import app
        return TestClient(app)

    def _payload(self, profile_type: str = "general", n_segs: int = 20) -> dict:
        segments = [
            {"text": f"Segment {i} content text here.", "start": float(i * 5), "end": float(i * 5 + 5)}
            for i in range(n_segs)
        ]
        segments[0]["text"] = "I was absolutely terrified suddenly everything changed"
        return {
            "video_id": "vid-v2",
            "segments": segments,
            "hook_detections": [],
            "profile_type": profile_type,
            "min_clip_score": 0,
            "max_clips": 5,
        }

    def test_returns_200(self, client):
        resp = client.post("/api/v1/clips/v2/generate", json=self._payload())
        assert resp.status_code == 200

    def test_response_structure(self, client):
        resp = client.post("/api/v1/clips/v2/generate", json=self._payload())
        data = resp.json()
        assert "video_id" in data
        assert "clips" in data
        assert "total" in data
        assert "profile_type" in data

    def test_clips_have_hms_format(self, client):
        resp = client.post("/api/v1/clips/v2/generate", json=self._payload())
        data = resp.json()
        import re
        hms_re = re.compile(r"^\d{2}:\d{2}:\d{2}$")
        for clip in data["clips"]:
            assert hms_re.match(clip["start"]), f"bad start: {clip['start']}"
            assert hms_re.match(clip["end"]),   f"bad end: {clip['end']}"

    def test_clips_score_in_bounds(self, client):
        resp = client.post("/api/v1/clips/v2/generate", json=self._payload())
        for c in resp.json()["clips"]:
            assert 0 <= c["score"] <= 100

    def test_empty_segments_rejected(self, client):
        payload = self._payload()
        payload["segments"] = []
        resp = client.post("/api/v1/clips/v2/generate", json=payload)
        assert resp.status_code == 422

    def test_gaming_profile_accepted(self, client):
        resp = client.post("/api/v1/clips/v2/generate", json=self._payload("gaming"))
        assert resp.status_code == 200
        assert resp.json()["profile_type"] == "gaming"

    def test_podcast_profile_accepted(self, client):
        payload = self._payload("podcast", n_segs=30)
        payload["segments"] = [
            {"text": f"Segment {i} content.", "start": float(i * 10), "end": float(i * 10 + 10)}
            for i in range(30)
        ]
        payload["segments"][0]["text"] = "Absolutely terrified devastated sobbing"
        resp = client.post("/api/v1/clips/v2/generate", json=payload)
        assert resp.status_code == 200

    def test_hook_detections_accepted(self, client):
        payload = self._payload()
        payload["hook_detections"] = [
            {"start": 0.0, "end": 5.0, "type": "storytelling", "score": 88, "matched_pattern": "suddenly"}
        ]
        resp = client.post("/api/v1/clips/v2/generate", json=payload)
        assert resp.status_code == 200

    def test_max_clips_respected_by_api(self, client):
        payload = self._payload()
        payload["max_clips"] = 2
        resp = client.post("/api/v1/clips/v2/generate", json=payload)
        assert len(resp.json()["clips"]) <= 2

    def test_video_id_preserved(self, client):
        payload = self._payload()
        payload["video_id"] = "my-special-video"
        resp = client.post("/api/v1/clips/v2/generate", json=payload)
        assert resp.json()["video_id"] == "my-special-video"

    def test_invalid_profile_type_rejected(self, client):
        payload = self._payload()
        payload["profile_type"] = "unknown_profile"
        resp = client.post("/api/v1/clips/v2/generate", json=payload)
        assert resp.status_code == 422

    def test_historical_analytics_payload_accepted(self, client):
        payload = self._payload("gaming")
        payload["historical_analytics"] = {
            "sample_size": 150,
            "baseline_retention": 0.7,
            "duration_bucket_retention": {"short": 0.6, "medium": 0.75, "long": 0.65},
            "category_retention": {"gaming": 0.8},
            "hook_type_retention": {"storytelling": 0.78},
        }
        resp = client.post("/api/v1/clips/v2/generate", json=payload)
        assert resp.status_code == 200
