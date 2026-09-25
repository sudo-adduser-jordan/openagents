import { neon } from "@neondatabase/serverless";
import { handleJsonPost } from "./json-post-handler";

type Env = {
  DATABASE_URL: string;
};

const MAX_BODY_BYTES = 32_768;
const MAX_TESTIMONIAL_WORDS = 350;
const RATE_LIMIT_MAX_REQUESTS = 20;

let ensureTablePromise: Promise<void> | undefined;

function parseSingleLine(value: unknown, maxLength: number) {
  return typeof value === "string"
    ? value.replace(/[\r\n\0]/g, "").trim().slice(0, maxLength)
    : "";
}

function parseTestimonial(value: unknown) {
  return typeof value === "string"
    ? value.replace(/\0/g, "").trim().slice(0, 10_000)
    : "";
}

function countWords(value: string) {
  return value.trim() ? value.trim().split(/\s+/u).length : 0;
}

function validLinkedInProfileUrl(value: string) {
  try {
    const url = new URL(value);
    const hostname = url.hostname.toLowerCase().replace(/^www\./, "");
    return (
      url.protocol === "https:" &&
      hostname === "linkedin.com" &&
      /^\/in\/[^/]+\/?$/u.test(url.pathname)
    );
  } catch {
    return false;
  }
}

function validTweetUrl(value: string) {
  try {
    const url = new URL(value);
    const hostname = url.hostname.toLowerCase().replace(/^(?:www\.|mobile\.)/, "");
    return (
      url.protocol === "https:" &&
      (hostname === "x.com" || hostname === "twitter.com") &&
      /^\/[^/]+\/status\/\d+\/?$/u.test(url.pathname)
    );
  } catch {
    return false;
  }
}

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

export default {
  async fetch(request: Request, env: Env) {
    return handleJsonPost(request, env, {
      maxBodyBytes: MAX_BODY_BYTES,
      rateLimitMaxRequests: RATE_LIMIT_MAX_REQUESTS,
      parse: (body) => {
        const testimonial = parseTestimonial(body.testimonial);
        const linkedinUrl = parseSingleLine(body.linkedinUrl, 500);
        const tweetUrl = parseSingleLine(body.tweetUrl, 500);
        const testimonialWords = countWords(testimonial);

        if (
          testimonial.length < 20 ||
          testimonialWords > MAX_TESTIMONIAL_WORDS ||
          !validLinkedInProfileUrl(linkedinUrl) ||
          (tweetUrl && !validTweetUrl(tweetUrl))
        ) {
          return null;
        }

        return { testimonial, linkedinUrl, tweetUrl };
      },
      invalidError: "Please check your testimonial and profile links.",
      save: async ({ testimonial, linkedinUrl, tweetUrl }, databaseUrl) => {
        const sql = neon(databaseUrl);

        await ensureTable(databaseUrl);

        await sql`
          INSERT INTO open_agents_testimonial_submissions (testimonial, linkedin_url, tweet_url)
          VALUES (${testimonial}, ${linkedinUrl}, ${tweetUrl || null})
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
  },
};