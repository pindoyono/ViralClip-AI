import os
import uuid
from pathlib import Path
from loguru import logger


def ensure_dir(path: str) -> str:
    """Create directory if it doesn't exist and return the path."""
    Path(path).mkdir(parents=True, exist_ok=True)
    return path


def generate_temp_path(base_dir: str, extension: str) -> str:
    """Generate a unique temporary file path."""
    ensure_dir(base_dir)
    filename = f"{uuid.uuid4().hex}{extension}"
    return os.path.join(base_dir, filename)


def clean_temp_file(path: str) -> None:
    """Safely remove a temporary file."""
    try:
        if path and os.path.exists(path):
            os.remove(path)
            logger.debug(f"Cleaned up temp file: {path}")
    except Exception as e:
        logger.warning(f"Failed to clean up temp file {path}: {e}")


def human_readable_size(num_bytes: int) -> str:
    """Convert bytes to human-readable string."""
    for unit in ["B", "KB", "MB", "GB"]:
        if num_bytes < 1024.0:
            return f"{num_bytes:.1f} {unit}"
        num_bytes /= 1024.0
    return f"{num_bytes:.1f} TB"


def safe_filename(name: str) -> str:
    """Sanitize a string to be a safe filename."""
    keepchars = (" ", ".", "_", "-")
    return "".join(c for c in name if c.isalnum() or c in keepchars).rstrip()
