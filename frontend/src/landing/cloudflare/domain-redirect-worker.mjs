// Only the canonical www hostname redirects. Service domains under aoagents.dev
// remain independent origins and are intentionally not handled here.
const aliases = new Set(["www.orchestrator.inc"]);

export default {
  fetch(request) {
    const url = new URL(request.url);
    if (!aliases.has(url.hostname)) {
      return new Response("Not found", { status: 404 });
    }
    url.protocol = "https:";
    url.host = "orchestrator.inc";
    url.port = "";
    return Response.redirect(url.href, 308);
  },
};
