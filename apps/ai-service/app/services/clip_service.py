import json
import time
from typing import List, Dict, Any, Optional
from loguru import logger
from openai import AsyncOpenAI
from app.config import settings
from app.models.schemas import TranscriptSegment, ClipSegment

_client: Optional[AsyncOpenAI] = None


def _get_client() -> AsyncOpenAI:
    global _client
    if _client is None:
        _client = AsyncOpenAI(api_key=settings.openai_api_key)
    return _client


CLIP_SYSTEM_PROMPT = """You are a viral content analyst for short-form video platforms (TikTok, Instagram Reels, YouTube Shorts).

Analyze transcript segments and identify the most viral-worthy clips. Consider:
- Emotional peaks and storytelling arcs
- Surprising facts or revelations
- Relatable moments
- Strong hooks and clean endings
- Optimal duration (15-90 seconds)
- Multi-platform potential

Return a JSON object with a "clips" array. Each clip must have:
- start_time (float, seconds)
- end_time (float, seconds)
- viral_score (float 0.0-1.0)
- rationale (string, why this clip is viral-worthy)
- hook_text (string, the opening line/hook)
- suggested_title (string)
- hashtags (array of strings, 5-10 tags)
- suggested_for (array of platforms: ["tiktok", "reels", "shorts"])"""


async def identify_viral_segments(
    transcript_segments: List[TranscriptSegment],
    content_profile: Optional[Dict[str, Any]] = None,
    max_clips: int = 10,
    min_duration: float = 15.0,
    max_duration: float = 90.0,
) -> List[ClipSegment]:
    """Use GPT-4 to identify the most viral segments from a transcript."""
    client = _get_client()

    segments_text = "\n".join(
        [f"[{s.start:.1f}s - {s.end:.1f}s]: {s.text}" for s in transcript_segments]
    )

    profile_context = ""
    if content_profile:
        niche = content_profile.get("niche", "")
        tone = content_profile.get("tone", "")
        platforms = content_profile.get("platforms", [])
        if niche or tone or platforms:
            profile_context = f"\nContent Profile - Niche: {niche}, Tone: {tone}, Target Platforms: {', '.join(platforms)}"

    user_prompt = f"""Analyze these transcript segments and identify up to {max_clips} viral clips.
Duration constraints: {min_duration}s minimum, {max_duration}s maximum.{profile_context}

Transcript segments:
{segments_text[:6000]}

Return valid JSON with a "clips" array."""

    logger.info(f"Identifying viral segments from {len(transcript_segments)} transcript segments")
    start = time.time()

    response = await client.chat.completions.create(
        model=settings.openai_model,
        messages=[
            {"role": "system", "content": CLIP_SYSTEM_PROMPT},
            {"role": "user", "content": user_prompt},
        ],
        max_tokens=settings.openai_max_tokens,
        temperature=0.5,
        response_format={"type": "json_object"},
    )

    elapsed = round(time.time() - start, 2)
    logger.info(f"Segment identification completed in {elapsed}s")

    raw = response.choices[0].message.content
    data = json.loads(raw)
    raw_clips = data.get("clips", [])

    clips = []
    for c in raw_clips[:max_clips]:
        try:
            start_time = float(c["start_time"])
            end_time = float(c["end_time"])
            duration = end_time - start_time
            if not (min_duration <= duration <= max_duration):
                continue
            clips.append(
                ClipSegment(
                    start_time=round(start_time, 3),
                    end_time=round(end_time, 3),
                    duration=round(duration, 3),
                    viral_score=min(1.0, max(0.0, float(c.get("viral_score", 0.5)))),
                    rationale=c.get("rationale", ""),
                    hook_text=c.get("hook_text", ""),
                    suggested_title=c.get("suggested_title", ""),
                    hashtags=c.get("hashtags", []),
                    suggested_for=c.get("suggested_for", ["tiktok", "reels", "shorts"]),
                )
            )
        except (KeyError, ValueError, TypeError) as e:
            logger.warning(f"Skipping malformed clip segment: {e}")
            continue

    clips.sort(key=lambda x: x.viral_score, reverse=True)
    return clips
