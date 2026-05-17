import os
from typing import List
from loguru import logger
from app.models.schemas import TranscriptSegment, SubtitleStyle
from app.utils.file_utils import generate_temp_path, ensure_dir
from app.utils.ffmpeg_utils import add_subtitles_to_video
from app.config import settings


def _segments_to_srt(segments: List[TranscriptSegment]) -> str:
    """Convert transcript segments to SRT subtitle format."""
    lines = []
    for i, seg in enumerate(segments, 1):
        start = _seconds_to_srt_time(seg.start)
        end = _seconds_to_srt_time(seg.end)
        lines.append(f"{i}\n{start} --> {end}\n{seg.text}\n")
    return "\n".join(lines)


def _seconds_to_srt_time(seconds: float) -> str:
    """Convert seconds to SRT time format (HH:MM:SS,mmm)."""
    h = int(seconds // 3600)
    m = int((seconds % 3600) // 60)
    s = int(seconds % 60)
    ms = int((seconds % 1) * 1000)
    return f"{h:02d}:{m:02d}:{s:02d},{ms:03d}"


def _get_subtitle_style_params(style: SubtitleStyle, font_size: int, primary_color: str, outline_color: str) -> str:
    """Build ASS/SSA subtitle style string."""
    base = f"Fontsize={font_size},PrimaryColour={primary_color},OutlineColour={outline_color},MarginV=30"

    if style == SubtitleStyle.BOLD:
        return f"{base},Bold=1"
    elif style == SubtitleStyle.OUTLINE:
        return f"{base},BorderStyle=1,Outline=3,Shadow=0"
    elif style == SubtitleStyle.SHADOW:
        return f"{base},BorderStyle=1,Outline=1,Shadow=3"
    else:
        return f"{base},Bold=1,BorderStyle=3,Outline=2,Shadow=1,BackColour=&H80000000"


async def burn_subtitles(
    clip_path: str,
    transcript_segments: List[TranscriptSegment],
    style: SubtitleStyle = SubtitleStyle.DEFAULT,
    font_size: int = 24,
    primary_color: str = "&H00FFFFFF",
    outline_color: str = "&H00000000",
) -> dict:
    """Burn subtitles into a clip and return paths."""
    clips_dir = os.path.join(settings.local_storage_path, "clips_with_subs")
    ensure_dir(clips_dir)

    srt_path = generate_temp_path(settings.local_storage_path + "/tmp", ".srt")
    clip_basename = os.path.splitext(os.path.basename(clip_path))[0]
    output_path = os.path.join(clips_dir, f"{clip_basename}_subtitled.mp4")

    # Write SRT file
    srt_content = _segments_to_srt(transcript_segments)
    with open(srt_path, "w", encoding="utf-8") as f:
        f.write(srt_content)

    logger.info(f"Burning subtitles into {clip_path}")

    add_subtitles_to_video(clip_path, srt_path, output_path)

    return {
        "output_path": output_path,
        "subtitle_path": srt_path,
    }
