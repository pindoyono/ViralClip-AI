"""Pytest configuration - pre-import all service modules so patch targets resolve."""
import importlib
import sys
from unittest.mock import MagicMock


def pytest_configure(config):
    """Import service modules before any test collection so `patch` targets work.

    The ``whisper`` ML library is heavy and not installed in CI; stub it so that
    the service modules can be imported without error.  ``openai`` IS available
    in the test environment and must NOT be stubbed — doing so breaks async
    mock behaviour.
    """
    if "whisper" not in sys.modules:
        sys.modules["whisper"] = MagicMock()

    # Ensure modules are loaded into sys.modules under their full dotted names
    for mod in [
        "app.services.transcript_service",
        "app.services.clip_service",
        "app.services.hook_service",
        "app.services.hook_pattern_detector",
        "app.services.hook_score_calculator",
        "app.services.hook_engine_v2",
        "app.services.emotion_analyzer",
        "app.services.story_arc_detector",
        "app.services.retention_predictor",
        "app.services.clip_score_calculator_v2",
        "app.services.clip_engine_v2",
        "app.services.metadata_service",
        "app.services.subtitle_service",
        "app.services.category_service",
        "app.routers.process",
        "app.routers.hooks_v2",
        "app.routers.clips_v2",
    ]:
        try:
            importlib.import_module(mod)
        except Exception:
            pass  # will surface as import error in the test itself
