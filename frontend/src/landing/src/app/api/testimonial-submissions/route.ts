import { neon } from "@neondatabase/serverless";
import { type NextRequest } from "next/server";
import { z } from "zod";
import {
  countWords,
  isLinkedInProfileUrl,
  isTweetUrl,
  MAX_TESTIMONIAL_WORDS,
} from "@/lib/testimonial-submission";
import { getDatabaseUrl, handleJsonPost } from "@/lib/json-post-handler";

const MAX_BODY_BYTES = 32_768;
const RATE_LIMIT_MAX_REQUESTS = 20;

const testimonialSchema = z.object({
  testimonial: z
    .string()
    .trim()
    .min(20)
    .max(10_000)
    .refine((value) => countWords(value) <= MAX_TESTIMONIAL_WORDS),
  linkedinUrl: z.string().trim().max(500).refine(isLinkedInProfileUrl),
  tweetUrl: z
    .string()
    .trim()
    .max(500)
    .refine((value) => !value || isTweetUrl(value))
    .optional()
    .default(""),
});

let ensureTablePromise: Promise<void> | undefined;

async function ensureTable(databaseUrl: string) {
  if (!ensureTablePromise) {
    const sql = neon(databaseUrl);

    ensureTablePromise = sql`
      CREATE TABLE IF NOT EXISTS open_agents_testimonial_submissions (
        id BIGSERIAL PRIMARY KEY,
        testimonial TEXT NOT NULL,
        linkedin_url TEXT NOT NULL UNIQUE,
        tweet_url TEXT,
        status TEXT NOT NULL DEFAULT 'pending',
        source TEXT NOT NULL DEFAULT 'open_agents_testimonial_submission',
        created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
      )
    `.then(() => undefined);
  }

  return ensureTablePromise;
}

export async function POST(request: NextRequest) {
  return handleJsonPost(request, {
    maxBodyBytes: MAX_BODY_BYTES,
    rateLimitMaxRequests: RATE_LIMIT_MAX_REQUESTS,
    parse: (body) => {
      const parsed = testimonialSchema.safeParse(body);

      return parsed.success ? parsed.data : null;
    },
    invalidError: "Please check your testimonial and profile links.",
    save: async (data) => {
      const databaseUrl = getDatabaseUrl();
      const sql = neon(databaseUrl);

      await ensureTable(databaseUrl);

      await sql`
        INSERT INTO open_agents_testimonial_submissions (testimonial, linkedin_url, tweet_url)
        VALUES (
          ${data.testimonial},
          ${data.linkedinUrl},
          ${data.tweetUrl || null}
        )
        ON CONFLICT (linkedin_url)
        DO UPDATE SET
          testimonial = EXCLUDED.testimonial,
          tweet_url = EXCLUDED.tweet_url,
          status = 'pending',
          updated_at = now()
      `;
    },
    failureLog: "Open Agents testimonial storage failed.",
    failureError: "Unable to save testimonial.",
  });
}