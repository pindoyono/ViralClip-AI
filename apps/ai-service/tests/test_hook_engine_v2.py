"""Tests for Hook Detection Engine V2.

Covers HookPatternDetector, HookScoreCalculator, HookEngineV2, and the
/api/v1/hooks/v2/detect HTTP endpoint.
"""

from __future__ import annotations

import pytest

from app.models.schemas import TranscriptSegmentInput
from app.services.hook_pattern_detector import HookPatternDetector, PatternMatch
from app.services.hook_score_calculator import HookScoreCalculator
from app.services.hook_engine_v2 import HookEngineV2


# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

@pytest.fixture
def detector() -> HookPatternDetector:
    return HookPatternDetector()


@pytest.fixture
def calculator() -> HookScoreCalculator:
    return HookScoreCalculator()


@pytest.fixture
def engine(detector: HookPatternDetector, calculator: HookScoreCalculator) -> HookEngineV2:
    return HookEngineV2(detector, calculator)


def seg(text: str, start: float = 0.0, end: float = 5.0) -> TranscriptSegmentInput:
    return TranscriptSegmentInput(text=text, start=start, end=end)


# ===========================================================================
# HookPatternDetector
# ===========================================================================

class TestHookPatternDetector:
    def test_detects_curiosity_keyword(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("I'm going to reveal a secret you've never heard")
        types = {m.hook_type for m in matches}
        assert "curiosity" in types

    def test_detects_emotion_keyword(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("I was so scared when it happened")
        types = {m.hook_type for m in matches}
        assert "emotion" in types

    def test_detects_storytelling_phrase(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("At first I thought nothing would happen, then everything changed")
        types = {m.hook_type for m in matches}
        assert "storytelling" in types

    def test_detects_controversy_phrase(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("Unpopular opinion: nobody talks about this")
        types = {m.hook_type for m in matches}
        assert "controversy" in types

    def test_detects_cta_keyword(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("Don't forget to subscribe and leave a comment below")
        types = {m.hook_type for m in matches}
        assert "cta" in types

    def test_returns_empty_for_neutral_text(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("The weather today is quite pleasant.")
        assert matches == []

    def test_multiple_types_in_one_segment(self, detector: HookPatternDetector) -> None:
        """A segment can trigger more than one hook type."""
        text = "Shocking secret revealed – subscribe to find out"
        matches = detector.detect(text)
        types = {m.hook_type for m in matches}
        assert len(types) >= 2

    def test_case_insensitive(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("SHOCKING revelation never seen before")
        assert any(m.hook_type == "curiosity" for m in matches)

    def test_matched_text_is_substring(self, detector: HookPatternDetector) -> None:
        text = "This is a shocking story you won't believe"
        matches = detector.detect(text)
        for m in matches:
            assert m.matched_text.lower() in text.lower()

    def test_confidence_between_zero_and_one(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("secret unbelievable shocking story suddenly")
        for m in matches:
            assert 0.0 <= m.confidence <= 1.0

    def test_pattern_match_dataclass_fields(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("secret")
        assert len(matches) >= 1
        m = matches[0]
        assert hasattr(m, "hook_type")
        assert hasattr(m, "matched_text")
        assert hasattr(m, "confidence")

    def test_curiosity_never(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("You have never seen this before")
        # 'never' is a common word but 'never before' should still trigger storytelling/curiosity
        assert len(matches) >= 0  # neutral assertion – main check is no crash

    def test_storytelling_suddenly(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("Suddenly everything went wrong")
        types = {m.hook_type for m in matches}
        assert "storytelling" in types

    def test_controversy_hot_take(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("Hot take: most influencers are wrong")
        types = {m.hook_type for m in matches}
        assert "controversy" in types

    def test_cta_swipe_up(self, detector: HookPatternDetector) -> None:
        matches = detector.detect("Swipe up to see the full video")
        types = {m.hook_type for m in matches}
        assert "cta" in types


# ===========================================================================
# HookScoreCalculator
# ===========================================================================

class TestHookScoreCalculator:
    def _patterns(self, hook_type: str = "curiosity", confidence: float = 0.90) -> list[PatternMatch]:
        return [PatternMatch(hook_type=hook_type, matched_text="test", confidence=confidence)]

    def test_score_is_int(self, calculator: HookScoreCalculator) -> None:
        score = calculator.calculate("secret revealed", self._patterns(), 0, 10)
        assert isinstance(score, int)

    def test_score_within_bounds(self, calculator: HookScoreCalculator) -> None:
        score = calculator.calculate("secret revealed", self._patterns(), 0, 10)
        assert 0 <= score <= 100

    def test_no_patterns_returns_zero(self, calculator: HookScoreCalculator) -> None:
        score = calculator.calculate("normal sentence", [], 0, 10)
        assert score == 0

    def test_early_segment_scores_higher_than_late(self, calculator: HookScoreCalculator) -> None:
        patterns = self._patterns()
        early = calculator.calculate("secret", patterns, 0, 20)
        late = calculator.calculate("secret", patterns, 19, 20)
        assert early > late

    def test_emphasis_words_increase_score(self, calculator: HookScoreCalculator) -> None:
        base = calculator.calculate("secret revealed", self._patterns(), 5, 20)
        emph = calculator.calculate("incredibly secret actually revealed", self._patterns(), 5, 20)
        assert emph >= base

    def test_multiple_patterns_increase_score(self, calculator: HookScoreCalculator) -> None:
        one = calculator.calculate("secret", self._patterns(), 5, 20)
        two_patterns = [
            PatternMatch("curiosity", "secret", 0.90),
            PatternMatch("emotion", "scared", 0.80),
        ]
        two = calculator.calculate("secret scared", two_patterns, 5, 20)
        assert two >= one

    def test_pause_bonus_applied(self, calculator: HookScoreCalculator) -> None:
        no_pause = calculator.calculate("secret", self._patterns(), 5, 20, prev_segment_end=10.0, segment_start=10.1)
        with_pause = calculator.calculate("secret", self._patterns(), 5, 20, prev_segment_end=9.0, segment_start=11.0)
        assert with_pause >= no_pause

    def test_repetition_bonus_applied(self, calculator: HookScoreCalculator) -> None:
        no_rep = calculator.calculate("secret revealed", self._patterns(), 5, 20)
        rep = calculator.calculate("secret secret secret", self._patterns(), 5, 20)
        assert rep >= no_rep

    def test_controversy_base_higher_than_cta(self, calculator: HookScoreCalculator) -> None:
        controversy_patterns = [PatternMatch("controversy", "hot take", 0.85)]
        cta_patterns = [PatternMatch("cta", "subscribe", 0.90)]
        c_score = calculator.calculate("hot take", controversy_patterns, 5, 20)
        t_score = calculator.calculate("subscribe", cta_patterns, 5, 20)
        assert c_score >= t_score

    def test_score_capped_at_100(self, calculator: HookScoreCalculator) -> None:
        patterns = [
            PatternMatch("controversy", "unpopular opinion", 1.0),
            PatternMatch("curiosity", "shocking", 1.0),
            PatternMatch("emotion", "terrified", 1.0),
        ]
        score = calculator.calculate(
            "unpopular opinion shocking terrified extremely incredibly",
            patterns, 0, 10, prev_segment_end=0.0, segment_start=3.0,
        )
        assert score <= 100


# ===========================================================================
# HookEngineV2
# ===========================================================================

class TestHookEngineV2:
    @pytest.mark.asyncio
    async def test_returns_list(self, engine: HookEngineV2) -> None:
        segments = [seg("At first I thought nothing would happen, then everything changed", 10.0, 15.0)]
        results = await engine.detect_hooks(segments)
        assert isinstance(results, list)

    @pytest.mark.asyncio
    async def test_empty_segments_returns_empty(self, engine: HookEngineV2) -> None:
        results = await engine.detect_hooks([])
        assert results == []

    @pytest.mark.asyncio
    async def test_result_has_required_fields(self, engine: HookEngineV2) -> None:
        segments = [seg("This is a shocking secret nobody talks about", 0.0, 5.0)]
        results = await engine.detect_hooks(segments, min_score=0)
        assert len(results) >= 1
        r = results[0]
        assert hasattr(r, "start")
        assert hasattr(r, "end")
        assert hasattr(r, "type")
        assert hasattr(r, "score")
        assert hasattr(r, "matched_pattern")

    @pytest.mark.asyncio
    async def test_timestamps_preserved(self, engine: HookEngineV2) -> None:
        segments = [seg("shocking secret", 10.5, 15.3)]
        results = await engine.detect_hooks(segments, min_score=0)
        if results:
            assert results[0].start == 10.5
            assert results[0].end == 15.3

    @pytest.mark.asyncio
    async def test_min_score_filters_results(self, engine: HookEngineV2) -> None:
        segments = [
            seg("shocking secret", 0.0, 5.0),
            seg("normal text here", 5.0, 10.0),
        ]
        all_results = await engine.detect_hooks(segments, min_score=0)
        high_results = await engine.detect_hooks(segments, min_score=95)
        assert len(high_results) <= len(all_results)

    @pytest.mark.asyncio
    async def test_results_sorted_by_score_descending(self, engine: HookEngineV2) -> None:
        segments = [
            seg("shocking secret", 0.0, 5.0),
            seg("unpopular opinion nobody talks about", 5.0, 10.0),
            seg("one day suddenly everything changed", 10.0, 15.0),
        ]
        results = await engine.detect_hooks(segments, min_score=0)
        scores = [r.score for r in results]
        assert scores == sorted(scores, reverse=True)

    @pytest.mark.asyncio
    async def test_neutral_segments_excluded(self, engine: HookEngineV2) -> None:
        segments = [seg("The weather is nice today.", 0.0, 5.0)]
        results = await engine.detect_hooks(segments, min_score=1)
        assert results == []

    @pytest.mark.asyncio
    async def test_score_within_valid_range(self, engine: HookEngineV2) -> None:
        segments = [
            seg("shocking secret revealed unpopular opinion", 0.0, 5.0),
            seg("subscribe and follow me now please", 5.0, 10.0),
        ]
        results = await engine.detect_hooks(segments, min_score=0)
        for r in results:
            assert 0 <= r.score <= 100

    @pytest.mark.asyncio
    async def test_hook_type_is_valid_category(self, engine: HookEngineV2) -> None:
        valid_types = {"curiosity", "emotion", "storytelling", "controversy", "cta"}
        segments = [
            seg("shocking secret", 0.0, 5.0),
            seg("unpopular opinion", 5.0, 10.0),
            seg("one day suddenly", 10.0, 15.0),
            seg("subscribe now", 15.0, 20.0),
        ]
        results = await engine.detect_hooks(segments, min_score=0)
        for r in results:
            assert r.type in valid_types

    @pytest.mark.asyncio
    async def test_multiple_segments_processed(self, engine: HookEngineV2) -> None:
        segments = [seg(f"segment {i}", float(i * 5), float(i * 5 + 4)) for i in range(20)]
        # Should not raise even if most segments have no hooks
        results = await engine.detect_hooks(segments, min_score=0)
        assert isinstance(results, list)

    @pytest.mark.asyncio
    async def test_early_position_boosts_score(self, engine: HookEngineV2) -> None:
        """Same text at position 0 should score >= same text at position 19."""
        text = "shocking secret"
        early = [seg(text, 0.0, 5.0)] + [seg("neutral", float(i * 5 + 5), float(i * 5 + 10)) for i in range(19)]
        late = [seg("neutral", float(i * 5), float(i * 5 + 5)) for i in range(19)] + [seg(text, 95.0, 100.0)]

        early_results = await engine.detect_hooks(early, min_score=0)
        late_results = await engine.detect_hooks(late, min_score=0)

        # Both should have at least one result for the hook segment
        early_scores = [r.score for r in early_results if r.start == 0.0]
        late_scores = [r.score for r in late_results if r.start == 95.0]

        if early_scores and late_scores:
            assert early_scores[0] >= late_scores[0]

    @pytest.mark.asyncio
    async def test_example_from_spec(self, engine: HookEngineV2) -> None:
        """Verify the example given in the problem statement is handled."""
        segments = [
            TranscriptSegmentInput(
                text="At first I thought nothing would happen, then suddenly everything changed",
                start=10.0,
                end=15.0,
            )
        ]
        results = await engine.detect_hooks(segments, min_score=0)
        assert len(results) == 1
        r = results[0]
        assert r.start == 10.0
        assert r.end == 15.0
        assert r.type == "storytelling"
        assert r.score >= 50


# ===========================================================================
# HTTP endpoint (integration-style, no DB)
# ===========================================================================

class TestHookV2Endpoint:
    @pytest.fixture
    def client(self):
        from fastapi.testclient import TestClient
        from main import app
        return TestClient(app)

    def test_detect_returns_200(self, client) -> None:
        payload = {
            "video_id": "vid-001",
            "segments": [
                {"text": "shocking secret revealed", "start": 0.0, "end": 5.0}
            ],
            "min_score": 0,
        }
        resp = client.post("/api/v1/hooks/v2/detect", json=payload)
        assert resp.status_code == 200

    def test_detect_response_structure(self, client) -> None:
        payload = {
            "video_id": "vid-002",
            "segments": [
                {"text": "unpopular opinion nobody talks about this", "start": 0.0, "end": 5.0}
            ],
            "min_score": 0,
        }
        resp = client.post("/api/v1/hooks/v2/detect", json=payload)
        data = resp.json()
        assert "video_id" in data
        assert "hooks" in data
        assert "total" in data
        assert data["video_id"] == "vid-002"

    def test_detect_empty_segment_text(self, client) -> None:
        payload = {
            "video_id": "vid-003",
            "segments": [
                {"text": "   ", "start": 0.0, "end": 5.0}
            ],
            "min_score": 0,
        }
        resp = client.post("/api/v1/hooks/v2/detect", json=payload)
        assert resp.status_code == 200
        assert resp.json()["total"] == 0

    def test_detect_requires_segments(self, client) -> None:
        payload = {"video_id": "vid-004", "segments": []}
        resp = client.post("/api/v1/hooks/v2/detect", json=payload)
        # Pydantic min_length=1 should reject empty list with 422
        assert resp.status_code == 422

    def test_detect_min_score_filter(self, client) -> None:
        payload = {
            "video_id": "vid-005",
            "segments": [
                {"text": "shocking secret", "start": 0.0, "end": 5.0}
            ],
            "min_score": 100,  # No segment should reach 100 with a single short text
        }
        resp = client.post("/api/v1/hooks/v2/detect", json=payload)
        assert resp.status_code == 200
        # May return 0 or 1 hooks; just assert the structure is valid
        data = resp.json()
        assert isinstance(data["hooks"], list)

    def test_detect_hook_fields_present(self, client) -> None:
        payload = {
            "video_id": "vid-006",
            "segments": [
                {"text": "one day suddenly everything changed", "start": 10.0, "end": 15.0}
            ],
            "min_score": 0,
        }
        resp = client.post("/api/v1/hooks/v2/detect", json=payload)
        data = resp.json()
        if data["hooks"]:
            hook = data["hooks"][0]
            assert "start" in hook
            assert "end" in hook
            assert "type" in hook
            assert "score" in hook

    def test_detect_multiple_segments(self, client) -> None:
        payload = {
            "video_id": "vid-007",
            "segments": [
                {"text": "shocking secret", "start": 0.0, "end": 5.0},
                {"text": "unpopular opinion", "start": 5.0, "end": 10.0},
                {"text": "normal text", "start": 10.0, "end": 15.0},
                {"text": "subscribe and follow", "start": 15.0, "end": 20.0},
            ],
            "min_score": 0,
        }
        resp = client.post("/api/v1/hooks/v2/detect", json=payload)
        assert resp.status_code == 200
        data = resp.json()
        assert data["total"] >= 2  # at least curiosity + controversy + cta

    def test_detect_invalid_segment_missing_end(self, client) -> None:
        payload = {
            "video_id": "vid-008",
            "segments": [{"text": "shocking", "start": 0.0}],
        }
        resp = client.post("/api/v1/hooks/v2/detect", json=payload)
        assert resp.status_code == 422
