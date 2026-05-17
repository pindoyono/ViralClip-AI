import json
from typing import Optional
from loguru import logger
from openai import AsyncOpenAI
from app.config import settings

_client: Optional[AsyncOpenAI] = None

CATEGORY_SYSTEM_PROMPT = """You are a content classification expert for social media video content.
Analyze the transcript and return a JSON object with:
- primary_category: main category (e.g., Education, Entertainment, Comedy, Finance, Fitness, Tech, etc.)
- sub_categories: array of 2-3 sub-categories
- niche: specific niche (e.g., "Personal Finance for Millennials")
- audience_type: target audience (e.g., "Young Professionals 25-35")
- content_type: type of content (tutorial, vlog, commentary, storytelling, motivational, etc.)"""


async def categorize_content(transcript: str, title: Optional[str] = None) -> dict:
    """Categorize video content using GPT-4."""
    client = _get_client()

    text = f"Title: {title}\n\n" if title else ""
    text += f"Transcript: {transcript[:2000]}"

    response = await client.chat.completions.create(
        model=settings.openai_model,
        messages=[
            {"role": "system", "content": CATEGORY_SYSTEM_PROMPT},
            {"role": "user", "content": text},
        ],
        max_tokens=400,
        temperature=0.3,
        response_format={"type": "json_object"},
    )

    data = json.loads(response.choices[0].message.content)
    logger.info(f"Categorized content as: {data.get('primary_category')}")
    return data


def _get_client() -> AsyncOpenAI:
    global _client
    if _client is None:
        _client = AsyncOpenAI(api_key=settings.openai_api_key)
    return _client
