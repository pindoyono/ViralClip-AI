import OpenAI from "openai";

export interface MetadataEngineConfig {
  openaiApiKey: string;
  model?: string;
}

export interface GeneratedMetadata {
  title: string;
  description: string;
  hashtags: string[];
  keywords: string[];
  category: string;
  optimal_post_times: string[];
}

export async function generateMetadata(
  config: MetadataEngineConfig,
  transcript: string,
  platform: string,
  options: { niche?: string; tone?: string } = {}
): Promise<GeneratedMetadata> {
  const client = new OpenAI({ apiKey: config.openaiApiKey });

  const response = await client.chat.completions.create({
    model: config.model ?? "gpt-4-turbo-preview",
    messages: [
      {
        role: "system",
        content:
          "You are a social media SEO expert. Generate platform-optimized metadata for video clips. Return JSON with: title, description, hashtags (array), keywords (array), category, optimal_post_times (array).",
      },
      {
        role: "user",
        content: `Platform: ${platform}\nOptions: ${JSON.stringify(options)}\n\nTranscript: ${transcript.slice(0, 2000)}\n\nReturn valid JSON.`,
      },
    ],
    response_format: { type: "json_object" },
  });

  return JSON.parse(response.choices[0].message.content ?? "{}") as GeneratedMetadata;
}
