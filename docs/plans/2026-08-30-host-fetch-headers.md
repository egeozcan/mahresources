# Host fetch User-Agent and per-download headers

## Problem
Every host fetch (`AddRemoteResource`, the download queue's `downloadWithProgress`,
and every HLS sub-request) goes out with Go's default `Go-http-client/1.1`
User-Agent. At least one supported platform's media endpoint answers 403 to it.
There is also no way to attach a `Referer`, `Cookie` or similar to one download.

## Design
1. New leaf package `hostfetch/`:
   - `DefaultUserAgent` — a browser-like UA string.
   - `ValidateHeaders` — intake validation (CR/LF, forbidden and hop-by-hop
     headers, `Range`, count/size bounds).
   - `Decorate(client, ua, headers, submittedURL)` — a RoundTripper decorator.
     UA on every request; custom headers **only** on requests whose `host:port`
     equals the submitted URL's. One choke point covers the initial GET, every
     HLS playlist/key/segment request and every redirect hop.
2. `-remote-user-agent` / `REMOTE_USER_AGENT`, runtime-editable as
   `remote_user_agent`.
3. `ResourceFromRemoteCreator.Headers map[string]string` — so the download
   history payload persists them and a retry replays them.
4. `headers` option on `mah.db.create_resource_from_url` and
   `mah.download.submit`, read through one shared helper.

## Non-goals
- `mah.http` keeps its own honest plugin UA.
- Form posts do not carry headers (JSON only).
