import { neon } from "@neondatabase/serverless";
import { type NextRequest } from "next/server";
import { z } from "zod";
import { getDatabaseUrl, handleJsonPost } from "@/lib/json-post-handler";

const MAX_BODY_BYTES = 4096;
const RATE_LIMIT_MAX_REQUESTS = 60;

function isSocialProfile(value: string) {
  if (/^@[A-Za-z0-9_]{1,15}$/.test(value)) {
    return true;
  }

  try {
    const url = new URL(value.includes("://") ? value : `https://${value}`);
    const hostname = url.hostname.toLowerCase().replace(/^www\./, "");
    const pathParts = url.pathname.split("/").filter(Boolean);

    if (hostname === "linkedin.com" || hostname.endsWith(".linkedin.com")) {
      return pathParts.length >= 2 && pathParts[0]?.toLowerCase() === "in";
    }

    if (hostname === "x.com" || hostname === "twitter.com") {
      return pathParts.length === 1 && /^[A-Za-z0-9_]{1,15}$/.test(pathParts[0] || "");
    }
  } catch {
    return false;
  }

  return false;
}

const waitlistSchema = z.object({
  email: z.string().trim().toLowerCase().email().max(254),
  role: z.string().trim().min(2).max(120),
  socialProfile: z.string().trim().min(2).max(300).refine(isSocialProfile),
});

let ensureTablePromise: Promise<void> | undefined;

async function ensureTable(databaseUrl: string) {
  if (!ensureTablePromise) {
    const sql = neon(databaseUrl);

    ensureTablePromise = sql`
      CREATE TABLE IF NOT EXISTS open_agents_cloud_waitlist (
        id BIGSERIAL PRIMARY KEY,
        email TEXT NOT NULL UNIQUE,
        role TEXT NOT NULL,
        social_profile TEXT,
        source TEXT NOT NULL DEFAULT 'open_agents_cloud_waitlist',
        created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
      )
    `.then(async () => {
      await sql`
        ALTER TABLE open_agents_cloud_waitlist
        ADD COLUMN IF NOT EXISTS social_profile TEXT
      `;
    });
  }

  return ensureTablePromise;
}

export async function POST(request: NextRequest) {
  return handleJsonPost(request, {
    maxBodyBytes: MAX_BODY_BYTES,
    rateLimitMaxRequests: RATE_LIMIT_MAX_REQUESTS,
    parse: (body) => {
      const parsed = waitlistSchema.safeParse(body);

      return parsed.success ? parsed.data : null;
    },
    invalidError: "Please enter a valid email, role, and LinkedIn or Twitter profile.",
    save: async (data) => {
      const databaseUrl = getDatabaseUrl();
      const sql = neon(databaseUrl);

      await ensureTable(databaseUrl);

      await sql`
        INSERT INTO open_agents_cloud_waitlist (email, role, social_profile)
        VALUES (${data.email}, ${data.role}, ${data.socialProfile})
        ON CONFLICT (email)
        DO UPDATE SET
          role = EXCLUDED.role,
          social_profile = EXCLUDED.social_profile,
          updated_at = now()
      `;
    },
    failureLog: "Open Agents Cloud waitlist storage failed.",
    failureError: "Unable to save waitlist request.",
  });
}