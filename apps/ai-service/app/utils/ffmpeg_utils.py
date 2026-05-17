import subprocess
import os
import json
from pathlib import Path
from loguru import logger
from typing import Optional, Dict, Any, Tuple


def get_video_info(video_path: str) -> Dict[str, Any]:
    """Extract video metadata using ffprobe."""
    cmd = [
        "ffprobe",
        "-v", "quiet",
        "-print_format", "json",
        "-show_format",
        "-show_streams",
        video_path,
    ]

    result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
    if result.returncode != 0:
        raise RuntimeError(f"ffprobe failed: {result.stderr}")

    data = json.loads(result.stdout)
    video_stream = next(
        (s for s in data.get("streams", []) if s["codec_type"] == "video"),
        None
    )
    audio_stream = next(
        (s for s in data.get("streams", []) if s["codec_type"] == "audio"),
        None
    )

    fmt = data.get("format", {})
    duration = float(fmt.get("duration", 0))

    return {
        "duration": duration,
        "size": int(fmt.get("size", 0)),
        "bitrate": int(fmt.get("bit_rate", 0)),
        "width": int(video_stream.get("width", 0)) if video_stream else 0,
        "height": int(video_stream.get("height", 0)) if video_stream else 0,
        "fps": eval(video_stream.get("r_frame_rate", "0/1")) if video_stream else 0.0,
        "video_codec": video_stream.get("codec_name") if video_stream else None,
        "audio_codec": audio_stream.get("codec_name") if audio_stream else None,
        "has_audio": audio_stream is not None,
    }


def extract_audio(video_path: str, output_path: str) -> str:
    """Extract audio from video as WAV for Whisper transcription."""
    Path(output_path).parent.mkdir(parents=True, exist_ok=True)

    cmd = [
        "ffmpeg",
        "-i", video_path,
        "-vn",
        "-acodec", "pcm_s16le",
        "-ar", "16000",
        "-ac", "1",
        "-y",
        output_path,
    ]

    result = subprocess.run(cmd, capture_output=True, text=True, timeout=300)
    if result.returncode != 0:
        raise RuntimeError(f"FFmpeg audio extraction failed: {result.stderr}")

    logger.info(f"Audio extracted to {output_path}")
    return output_path


def extract_clip(
    video_path: str,
    output_path: str,
    start_time: float,
    end_time: float,
    add_fade: bool = True,
) -> str:
    """Extract a clip segment from a video."""
    Path(output_path).parent.mkdir(parents=True, exist_ok=True)

    duration = end_time - start_time
    fade_duration = min(0.5, duration * 0.05)

    filter_complex = ""
    if add_fade:
        filter_complex = (
            f"[0:v]fade=t=in:st=0:d={fade_duration},"
            f"fade=t=out:st={duration - fade_duration}:d={fade_duration}[v];"
            f"[0:a]afade=t=in:st=0:d={fade_duration},"
            f"afade=t=out:st={duration - fade_duration}:d={fade_duration}[a]"
        )
        cmd = [
            "ffmpeg",
            "-ss", str(start_time),
            "-i", video_path,
            "-t", str(duration),
            "-filter_complex", filter_complex,
            "-map", "[v]",
            "-map", "[a]",
            "-c:v", "libx264",
            "-crf", "23",
            "-preset", "fast",
            "-c:a", "aac",
            "-b:a", "128k",
            "-movflags", "+faststart",
            "-y",
            output_path,
        ]
    else:
        cmd = [
            "ffmpeg",
            "-ss", str(start_time),
            "-i", video_path,
            "-t", str(duration),
            "-c:v", "libx264",
            "-crf", "23",
            "-preset", "fast",
            "-c:a", "aac",
            "-b:a", "128k",
            "-movflags", "+faststart",
            "-y",
            output_path,
        ]

    result = subprocess.run(cmd, capture_output=True, text=True, timeout=300)
    if result.returncode != 0:
        raise RuntimeError(f"FFmpeg clip extraction failed: {result.stderr}")

    logger.info(f"Clip extracted: {start_time:.1f}s-{end_time:.1f}s -> {output_path}")
    return output_path


def generate_thumbnail(video_path: str, output_path: str, timestamp: Optional[float] = None) -> str:
    """Generate a thumbnail from a video at the specified timestamp."""
    Path(output_path).parent.mkdir(parents=True, exist_ok=True)

    if timestamp is None:
        # Get video info to seek to middle
        info = get_video_info(video_path)
        timestamp = info["duration"] / 2

    cmd = [
        "ffmpeg",
        "-ss", str(timestamp),
        "-i", video_path,
        "-vframes", "1",
        "-vf", "scale=1280:-1",
        "-q:v", "2",
        "-y",
        output_path,
    ]

    result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
    if result.returncode != 0:
        raise RuntimeError(f"FFmpeg thumbnail generation failed: {result.stderr}")

    logger.info(f"Thumbnail generated at {output_path}")
    return output_path


def add_subtitles_to_video(video_path: str, subtitle_path: str, output_path: str) -> str:
    """Burn subtitles into a video clip."""
    Path(output_path).parent.mkdir(parents=True, exist_ok=True)

    cmd = [
        "ffmpeg",
        "-i", video_path,
        "-vf", f"subtitles={subtitle_path}:force_style='Fontsize=24,Bold=1,PrimaryColour=&H00FFFFFF,OutlineColour=&H00000000,BackColour=&H80000000,BorderStyle=3,Outline=2,Shadow=1,MarginV=30'",
        "-c:v", "libx264",
        "-crf", "23",
        "-preset", "fast",
        "-c:a", "copy",
        "-movflags", "+faststart",
        "-y",
        output_path,
    ]

    result = subprocess.run(cmd, capture_output=True, text=True, timeout=300)
    if result.returncode != 0:
        raise RuntimeError(f"FFmpeg subtitle burning failed: {result.stderr}")

    logger.info(f"Subtitles added to {output_path}")
    return output_path


def get_file_size(path: str) -> int:
    """Return file size in bytes."""
    return os.path.getsize(path)
