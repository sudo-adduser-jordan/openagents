const RATE_LIMIT_WINDOW_MS = 60_000;
const buckets = new Map<string, { count: number; resetAt: number }>();

function json(body: { ok: boolean; error?: string }, status = 200) {
  return Response.json(body, {
    status,
    headers: {
      "Cache-Control": "no-store",
      "Access-Control-Allow-Origin": "https://orchestrator.inc",
      "Access-Control-Allow-Methods": "POST, OPTIONS",
      "Access-Control-Allow-Headers": "Content-Type",
    },
  });
}

async function hash(value: string) {
  const bytes = new TextEncoder().encode(value);
  const digest = await crypto.subtle.digest("SHA-256", bytes);

  return [...new Uint8Array(digest)]
    .map((byte) => byte.toString(16).padStart(2, "0"))
    .join("");
}

async function rateLimitKey(request: Request) {
  return hash(request.headers.get("cf-connecting-ip") || "unknown");
}

function isRateLimited(key: string, maxRequests: number) {
  const now = Date.now();
  const bucket = buckets.get(key);

  if (!bucket || bucket.resetAt <= now) {
    buckets.set(key, {
      count: 1,
      resetAt: now + RATE_LIMIT_WINDOW_MS,
    });
    return false;
  }

  bucket.count += 1;
  return bucket.count > maxRequests;
}

export type WorkerJsonPostConfig<TData> = {
  /** Hard cap on the accepted body size, checked before and after reading. */
  maxBodyBytes: number;
  /** Max requests per IP per 60s window. */
  rateLimitMaxRequests: number;
  /** Validate/normalize the parsed body; return null to reject with `invalidError`. */
  parse: (body: Record<string, unknown>) => TData | null;
  invalidError: string;
  /** Persist the validated payload. */
  save: (data: TData, databaseUrl: string) => Promise<void>;
  failureLog: string;
  failureError: string;
};

/**
 * Shared plumbing for Cloudflare JSON POST workers: CORS, content-type/size
 * guards, per-IP rate limiting, body parsing, validation, and a uniform
 * success/error envelope. Endpoints supply their validation (`parse`) and
 * persistence (`save`).
 */
export async function handleJsonPost<TData>(
  request: Request,
  env: { DATABASE_URL: string },
  config: WorkerJsonPostConfig<TData>,
): Promise<Response> {
  if (request.method === "OPTIONS") {
    return json({ ok: true });
  }

  if (request.method !== "POST") {
    return json({ ok: false, error: "Method not allowed." }, 405);
  }

  const contentType = request.headers.get("content-type") || "";

  if (!contentType.includes("application/json")) {
    return json({ ok: false, error: "Invalid request." }, 415);
  }

  const contentLength = Number(request.headers.get("content-length") || 0);

  if (contentLength > config.maxBodyBytes) {
    return json({ ok: false, error: "Request too large." }, 413);
  }

  if (isRateLimited(await rateLimitKey(request), config.rateLimitMaxRequests)) {
    return json({ ok: false, error: "Please try again in a minute." }, 429);
  }

  let body: Record<string, unknown>;

  try {
    const rawBody = await request.text();

    if (new TextEncoder().encode(rawBody).length > config.maxBodyBytes) {
      return json({ ok: false, error: "Request too large." }, 413);
    }

    const parsed = JSON.parse(rawBody);
    body = parsed && typeof parsed === "object" ? parsed : {};
  } catch {
    return json({ ok: false, error: "Invalid request." }, 400);
  }

  const data = config.parse(body);

  if (data === null) {
    return json({ ok: false, error: config.invalidError }, 400);
  }

  try {
    await config.save(data, env.DATABASE_URL.trim().replace(/^\uFEFF/, ""));
    return json({ ok: true });
  } catch {
    console.error(config.failureLog);
    return json({ ok: false, error: config.failureError }, 500);
  }
}