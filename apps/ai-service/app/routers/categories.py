from fastapi import APIRouter, HTTPException
from loguru import logger
from app.models.schemas import CategoryRequest, CategoryResponse
from app.services.category_service import categorize_content

router = APIRouter(prefix="/categories", tags=["categories"])


@router.post("", response_model=CategoryResponse)
async def categorize(request: CategoryRequest):
    """Categorize video content using AI."""
    if not request.transcript.strip():
        raise HTTPException(status_code=400, detail="Transcript is required")

    try:
        result = await categorize_content(
            transcript=request.transcript,
            title=request.title,
        )
        return CategoryResponse(**result)
    except Exception as e:
        logger.error(f"Categorization failed: {e}")
        raise HTTPException(status_code=500, detail=str(e))
