from pydantic_settings import BaseSettings
from pydantic import Field
from typing import Optional


class Settings(BaseSettings):
    # Application
    app_env: str = Field(default="development", alias="APP_ENV")
    app_version: str = Field(default="0.1.0", alias="APP_VERSION")

    # Server
    ai_service_port: int = Field(default=8000, alias="AI_SERVICE_PORT")
    ai_service_host: str = Field(default="0.0.0.0", alias="AI_SERVICE_HOST")

    # Redis
    redis_url: str = Field(default="redis://localhost:6379/0", alias="REDIS_URL")

    # OpenAI
    openai_api_key: Optional[str] = Field(default=None, alias="OPENAI_API_KEY")
    openai_model: str = Field(default="gpt-4-turbo-preview", alias="OPENAI_MODEL")
    openai_max_tokens: int = Field(default=4096, alias="OPENAI_MAX_TOKENS")
    openai_temperature: float = Field(default=0.7, alias="OPENAI_TEMPERATURE")

    # Whisper
    whisper_model: str = Field(default="base", alias="WHISPER_MODEL")
    whisper_device: str = Field(default="cpu", alias="WHISPER_DEVICE")
    whisper_compute_type: str = Field(default="int8", alias="WHISPER_COMPUTE_TYPE")

    # FFmpeg
    ffmpeg_path: str = Field(default="/usr/bin/ffmpeg", alias="FFMPEG_PATH")
    ffprobe_path: str = Field(default="/usr/bin/ffprobe", alias="FFPROBE_PATH")
    ffmpeg_max_threads: int = Field(default=4, alias="FFMPEG_MAX_THREADS")

    # Storage
    local_storage_path: str = Field(default="./storage", alias="LOCAL_STORAGE_PATH")

    # Logging
    log_level: str = Field(default="INFO", alias="LOG_LEVEL")

    # Sentry
    sentry_dsn: Optional[str] = Field(default=None, alias="SENTRY_DSN")

    class Config:
        env_file = ".env"
        populate_by_name = True


settings = Settings()
