# Landing deployment

The landing site is a Next.js static export served by **GitHub Pages**, with
Cloudflare providing DNS, HTTPS, Worker routes, and the old-domain redirects.
The public origin is `https://orchestrator.inc`.

`.github/workflows/deploy-landing.yml` builds `frontend/src/landing/out` and
deploys it on landing changes to `main`, every six hours, or a manual dispatch.
The schedule refreshes release/download data. The GitHub Pages custom domain
is a repository setting, not a Cloudflare Pages project or a build environment
variable. An Actions-based Pages deployment does not use a `CNAME` file.

## Domain configuration

- GitHub repository Settings → Pages → Custom domain: `orchestrator.inc`.
- Cloudflare `orchestrator.inc`: proxied apex A records pointing to GitHub Pages
  (`185.199.108.153`, `185.199.109.153`, `185.199.110.153`, `185.199.111.153`).
- `www.orchestrator.inc`: a redirect Worker custom domain that sends traffic to
  `https://orchestrator.inc`.
- Do not attach the landing redirect Worker to any `aoagents.dev` hostname. The
  API, staging API, status page, and other existing service origins remain
  independent and are intentionally preserved. The Android and iOS application
  IDs are `dev.openagents.mobile`.

The `open-agents-landing-domain-redirect` Worker implements these redirects. Its source
and Wrangler configuration live under `cloudflare/domain-redirect*`. It uses
an exact hostname allowlist and replaces only the scheme and host, preserving
the encoded path and query string. Deploy it separately from the static site:

```bash
wrangler deploy --config cloudflare/domain-redirect.wrangler.toml
```

The Worker handles only `www.orchestrator.inc`; it has no legacy marketing-domain
aliases. Existing `*.aoagents.dev` services are not routed through it.

These existing Workers must also have routes on the new domain; GitHub Pages
cannot execute the application's API handlers:

| Route | Worker |
| --- | --- |
| `orchestrator.inc/api/cloud-waitlist*` | `open-agents-cloud-waitlist` |
| `orchestrator.inc/api/testimonial-submissions*` | `open-agents-cloud-waitlist` |
| `orchestrator.inc/hackathons/syndicate/pass*` | `open-agents-syndicate-pass-router` |

The deployed `open-agents-cloud-waitlist` Worker handles both form routes. The two source
examples under `cloudflare/` are separate handlers; do not overwrite the
combined live Worker with just one of them. Browser forms use same-origin
relative URLs and therefore do not depend on the CORS response header.

## Asset caching

GitHub Pages sends `cache-control: max-age=600` for HTML and `max-age=14400`
for every other file, and it has no way to override that: Actions-based Pages
deployments ignore a `_headers` file, so this cannot be fixed in the repo. The
effect is that a visitor returning after four hours re-downloads the entire
JS bundle, the fonts, and all artwork even though none of it changed.

Cloudflare proxies the apex, so the headers are fixed there. Two rules, both
under the `orchestrator.inc` zone:

1. **Cache Rule** — "Next static assets", expression
   `(http.host eq "orchestrator.inc" and starts_with(http.request.uri.path, "/_next/static/"))`,
   Edge TTL *Override origin* 1 year, Browser TTL *Override origin* 1 year.
   Turbopack content-hashes every filename under `/_next/static/`, so a changed
   file is always a new URL and can never be served stale.
2. **Response Header Transform Rule** — same expression, set
   `cache-control: public, max-age=31536000, immutable`. The Cache Rule alone
   cannot emit `immutable`, which is what stops a reload from revalidating.

Artwork under `/optimized/`, `/app-icons/` and `/docs/logos/` is *not*
content-hashed (`optimize-images.mjs` writes stable names), so give it its own
Transform Rule with `public, max-age=86400, stale-while-revalidate=604800`
rather than a year.

Verify after a deploy:

```bash
curl -sI https://orchestrator.inc/_next/static/chunks/<hashed>.js | grep -i cache-control
# expect: cache-control: public, max-age=31536000, immutable
```

## Canonical-domain verification

Verify HTTPS on `orchestrator.inc`, the `www` redirect, service-domain routing,
and the Worker API routes. Deploy the static export so canonical metadata,
sitemaps, feeds, and public links use only the canonical origin.

```bash
cd frontend/src/landing
npm ci
npm run build
curl -sI https://orchestrator.inc/
curl -I 'http://www.orchestrator.inc/docs/installation/?utm_source=migration-check'
curl -IL 'https://www.orchestrator.inc/docs/installation/?utm_source=migration-check'
curl -I https://status.aoagents.dev/
curl -I https://api.aoagents.dev/
curl -I https://staging-api.aoagents.dev/
curl -I https://orchestrator.inc/hackathons/syndicate/pass/
curl -i -X OPTIONS https://orchestrator.inc/api/cloud-waitlist/
curl -i -X OPTIONS https://orchestrator.inc/api/testimonial-submissions/
```

Expect the landing page and pass to return 200, `www.orchestrator.inc` to redirect
to the canonical origin, the preserved service domains to remain independent,
and API preflights to reach the Workers. Verify `/sitemap.xml`, `/robots.txt`, and
page canonical metadata refer to `orchestrator.inc`. Avoid submitting real form
data as a deployment check.

## Rollback

Redeploy the last known-good Pages artifact, keep the `www.orchestrator.inc`
Worker attached, and verify the canonical origin and service-domain routing.
Do not restore retired marketing-domain aliases as part of a rollback.

References: [GitHub Pages custom domains](https://docs.github.com/en/pages/configuring-a-custom-domain-for-your-github-pages-site/managing-a-custom-domain-for-your-github-pages-site),
[Cloudflare Worker routes](https://developers.cloudflare.com/workers/configuration/routing/routes/),
[Cloudflare Worker custom domains](https://developers.cloudflare.com/workers/configuration/routing/custom-domains/).
