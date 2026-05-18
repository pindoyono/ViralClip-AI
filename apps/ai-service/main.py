import sys
import time
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse
from loguru import logger

from app.config import settings
from app.routers import transcript, hooks, clips, subtitles, metadata, categories, process
from app.routers import hooks_v2
from app.routers import hooks_v3
from app.routers import clips_v2

# Configure loguru
logger.remove()
logger.add(
    sys.stdout,
    format="<green>{time:YYYY-MM-DD HH:mm:ss}</green> | <level>{level: <8}</level> | <cyan>{name}</cyan>:<cyan>{line}</cyan> - <level>{message}</level>",
    level=settings.log_level,
    colorize=True,
)


@asynccontextmanager
async def lifespan(app: FastAPI):
    logger.info(f"ViralClip AI Service v{settings.app_version} starting ({settings.app_env})")
    logger.info(f"Whisper model: {settings.whisper_model} on {settings.whisper_device}")
    yield
    logger.info("AI Service shutting down")


app = FastAPI(
    title="ViralClip AI Service",
    description="AI-powered clip generation, transcription, and metadata service",
    version=settings.app_version,
    lifespan=lifespan,
    docs_url="/docs" if settings.app_env != "production" else None,
    redoc_url=None,
)

app.add_middleware(
    CORSMiddleware,
    allow_origins=["*"],
    allow_credentials=True,
    allow_methods=["*"],
    allow_headers=["*"],
)


@app.middleware("http")
async def log_requests(request: Request, call_next):
    start = time.time()
    response = await call_next(request)
    elapsed = round((time.time() - start) * 1000, 2)
    logger.info(f"{request.method} {request.url.path} → {response.status_code} ({elapsed}ms)")
    return response


@app.exception_handler(Exception)
async def global_exception_handler(request: Request, exc: Exception):
    logger.error(f"Unhandled exception: {exc}", exc_info=True)
    return JSONResponse(status_code=500, content={"detail": "Internal server error"})


# Health & readiness
@app.get("/health", tags=["health"])
async def health():
    return {"status": "ok", "version": settings.app_version}


@app.get("/ready", tags=["health"])
async def ready():
    return {"status": "ready"}


# Include routers
app.include_router(transcript.router, prefix="/api/v1")
app.include_router(hooks.router, prefix="/api/v1")
app.include_router(clips.router, prefix="/api/v1")
app.include_router(subtitles.router, prefix="/api/v1")
app.include_router(metadata.router, prefix="/api/v1")
app.include_router(categories.router, prefix="/api/v1")
app.include_router(process.router)  # /process/* — no extra prefix; router itself declares /process
app.include_router(hooks_v2.router, prefix="/api/v1")
app.include_router(hooks_v3.router, prefix="/api/v1")
app.include_router(clips_v2.router, prefix="/api/v1")
