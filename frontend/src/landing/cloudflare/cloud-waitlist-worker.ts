import { neon } from "@neondatabase/serverless";
import { handleJsonPost } from "./json-post-handler";

type Env = {
  DATABASE_URL: string;
};

const MAX_BODY_BYTES = 4096;
const RATE_LIMIT_MAX_REQUESTS = 60;

let ensureTablePromise: Promise<void> | undefined;

function parseText(value: unknown, maxLength: number) {
  if (typeof value !== "string") {
    return "";
  }

  return value.replace(/[\r\n\0]/g, "").trim().slice(0, maxLength);
}

function parseEmail(value: unknown) {
  const email = parseText(value, 254).toLowerCase();

  if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) {
    return "";
  }

  return email;
}

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

async function ensureTable(databaseUrl: string) {
  if (!ensureTablePromise) {
    const sql = neon(databaseUrl);

    ensureTablePromise = sql`
      CREATE TABLE IF NOT EXISTS ao_cloud_waitlist (
        id BIGSERIAL PRIMARY KEY,
        email TEXT NOT NULL UNIQUE,
        role TEXT NOT NULL,
        social_profile TEXT,
        source TEXT NOT NULL DEFAULT 'ao_cloud_waitlist',
        created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
        updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
      )
    `.then(async () => {
      await sql`
        ALTER TABLE ao_cloud_waitlist
        ADD COLUMN IF NOT EXISTS social_profile TEXT
      `;
    });
  }

  return ensureTablePromise;
}

export default {
  async fetch(request: Request, env: Env) {
    return handleJsonPost(request, env, {
      maxBodyBytes: MAX_BODY_BYTES,
      rateLimitMaxRequests: RATE_LIMIT_MAX_REQUESTS,
      parse: (body) => {
        const email = parseEmail(body.email);
        const role = parseText(body.role, 120);
        const socialProfile = parseText(body.socialProfile, 300);

        if (!email || role.length < 2 || !isSocialProfile(socialProfile)) {
          return null;
        }

        return { email, role, socialProfile };
      },
      invalidError: "Please enter a valid email, role, and LinkedIn or Twitter profile.",
      save: async ({ email, role, socialProfile }, databaseUrl) => {
        const sql = neon(databaseUrl);

        await ensureTable(databaseUrl);

        await sql`
          INSERT INTO ao_cloud_waitlist (email, role, social_profile)
          VALUES (${email}, ${role}, ${socialProfile})
          ON CONFLICT (email)
          DO UPDATE SET
            role = EXCLUDED.role,
            social_profile = EXCLUDED.social_profile,
            updated_at = now()
        `;
      },
      failureLog: "AO Cloud waitlist storage failed.",
      failureError: "Unable to save waitlist request.",
    });
  },
};