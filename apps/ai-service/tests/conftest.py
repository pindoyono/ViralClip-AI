"""Pytest configuration - pre-import all service modules so patch targets resolve."""
import importlib
import sys


def pytest_configure(config):
    """Import service modules before any test collection so `patch` targets work."""
    # Ensure modules are loaded into sys.modules under their full dotted names
    for mod in [
        "app.services.transcript_service",
        "app.services.clip_service",
        "app.services.hook_service",
        "app.services.metadata_service",
        "app.services.subtitle_service",
        "app.services.category_service",
    ]:
        try:
            importlib.import_module(mod)
        except Exception:
            pass  # will surface as import error in the test itself
