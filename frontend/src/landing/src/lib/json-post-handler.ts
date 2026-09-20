import { createHash } from "node:crypto";
import { type NextRequest, NextResponse } from "next/server";

const RATE_LIMIT_WINDOW_MS = 60_000;
const submissionBuckets = new Map<string, { count: number; resetAt: number }>();

function json(body: { ok: boolean; error?: string }, status = 200) {
  return NextResponse.json(body, {
    status,
    headers: {
      "Cache-Control": "no-store",
    },
  });
}

function getRateLimitKey(request: NextRequest) {
  const forwardedFor = request.headers.get("x-forwarded-for");
  const ip =
    forwardedFor?.split(",")[0]?.trim() ||
    request.headers.get("x-real-ip") ||
    "unknown";

  return createHash("sha256").update(ip).digest("hex");
}

function isRateLimited(key: string, maxRequests: number) {
  const now = Date.now();
  const bucket = submissionBuckets.get(key);

  if (!bucket || bucket.resetAt <= now) {
    submissionBuckets.set(key, {
      count: 1,
      resetAt: now + RATE_LIMIT_WINDOW_MS,
    });
    return false;
  }

  bucket.count += 1;
  return bucket.count > maxRequests;
}

export function getDatabaseUrl() {
  const databaseUrl = process.env.DATABASE_URL?.trim().replace(/^\uFEFF/, "");

  if (!databaseUrl) {
    throw new Error("DATABASE_URL is not configured");
  }

  return databaseUrl;
}

export type JsonPostConfig<TData> = {
  /** Hard cap on the accepted body size, checked before and after reading. */
  maxBodyBytes: number;
  /** Max requests per IP per 60s window. */
  rateLimitMaxRequests: number;
  /** Validate/normalize the parsed body; return null to reject with `invalidError`. */
  parse: (body: unknown) => TData | null;
  invalidError: string;
  /** Persist the validated payload. */
  save: (data: TData) => Promise<void>;
  failureLog: string;
  failureError: string;
};

/**
 * Shared plumbing for JSON POST endpoints: content-type/size guards, per-IP
 * rate limiting, body parsing, validation, and a uniform success/error
 * envelope. Endpoints supply their schema (`parse`) and persistence (`save`).
 */
export async function handleJsonPost<TData>(
  request: NextRequest,
  config: JsonPostConfig<TData>,
): Promise<NextResponse> {
  const contentType = request.headers.get("content-type") || "";

  if (!contentType.includes("application/json")) {
    return json({ ok: false, error: "Invalid request." }, 415);
  }

  const contentLength = Number(request.headers.get("content-length") || 0);

  if (contentLength > config.maxBodyBytes) {
    return json({ ok: false, error: "Request too large." }, 413);
  }

  if (isRateLimited(getRateLimitKey(request), config.rateLimitMaxRequests)) {
    return json({ ok: false, error: "Please try again in a minute." }, 429);
  }

  let body: unknown;

  try {
    const rawBody = await request.text();

    if (new TextEncoder().encode(rawBody).length > config.maxBodyBytes) {
      return json({ ok: false, error: "Request too large." }, 413);
    }

    body = JSON.parse(rawBody);
  } catch {
    return json({ ok: false, error: "Invalid request." }, 400);
  }

  const data = config.parse(body);

  if (data === null) {
    return json({ ok: false, error: config.invalidError }, 400);
  }

  try {
    await config.save(data);
    return json({ ok: true });
  } catch {
    console.error(config.failureLog);
    return json({ ok: false, error: config.failureError }, 500);
  }
}