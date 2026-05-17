import type { ViralHook } from "@viralclip/shared-types";
import OpenAI from "openai";

export interface HookEngineConfig {
  openaiApiKey: string;
  model?: string;
  temperature?: number;
}

export async function generateHooks(
  config: HookEngineConfig,
  transcript: string,
  options: { niche?: string; platform?: string; tone?: string; count?: number } = {}
): Promise<ViralHook[]> {
  const client = new OpenAI({ apiKey: config.openaiApiKey });
  const count = options.count ?? 5;

  const response = await client.chat.completions.create({
    model: config.model ?? "gpt-4-turbo-preview",
    messages: [
      {
        role: "system",
        content:
          "You are a viral content strategist. Generate compelling hooks for short-form video clips. Return JSON array of hooks with: text, type, viral_score (0-1), rationale.",
      },
      {
        role: "user",
        content: `Transcript: ${transcript.slice(0, 3000)}\n\nOptions: ${JSON.stringify(options)}\n\nGenerate ${count} hooks. Return valid JSON array.`,
      },
    ],
    temperature: config.temperature ?? 0.7,
    response_format: { type: "json_object" },
  });

  const raw = JSON.parse(response.choices[0].message.content ?? "{}");
  const hooks: ViralHook[] = Array.isArray(raw) ? raw : raw.hooks ?? [];
  return hooks.slice(0, count);
}
