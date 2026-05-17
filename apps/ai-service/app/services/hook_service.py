import json
import time
from typing import List, Dict, Any, Optional
from loguru import logger
from openai import AsyncOpenAI
from app.config import settings

_client: Optional[AsyncOpenAI] = None


def _get_client() -> AsyncOpenAI:
    global _client
    if _client is None:
        _client = AsyncOpenAI(api_key=settings.openai_api_key)
    return _client


HOOK_SYSTEM_PROMPT = """You are a viral content strategist specializing in short-form social media clips.
Analyze the provided video transcript and generate compelling hooks that will maximize viewer retention and engagement.

Hooks should be:
- Attention-grabbing within the first 3 seconds
- Platform-appropriate (TikTok/Reels/Shorts style)
- Emotionally resonant or curiosity-driven
- Under 15 words for maximum impact

Return a JSON array of hooks with: text, type (question/statement/statistic/story/challenge), viral_score (0-1), rationale."""


async def generate_hooks(
    transcript: str,
    niche: Optional[str] = None,
    platform: Optional[str] = None,
    tone: Optional[str] = None,
    count: int = 5,
) -> List[Dict[str, Any]]:
    """Generate viral hooks from a transcript using GPT-4."""
    client = _get_client()

    context_parts = []
    if niche:
        context_parts.append(f"Niche: {niche}")
    if platform:
        context_parts.append(f"Platform: {platform}")
    if tone:
        context_parts.append(f"Tone: {tone}")
    context = ". ".join(context_parts) if context_parts else ""

    user_prompt = f"""Transcript:
{transcript[:3000]}

{context}

Generate {count} viral hooks for this content. Return only valid JSON array."""

    logger.info(f"Generating {count} hooks for transcript ({len(transcript)} chars)")
    start = time.time()

    response = await client.chat.completions.create(
        model=settings.openai_model,
        messages=[
            {"role": "system", "content": HOOK_SYSTEM_PROMPT},
            {"role": "user", "content": user_prompt},
        ],
        max_tokens=1500,
        temperature=settings.openai_temperature,
        response_format={"type": "json_object"},
    )

    elapsed = round(time.time() - start, 2)
    logger.info(f"Hooks generated in {elapsed}s")

    raw = response.choices[0].message.content
    data = json.loads(raw)

    hooks = data if isinstance(data, list) else data.get("hooks", [])
    return hooks[:count]
