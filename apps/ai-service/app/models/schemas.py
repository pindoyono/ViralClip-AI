from pydantic import BaseModel, Field
from typing import Optional, List, Dict, Any
from enum import Enum


class TranscriptSegment(BaseModel):
    start: float
    end: float
    text: str
    confidence: float = 1.0


class TranscriptRequest(BaseModel):
    video_id: str
    storage_path: str
    language: Optional[str] = None


class TranscriptResponse(BaseModel):
    video_id: str
    language: str
    duration: float
    segments: List[TranscriptSegment]
    full_text: str


class HookType(str, Enum):
    QUESTION = "question"
    STATEMENT = "statement"
    STATISTIC = "statistic"
    STORY = "story"
    CHALLENGE = "challenge"


class HookRequest(BaseModel):
    video_id: str
    transcript: str
    niche: Optional[str] = None
    platform: Optional[str] = None
    tone: Optional[str] = None
    count: int = Field(default=5, ge=1, le=20)


class HookResponse(BaseModel):
    video_id: str
    hooks: List[Dict[str, Any]]


class ClipSegment(BaseModel):
    start_time: float
    end_time: float
    duration: float
    viral_score: float
    rationale: str
    hook_text: str
    suggested_title: str
    hashtags: List[str]
    suggested_for: List[str]


class ClipGenerationRequest(BaseModel):
    video_id: str
    storage_path: str
    transcript: Optional[str] = None
    segments: Optional[List[TranscriptSegment]] = None
    content_profile: Optional[Dict[str, Any]] = None
    max_clips: int = Field(default=10, ge=1, le=30)
    min_duration: float = Field(default=15.0, ge=5.0)
    max_duration: float = Field(default=90.0, le=300.0)


class ClipGenerationResponse(BaseModel):
    video_id: str
    clips: List[ClipSegment]
    processing_time: float


class SubtitleStyle(str, Enum):
    DEFAULT = "default"
    BOLD = "bold"
    OUTLINE = "outline"
    SHADOW = "shadow"


class SubtitleRequest(BaseModel):
    video_id: str
    clip_storage_path: str
    transcript_segments: List[TranscriptSegment]
    style: SubtitleStyle = SubtitleStyle.DEFAULT
    font_size: int = Field(default=24, ge=12, le=72)
    primary_color: str = Field(default="&H00FFFFFF")
    outline_color: str = Field(default="&H00000000")


class SubtitleResponse(BaseModel):
    video_id: str
    output_path: str
    subtitle_path: str


class MetadataRequest(BaseModel):
    video_id: str
    transcript: str
    platform: str
    niche: Optional[str] = None
    tone: Optional[str] = None


class MetadataResponse(BaseModel):
    video_id: str
    title: str
    description: str
    hashtags: List[str]
    keywords: List[str]
    category: str
    optimal_post_times: List[str]


class CategoryRequest(BaseModel):
    transcript: str
    title: Optional[str] = None


class CategoryResponse(BaseModel):
    primary_category: str
    sub_categories: List[str]
    niche: str
    audience_type: str
    content_type: str


class VideoProcessRequest(BaseModel):
    video_id: str
    storage_path: str
    content_profile: Optional[Dict[str, Any]] = None


class ProcessingStatus(str, Enum):
    ACCEPTED = "accepted"
    PROCESSING = "processing"
    COMPLETED = "completed"
    FAILED = "failed"


class VideoProcessResponse(BaseModel):
    video_id: str
    status: ProcessingStatus
    message: str
