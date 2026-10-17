---
id: int-reference-implementation
title: Client integration guide
kind: integration
status: current
summary: Step-by-step procedure for a client team to send feedback to the service — onboarding, the request, retries, errors, key rotation, a web proxy and a desktop outline.
read_when: you are wiring a new client application (desktop, mobile or web) to the feedback service, or changing what clients must send or handle
updated: 2026-10-17
verified: 2026-10-17
code_anchors: [src/intake.ts]
depends_on: [feat-intake, feat-intake-keys, feat-applications]
review_cycle: monthly
---

# Client integration guide

This guide is for a developer who has to make an application send feedback. You do not need the
server code. Each rule below is owned by another document, linked where it matters; this page puts
them in the order you meet them.

## 1. Pick your client style

| Style | Example | Where the key lives | Request goes |
|---|---|---|---|
| **Desktop or mobile app** | A desktop app with a Send Feedback dialog | Compiled in or in secure configuration of the app | Straight to the service |
| **Web app** | A browser widget that captures and annotates a screenshot | On **your own server only** | Browser to your server, then your server to the service |

The service has no CORS and accepts only JSON, so a browser cannot call it directly, by design: an
intake key authenticates anyone who holds it, and a browser would hand it to every visitor. Web apps
use the proxy pattern in section 10.

## 2. Onboarding

Done once per application by a service admin, in the backoffice.

1. **Create the application** (Applications screen): a slug (`my-app`, permanent), a display name,
   and the **kinds** your app will send (for example `bug`, `improvement`, `note`). Kinds are
   per-application; rules in [`applications.md`](../features/applications.md). Map your UI's
   categories one to one onto kinds, because a kind the application does not list is rejected.
2. **Create an intake key** (Intake keys screen): choose the application, give the key a name that
   says who holds it (`production build`, `web backend`), and optionally an expiry. The key looks like
   `fbk_ik_...` and **is shown once**.
3. **Hand it over securely**: through a secret store or password manager, straight into your CI secret
   or server configuration. Never through chat, email, tickets or the repository. If it was shown and
   lost, the admin issues a new key; nothing can retrieve the old one.
4. **Verify** with the connectivity check in section 8 before writing any UI.

The service URL (for example `https://feedback.example.com`) and the key are all a client needs.
Keys, expiry and revocation: [`intake-keys.md`](../features/intake-keys.md).

## 3. Where the key lives

| Client | Do | Do not |
|---|---|---|
| Desktop or mobile | Inject at build time or read from secure per-user or OS configuration | Commit it, log it, show it in the UI |
| Web | Keep it in your server's environment or secret manager; read it only in the proxy | Put it in browser code, HTML, a public config endpoint or a cookie |

Treat a desktop key as extractable by a determined user. The blast radius is limited: it only submits
reports into one application, and an admin can revoke it. Plan for rotation (section 11).

## 4. The request

`POST <service-url>/api/feedback`

| Header | Value |
|---|---|
| `X-Api-Key` | The intake key (or `Authorization: Bearer <key>`) |
| `Content-Type` | `application/json` |

The body is one JSON object. The field rules and limits are owned by
[`intake.md`](../features/intake.md); the summary is:

| Field | Required | Limit | Send |
|---|---|---|---|
| `id` | recommended | 26-character ULID | A new ULID per report, reused on every retry |
| `title` | yes | 1–300 characters, not blank | The user's summary |
| `kind` | yes | one of the application's kinds | The user's category |
| `description` | no | 20 000 characters | The details; may be empty |
| `version` | no | 100 characters | Your app version or build |
| `platform` | no | 100 characters | For example `macos/aarch64` or `web` |
| `context` | no | JSON object, 16 384 bytes serialized | Diagnostic metadata, below |
| `screenshot_png_base64` | no | 14 000 000 characters | Section 6 |

Anything else in the object is ignored. The whole request is capped at 15 000 000 bytes.

### Context conventions

`context` is free-form and flat keys display best. These names are conventional, so reports from
different applications look alike in the backoffice. Values that are `http(s)` URLs become links;
objects display as JSON.

| Key | Content |
|---|---|
| `url` | The page or screen the user was on |
| `user_agent` | The browser or runtime string |
| `locale` | For example `it-IT` |
| `user` | A stable user identifier your team can look up (an id, not a password or token) |
| `build` | Build number, commit or release channel, when `version` is not enough |
| `client_ts` | When the user submitted, ISO-8601 |
| `viewport` | For example `1440x900` |

Send what helps to reproduce the problem and nothing more: context lands in a shared backoffice, so
leave out secrets, tokens and personal data you do not need.

```json
{ "id": "01JC0000000000000000000000", "title": "Export button does nothing", "kind": "bug",
  "description": "Clicked Export on the reports page.", "version": "2.4.1", "platform": "web",
  "context": { "url": "https://app.example.com/reports", "locale": "it-IT", "user": "u_8123" } }
```

## 5. Idempotency and retries

Mint a **ULID** when the user submits, store it with the report, and send the same `id` on every
attempt for that report. The service then treats a repeat as a resend: it answers `200` with the
original reference and creates nothing. A resend counts as a success, and a lost reply does not
create a duplicate. Without an `id`, each request creates a new report.

| Outcome | Action |
|---|---|
| `201` or `200` | Done. Keep the reference; stop retrying |
| Network error, timeout | Retry |
| `429` | Retry, after a longer wait; the limit is per IP and per minute |
| `500`, `502`, `503`, `504` | Retry |
| `400`, `401`, `409`, `413` | **Never retry.** The same request fails again. Fix the cause or tell a human |

Suggested policy (the service sets none): exponential backoff with jitter, starting at one second
(five seconds after a `429`), doubling to a cap of one minute, five to eight attempts, then park the
report for a later manual or startup retry. Keep the same `id`.

## 6. Screenshots

- **Format:** PNG only. A JPEG or WebP is `400 screenshot is not a PNG`.
- **Encoding:** standard base64 of the raw file bytes, without a `data:image/png;base64,` prefix.
- **Size:** at most 14 000 000 base64 characters (about 10 MB of image) or the service answers `413`.
  Downscale or re-encode large captures before sending; a viewport capture is usually enough.
- **Optional:** omit the field, or send `null` or an empty string, to send none. A report without a
  screenshot is better than one that fails; if the capture is too large, drop it and send the report.
- **Not multipart:** the bytes go inside the JSON, never as a file part.

## 7. Responses and what to show

Success is `201` (new) or `200` (resend of a known `id`), with the same body:

```json
{ "reference": "FB-42", "url": "https://feedback.example.com/admin/#/feedback?open=01JC...", "application": "my-app" }
```

- Show the **reference** (`FB-42`) to the user as the confirmation, so they can quote it later.
- The **url** opens the report in the backoffice. Show it only to people who have backoffice access
  (internal users, support staff); end users cannot sign in, so give them the reference only. Whether
  end users should get a link is an open product question.
- Treat the body as extensible and ignore keys you do not know.

## 8. Connectivity check

`POST <service-url>/api/feedback/test` with the same header. It creates nothing and is safe to call
from a settings screen or a deploy smoke test. It shares the per-IP throttle.

```json
{ "ok": true, "key": "production build", "application": { "slug": "my-app", "name": "My App" },
  "kinds": ["bug", "improvement", "note"], "service": "feedback-tool", "version": "0.1.0" }
```

`key` is the key's name, `kinds` is what the application accepts: use it to fill a category picker
or to fail early when your UI and the application disagree. A bad key answers `401 {"ok":false}`.
Do not depend on the `service` string: its value is an open question for existing clients.

## 9. Errors

Error bodies are `{"error": "..."}`. The full list is under Failure in
[`intake.md`](../features/intake.md).

| Status | `error` | Meaning | Retry |
|---|---|---|---|
| `400` | `invalid JSON`, `invalid body` | Not a JSON object, or a bad `id`, `title`, `description`, `version` or `platform` | No |
| `400` | `invalid kind` (with `kinds`) | Kind not in the application's list | No: use one of `kinds` |
| `400` | `invalid context`, `context too large` | Not an object, or over 16 384 bytes | No |
| `400` | `invalid screenshot`, `screenshot is not a PNG` | Bad base64 or not a PNG | No: fix or drop the screenshot |
| `401` | `unauthorized` | Key unknown, revoked or expired, application archived, or a token used as a key | No: escalate; the key needs replacing |
| `404` | `not found` | Wrong path or method | No |
| `409` | `id conflict` | The `id` belongs to a report of another application | No: mint a new ULID |
| `413` | `too large` | Body or screenshot over the limit | No: shrink |
| `429` | `rate limited` | More than 10 requests in 60 seconds from your IP | Yes, later |
| `500` | `internal error`, `storage failure` | Service fault | Yes |

Log the status and `error` for every failure, never the key. A `401` in production means the key was
revoked or expired: alert your operators instead of silently dropping reports.

## 10. Web apps: the server-side proxy

The browser sends the report to **your** endpoint (the Swingo widget posts multipart form data to
`/api/v1/feedback`); your server turns it into the JSON above and forwards it with the key. The
proxy also authenticates your own user, because the service cannot tell your visitors apart.

1. Authenticate the caller (your session) and apply your own per-user rate limit and size limit.
2. Read the form fields; validate `category` against the application's kinds.
3. Read the screenshot file, check it is PNG and small enough, and base64 it.
4. Mint one ULID, build `context` from the form (`current_url`, `ts`) and the request headers.
5. Forward; retry only the statuses in section 5, with the same `id`.
6. Answer the browser with `201` and the reference. The widget treats only `201` as success, so
   answer `201` even when the service answered `200` for a resend. Map `429` to `429`; map every
   other failure to a generic error, never forwarding the service's body or the key.

```ts
// Web-standard handler (Hono, Next.js route, Workers...). Env: FEEDBACK_URL, FEEDBACK_KEY.
import { ulid } from 'ulid';
const KINDS = new Set(['bug', 'improvement', 'note']);
export async function POST(req: Request): Promise<Response> {
  const user = await requireUser(req);                       // your auth; 401 if none
  const f = await req.formData();
  const kind = String(f.get('category')), title = String(f.get('title') ?? '').trim();
  if (!KINDS.has(kind) || !title) return Response.json({ error: 'invalid' }, { status: 400 });
  const file = f.get('screenshot');
  let shot: string | undefined;
  if (file instanceof File && file.type === 'image/png') {
    const b64 = Buffer.from(await file.arrayBuffer()).toString('base64');
    if (b64.length <= 14_000_000) shot = b64;                // too big: send the report without it
  }
  // ...continued below
```

The handler continues by building the JSON and forwarding it:

```ts
  const context = { url: f.get('current_url'), client_ts: f.get('ts'), user: user.id,
    user_agent: req.headers.get('user-agent'), locale: req.headers.get('accept-language')?.split(',')[0] };
  const body = JSON.stringify({ id: ulid(), title: title.slice(0, 300), kind, platform: 'web',
    description: String(f.get('description') ?? ''), screenshot_png_base64: shot, context });
  const res = await sendWithRetry(`${process.env.FEEDBACK_URL}/api/feedback`, body); // section 5
  if (res.ok) return Response.json({ reference: (await res.json()).reference }, { status: 201 });
  return Response.json({ error: 'unavailable' }, { status: res.status === 429 ? 429 : 502 });
}
```

`sendWithRetry` does the `POST` with `X-Api-Key: ${process.env.FEEDBACK_KEY}` and
`Content-Type: application/json`, retrying network errors, `429` and `5xx` with backoff and the same
body (hence the same `id`). A user clicking Send twice creates two ULIDs and so two reports; to
prevent it, have the widget generate the `id` and post it as a form field, then use that value.

Test the whole path by hand, replacing the key and URL:

```sh
curl -sS -X POST "$FEEDBACK_URL/api/feedback" \
  -H "X-Api-Key: $FEEDBACK_KEY" -H 'Content-Type: application/json' \
  -d '{"id":"01JC0000000000000000000001","title":"Proxy test","kind":"note","platform":"web"}'
```

Run it twice: the first answer is `201`, the second `200` with the same reference. Note that every
user's report leaves your server from one address, so they share the service's per-IP allowance of 10
requests per minute ([`backlog.md`](../backlog.md)); throttle per user on your side and do not
retry a `429` in a tight loop.

## 11. Desktop apps: outline

1. Read the URL and key from build-time or secure configuration; expose a **Test** button that calls
   `/test` and shows `application.name` and `kinds`.
2. On Send, build the report, mint a ULID, and write it with the screenshot to a **local outbox**
   before any network call.
3. Send it. On `200` or `201`, delete it from the outbox and show the reference.
4. On a retryable failure, leave it in the outbox, retry with backoff while the dialog is open, then
   again at the next start or when connectivity returns. Same `id` every time.
5. On `400`, `409` or `413` show the error and let the user fix or drop the report. On `401` tell the
   user feedback is unavailable and log it; do not loop.
6. Fill `version` and `platform` from the app itself and put anything else in `context`.

## 12. Key rotation

1. In the backoffice, create a **second** key for the same application. Both work at once.
2. Ship the new key to your clients (a new build, a new secret value, a server restart).
3. Watch the old key's **Last used** column on the Intake keys screen; when it stops moving, nobody
   is using it. Desktop builds in the field keep the old key until users update, so allow for them.
4. **Revoke** the old key. It stops working on the next request (`401`).

A leaked key is revoked first and replaced after, accepting that clients fail with `401` until they
get the new one. A key set to expire stops working on its date: schedule the replacement before it.

## 13. Checklist

- [ ] Application and kinds exist; your UI categories map onto the kinds
- [ ] Key stored server-side or in secure app configuration, absent from source control and logs
- [ ] Web: the browser never receives the key; your server authenticates the user first
- [ ] `/test` succeeds from the real environment, and the picker uses the returned `kinds`
- [ ] A ULID `id` is minted per report and reused on every retry
- [ ] Retries only on network errors, `429` and `5xx`, with backoff and a cap; never on `400`, `401`, `409`, `413`
- [ ] Screenshots are PNG, base64 without a prefix, under 14 000 000 characters, dropped if too large
- [ ] The reference (`FB-n`) is shown to the user; the `url` only to backoffice users
- [ ] `version`, `platform` and the `context` conventions are filled; no secrets in `context`
- [ ] `401` raises an operator alert; no key in logs
- [ ] Rotation procedure written down, with an owner

## Related docs

- [`../features/intake.md`](../features/intake.md) — fields, limits, replies, errors
- [`../features/intake-keys.md`](../features/intake-keys.md) — issuing, expiry, revocation
- [`../features/applications.md`](../features/applications.md) — applications and kinds
- [`../tech/api-reference.md`](../tech/api-reference.md) — every route
- [`../tech/security.md`](../tech/security.md) — why a browser must not hold the key
- [`../backlog.md`](../backlog.md) — browser-direct intake, throttle per key, the reply `url`
