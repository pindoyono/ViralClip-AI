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
        "app.services.metadata_service",
        "app.services.subtitle_service",
        "app.services.category_service",
        "app.routers.process",
    ]:
        try:
            importlib.import_module(mod)
        except Exception:
            pass  # will surface as import error in the test itself
