import json
import time
from typing import List, Optional
from loguru import logger
from openai import AsyncOpenAI
from app.config import settings

_client: Optional[AsyncOpenAI] = None

PLATFORM_HASHTAG_LIMITS = {
    "tiktok": 10,
    "instagram": 30,
    "youtube": 15,
    "twitter": 3,
}

METADATA_SYSTEM_PROMPT = """You are a social media optimization expert specializing in viral content.
Generate platform-optimized metadata for short-form video clips.

Return a JSON object with:
- title: compelling title under 60 chars
- description: engaging description with keywords, 150-300 chars
- hashtags: array of relevant hashtags (without # prefix)
- keywords: array of SEO keywords
- category: primary content category
- optimal_post_times: array of best posting times (e.g. "7:00 PM EST on Weekdays")"""


async def generate_metadata(
    transcript: str,
    platform: str,
    niche: Optional[str] = None,
    tone: Optional[str] = None,
) -> dict:
    """Generate platform-optimized metadata for a clip."""
    client = _get_client()
    max_tags = PLATFORM_HASHTAG_LIMITS.get(platform.lower(), 10)

    context = f"Platform: {platform.upper()}"
    if niche:
        context += f", Niche: {niche}"
    if tone:
        context += f", Tone: {tone}"

    user_prompt = f"""{context}
Max hashtags: {max_tags}

Transcript excerpt:
{transcript[:2000]}

Generate optimized metadata. Return valid JSON."""

    start = time.time()
    response = await client.chat.completions.create(
        model=settings.openai_model,
        messages=[
            {"role": "system", "content": METADATA_SYSTEM_PROMPT},
            {"role": "user", "content": user_prompt},
        ],
        max_tokens=800,
        temperature=0.7,
        response_format={"type": "json_object"},
    )

    elapsed = round(time.time() - start, 2)
    logger.info(f"Metadata generated for {platform} in {elapsed}s")

    data = json.loads(response.choices[0].message.content)
    hashtags = data.get("hashtags", [])[:max_tags]

    return {
        "title": data.get("title", ""),
        "description": data.get("description", ""),
        "hashtags": hashtags,
        "keywords": data.get("keywords", []),
        "category": data.get("category", "Entertainment"),
        "optimal_post_times": data.get("optimal_post_times", []),
    }


def _get_client() -> AsyncOpenAI:
    global _client
    if _client is None:
        _client = AsyncOpenAI(api_key=settings.openai_api_key)
    return _client
