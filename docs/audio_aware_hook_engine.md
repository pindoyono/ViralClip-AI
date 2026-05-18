# Audio-Aware Hook Engine (V3)

## Overview

The Audio-Aware Hook Engine extends the text-based Hook Engine V2 with four
new audio-signal detectors.  Hook scores are now enriched by real acoustic
features extracted directly from the source audio/video file, making
detections more accurate and surfacing emotional moments that pure
transcript analysis may miss.

---

## Architecture

```
AudioAwareHookEngine
├── HookEngineV2                   ← existing text-pattern engine (unchanged)
│   ├── HookPatternDetector
│   └── HookScoreCalculator
└── AudioSignalAnalyzer            ← new orchestrator
    ├── PauseDetector              ← detects silence gaps
    ├── VoiceIntensityAnalyzer     ← measures RMS energy envelope
    └── SpeechPatternAnalyzer      ← measures words-per-second
```

Each component is **stateless** and fully **injectable** for testing.  The
`AudioAwareHookEngine` may be used without an audio analyzer (text-only
fallback) by passing `audio_analyzer=None`.

---

## Directory Changes

```
apps/ai-service/
  app/
    models/
      schemas.py                        # + AudioAwareHookDetectionRequest
                                        # + AudioAwareHookDetectionResponse
                                        # + SegmentAudioAnalysis
                                        # + PauseSignal / IntensitySignal
                                        # + SpeechPatternSignal
                                        # + PauseType / IntensityLevel
                                        # + SpeechRate / AudioEmotionLabel
    routers/
      hooks_v3.py                       # NEW  POST /api/v1/hooks/v3/detect
    services/
      pause_detector.py                 # NEW  PauseDetector
      voice_intensity_analyzer.py       # NEW  VoiceIntensityAnalyzer
      speech_pattern_analyzer.py        # NEW  SpeechPatternAnalyzer
      audio_signal_analyzer.py          # NEW  AudioSignalAnalyzer
      audio_aware_hook_engine.py        # NEW  AudioAwareHookEngine
  tests/
    test_audio_aware_hook_engine.py     # NEW  71 unit + integration tests
  main.py                               # + hooks_v3 router registration
```

---

## New Components

### PauseDetector

Detects silence regions in a PCM audio array by computing RMS energy per
20 ms frame and comparing against a configurable threshold.

**Pause types:**

| Type       | Duration  | Signal                              |
|------------|-----------|-------------------------------------|
| `dramatic` | ≥ 2.0 s   | Narrative beat, tension             |
| `long`     | ≥ 1.0 s   | Topic boundary, breath              |
| `emphasis` | ≥ 0.3 s   | Word stress, micro-pause            |

```python
detector = PauseDetector(silence_threshold=0.01, min_pause_duration=0.3)
pauses = detector.detect(audio, sample_rate=16000)
pre_pause = detector.detect_pre_segment_pause(audio, SR, prev_end=5.0, segment_start=7.5)
```

---

### VoiceIntensityAnalyzer

Measures the energy envelope of each transcript window relative to a track
baseline (median frame RMS).

**Intensity levels:**

| Level    | Condition                        | Signal                    |
|----------|----------------------------------|---------------------------|
| `loud`   | RMS ≥ 1.8× baseline              | Louder voice, emphasis    |
| `quiet`  | RMS ≤ 0.5× baseline              | Whisper, low energy       |
| `normal` | Otherwise                        | Average delivery          |

**Additional flags:**

- `has_sudden_increase` – any frame jumps ≥ 1.5× the preceding frame's RMS
- `is_emotional` – segment is both `loud` and energetically variable (std > 30 % of mean)

```python
analyzer = VoiceIntensityAnalyzer()
baseline = analyzer.compute_baseline(audio, SR)
info = analyzer.analyze(audio, SR, seg_start=10.0, seg_end=15.0, baseline_rms=baseline)
```

---

### SpeechPatternAnalyzer

Computes the words-per-second (wps) rate for each transcript segment and
classifies it relative to the transcript-wide distribution.

**Speech rates:**

| Rate           | Condition                                    | Signal                        |
|----------------|----------------------------------------------|-------------------------------|
| `fast`         | wps > μ + 1σ                                | Energetic, excited delivery   |
| `slow`         | wps < μ − 1σ                                | Deliberate, dramatic delivery |
| `sudden_change`| \|Δwps\| ≥ 2.0 (from preceding segment)    | Pacing shift, tension release |
| `normal`       | Otherwise                                    | Average pacing                |

Operates on **transcript segments only** – no audio file required.

```python
analyzer = SpeechPatternAnalyzer()
results = analyzer.analyze([(start, end, text), ...])
```

---

### AudioSignalAnalyzer

Orchestrates the three detectors above.  Loads the audio file via **ffmpeg**
(already a project dependency) into a 16 kHz mono PCM array, then produces
one `SegmentAudioSignals` per transcript segment.

**Audio emotion heuristic (rule-based):**

| Emotion      | Trigger                        |
|--------------|-------------------------------|
| `excitement` | loud + fast speech             |
| `anger`      | loud + sudden volume increase  |
| `surprise`   | volume spike, not slow         |
| `sadness`    | quiet + slow speech            |
| `neutral`    | none of the above              |

**Audio score (0–100):**

| Signal              | Points |
|---------------------|--------|
| Dramatic pause      | +25    |
| Long pause          | +15    |
| Emphasis pause      | +8     |
| Loud voice          | +20    |
| Sudden increase     | +15    |
| Emotional delivery  | +10    |
| Sudden speed change | +20    |
| Fast speech         | +10    |
| Slow speech         | +8     |

Gracefully degrades to an empty result when ffmpeg is unavailable or the
file cannot be read.

```python
analyzer = build_audio_signal_analyzer()
signals = analyzer.analyze(segments, audio_path="/data/video.mp4")
```

---

### AudioAwareHookEngine

Combines `HookEngineV2` (text) with `AudioSignalAnalyzer` (audio):

1. Run text-based detection (scores 0–100).
2. Run audio analysis (audio_score 0–100).
3. **Boost** text hooks: `combined = min(100, text_score + audio_score × 0.25)`.
4. **Surface audio-only hooks** when `audio_score ≥ 40` and no text pattern fired,
   with score `= int(audio_score × 0.85)`.
5. Filter by `min_score`, sort descending.

```python
engine = build_audio_aware_hook_engine()
hooks = await engine.detect_hooks(
    segments=transcript_segments,
    audio_path="/data/video.mp4",
    min_score=50,
)
```

---

## API

### `POST /api/v1/hooks/v3/detect`

**Request:**

```json
{
  "video_id": "abc123",
  "segments": [
    { "text": "At first I thought nothing would happen", "start": 10.0, "end": 15.0 }
  ],
  "audio_storage_path": "/data/storage/abc123/audio.mp4",
  "min_score": 50
}
```

`audio_storage_path` is **optional**.  When omitted the engine falls back to
text-only analysis identical to V2.

**Response:**

```json
{
  "video_id": "abc123",
  "hooks": [
    { "start": 10.0, "end": 15.0, "type": "storytelling", "score": 95, "matched_pattern": "at first" }
  ],
  "total": 1,
  "audio_enabled": true,
  "audio_analysis": [
    {
      "start": 10.0,
      "end": 15.0,
      "pre_pause": { "start": 8.5, "end": 10.0, "duration": 1.5, "pause_type": "long" },
      "intensity": {
        "rms_db": -18.4,
        "rms_relative": 1.92,
        "intensity_level": "loud",
        "has_sudden_increase": false,
        "is_emotional": true
      },
      "speech_pattern": {
        "words_per_second": 1.6,
        "speech_rate": "slow",
        "rate_deviation": -1.2
      },
      "audio_emotion": "sadness",
      "audio_score": 48,
      "audio_hook_type": "storytelling"
    }
  ]
}
```

**Hook types** remain the same five categories as V2:
`curiosity`, `emotion`, `storytelling`, `controversy`, `cta`.

Audio-only detections use `matched_pattern: "audio:<emotion>"` (e.g.
`"audio:excitement"`).

---

## Backward Compatibility

- `POST /api/v1/hooks/v2/detect` is **unchanged**.
- The V3 engine falls back to text-only when `audio_storage_path` is omitted
  or ffmpeg fails, producing identical results to V2.
- All existing schemas remain unchanged; only additive schema changes were
  made to `schemas.py`.

---

## Testing

```bash
cd apps/ai-service
python -m pytest tests/test_audio_aware_hook_engine.py -v
```

71 tests cover:

| Layer                      | Tests |
|----------------------------|-------|
| `PauseDetector`            | 11    |
| `VoiceIntensityAnalyzer`   | 10    |
| `SpeechPatternAnalyzer`    | 7     |
| `AudioSignalAnalyzer`      | 20    |
| `AudioAwareHookEngine`     | 11    |
| HTTP endpoint `/v3/detect` | 12    |

---

## Dependencies

No new Python packages required.  The implementation uses:

- `numpy` (already in `requirements.txt`) for audio frame analysis
- `ffmpeg` subprocess (already a project dependency) for audio decoding
