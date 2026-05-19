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


# ---------------------------------------------------------------------------
# Pipeline /process/* schemas (used by worker queue consumers)
# ---------------------------------------------------------------------------

class ProcessTranscriptRequest(BaseModel):
    video_id: str
    storage_path: str
    language: Optional[str] = None


class ProcessTranscriptResponse(BaseModel):
    video_id: str
    language: str
    duration: float
    segments: List[TranscriptSegment]
    full_text: str
    transcript_path: str  # path to cached JSON file


class ExtractedClip(BaseModel):
    """A single clip extracted from the source video."""
    index: int
    storage_path: str
    start_time: float
    end_time: float
    duration: float
    viral_score: float
    rationale: str
    hook_text: str
    suggested_title: str
    hashtags: List[str]
    suggested_for: List[str]


class ProcessClipsRequest(BaseModel):
    video_id: str
    storage_path: str
    content_profile: Optional[Dict[str, Any]] = None
    max_clips: int = Field(default=10, ge=1, le=30)
    min_duration: float = Field(default=15.0, ge=5.0)
    max_duration: float = Field(default=90.0, le=300.0)


class ProcessClipsResponse(BaseModel):
    video_id: str
    clips: List[ExtractedClip]
    manifest_path: str
    processing_time: float


class ProcessSubtitlesRequest(BaseModel):
    video_id: str
    storage_path: str
    style: SubtitleStyle = SubtitleStyle.DEFAULT
    font_size: int = Field(default=24, ge=12, le=72)
    primary_color: str = Field(default="&H00FFFFFF")
    outline_color: str = Field(default="&H00000000")


class ProcessSubtitlesResponse(BaseModel):
    video_id: str
    clips_processed: int


class ProcessVideoRequest(BaseModel):
    video_id: str
    storage_path: str


class ProcessVideoResponse(BaseModel):
    video_id: str
    duration: float
    width: int
    height: int
    fps: float
    has_audio: bool
    thumbnail_path: str


# ---------------------------------------------------------------------------
# Hook Engine V2 schemas
# ---------------------------------------------------------------------------

class HookTypeV2(str, Enum):
    CURIOSITY = "curiosity"
    EMOTION = "emotion"
    STORYTELLING = "storytelling"
    CONTROVERSY = "controversy"
    CTA = "cta"


class TranscriptSegmentInput(BaseModel):
    """A single transcript segment with timing information."""
    text: str
    start: float = Field(..., ge=0.0, description="Segment start time in seconds")
    end: float = Field(..., gt=0.0, description="Segment end time in seconds")


class HookDetectionResult(BaseModel):
    """A detected hook moment within a transcript."""
    start: float = Field(..., description="Segment start time in seconds")
    end: float = Field(..., description="Segment end time in seconds")
    type: str = Field(..., description="Hook category (curiosity/emotion/storytelling/controversy/cta)")
    score: int = Field(..., ge=0, le=100, description="Hook strength score 0–100")
    matched_pattern: str = Field(default="", description="The specific text fragment that triggered detection")


class HookDetectionRequest(BaseModel):
    """Request body for the V2 hook detection endpoint."""
    video_id: str
    segments: List[TranscriptSegmentInput] = Field(
        ..., min_length=1, description="Transcript segments with timestamps"
    )
    min_score: int = Field(default=50, ge=0, le=100, description="Minimum score to include in results")


class HookDetectionResponse(BaseModel):
    """Response from the V2 hook detection endpoint."""
    video_id: str
    hooks: List[HookDetectionResult]
    total: int = Field(..., description="Total number of hooks detected")


# ---------------------------------------------------------------------------
# Clip Engine V2 schemas
# ---------------------------------------------------------------------------

class ProfileType(str, Enum):
    GAMING    = "gaming"
    COMEDY    = "comedy"
    EDUCATION = "education"
    POLITICS  = "politics"
    PODCAST   = "podcast"
    GENERAL   = "general"


class ClipV2ResultSchema(BaseModel):
    """A single clip candidate from the V2 engine."""
    start: str = Field(..., description="Clip start time as HH:MM:SS")
    end: str   = Field(..., description="Clip end time as HH:MM:SS")
    start_seconds: float
    end_seconds: float
    score: int = Field(..., ge=0, le=100, description="Composite clip score 0–100")
    hook_score:      float = Field(..., description="Best hook score in window (0–100)")
    emotion_score:   float = Field(..., description="Average emotion score in window (0–100)")
    story_score:     float = Field(..., description="Average story arc score in window (0–100)")
    retention_score: float = Field(..., description="Predicted retention score (0–100)")
    profile_type:    str   = Field(..., description="Content profile used")


class HistoricalAnalytics(BaseModel):
    """Historical retention aggregates used by the learning-aware predictor."""
    sample_size: int = Field(default=0, ge=0, description="Number of historical clips used")
    baseline_retention: float = Field(default=0.0, ge=0.0, le=1.0, description="Historical retention baseline (0-1)")
    duration_bucket_retention: Dict[str, float] = Field(
        default_factory=dict,
        description="Historical retention by duration bucket: short|medium|long (0-1)",
    )
    category_retention: Dict[str, float] = Field(
        default_factory=dict,
        description="Historical retention by category/profile label (0-1)",
    )
    hook_type_retention: Dict[str, float] = Field(
        default_factory=dict,
        description="Historical retention by hook type (0-1)",
    )


class ClipGenerateV2Request(BaseModel):
    """Request body for the V2 clip generation endpoint."""
    video_id: str
    segments: List[TranscriptSegmentInput] = Field(
        ..., min_length=1, description="Ordered transcript segments"
    )
    hook_detections: List[HookDetectionResult] = Field(
        default_factory=list,
        description="V2 hook detections (from /hooks/v2/detect); can be empty",
    )
    profile_type: ProfileType = Field(
        default=ProfileType.GENERAL,
        description="Content profile controlling clip duration range",
    )
    min_clip_score: int = Field(default=50, ge=0, le=100)
    max_clips: int      = Field(default=10, ge=1, le=30)
    historical_analytics: Optional[HistoricalAnalytics] = Field(
        default=None,
        description="Optional historical analytics aggregates to improve retention prediction",
    )


class ClipGenerateV2Response(BaseModel):
    """Response from the V2 clip generation endpoint."""
    video_id: str
    profile_type: str
    clips: List[ClipV2ResultSchema]
    total: int


# ---------------------------------------------------------------------------
# Audio-Aware Hook Engine V3 schemas
# ---------------------------------------------------------------------------

class PauseType(str, Enum):
    DRAMATIC = "dramatic"
    LONG = "long"
    EMPHASIS = "emphasis"


class IntensityLevel(str, Enum):
    LOUD = "loud"
    NORMAL = "normal"
    QUIET = "quiet"


class SpeechRate(str, Enum):
    FAST = "fast"
    SLOW = "slow"
    NORMAL = "normal"
    SUDDEN_CHANGE = "sudden_change"


class AudioEmotionLabel(str, Enum):
    EXCITEMENT = "excitement"
    ANGER = "anger"
    SURPRISE = "surprise"
    SADNESS = "sadness"
    NEUTRAL = "neutral"


class PauseSignal(BaseModel):
    """Detected speech pause before a transcript segment."""
    start: float = Field(..., description="Pause start time in seconds")
    end: float = Field(..., description="Pause end time in seconds")
    duration: float = Field(..., description="Pause length in seconds")
    pause_type: PauseType = Field(..., description="Pause classification")


class IntensitySignal(BaseModel):
    """Voice intensity characteristics for a transcript segment."""
    rms_db: float = Field(..., description="Mean RMS of the segment in dBFS")
    rms_relative: float = Field(..., description="Segment RMS / track baseline RMS")
    intensity_level: IntensityLevel = Field(..., description="Intensity classification")
    has_sudden_increase: bool = Field(..., description="True if energy spikes within segment")
    is_emotional: bool = Field(..., description="True if loud and energetically variable")


class SpeechPatternSignal(BaseModel):
    """Speech rate characteristics for a transcript segment."""
    words_per_second: float = Field(..., description="Word rate for this segment")
    speech_rate: SpeechRate = Field(..., description="Speech rate classification")
    rate_deviation: float = Field(..., description="Deviation from mean in standard deviations")


class SegmentAudioAnalysis(BaseModel):
    """Complete audio analysis result for a single transcript segment."""
    start: float
    end: float
    pre_pause: Optional[PauseSignal] = Field(default=None, description="Pause before this segment")
    intensity: IntensitySignal
    speech_pattern: SpeechPatternSignal
    audio_emotion: AudioEmotionLabel
    audio_score: int = Field(..., ge=0, le=100, description="Combined audio signal score 0–100")
    audio_hook_type: Optional[str] = Field(
        default=None,
        description="Hook type suggested by audio signals",
    )


class AudioAwareHookDetectionRequest(BaseModel):
    """Request body for the V3 audio-aware hook detection endpoint."""
    video_id: str
    segments: List[TranscriptSegmentInput] = Field(
        ..., min_length=1, description="Transcript segments with timestamps"
    )
    audio_storage_path: Optional[str] = Field(
        default=None,
        description=(
            "Absolute path to the audio/video file for audio signal analysis. "
            "When omitted, falls back to text-only detection (same as V2)."
        ),
    )
    min_score: int = Field(default=50, ge=0, le=100, description="Minimum score to include")


class AudioAwareHookDetectionResponse(BaseModel):
    """Response from the V3 audio-aware hook detection endpoint."""
    video_id: str
    hooks: List[HookDetectionResult]
    total: int = Field(..., description="Total number of hooks detected")
    audio_analysis: List[SegmentAudioAnalysis] = Field(
        default_factory=list,
        description="Per-segment audio analysis (empty when no audio path provided)",
    )
    audio_enabled: bool = Field(
        default=False,
        description="True when audio signals were incorporated into scores",
    )
