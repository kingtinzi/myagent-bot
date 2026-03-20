# Platform Backend

## Repository vs deployment

- **`Platform/` in this repo** is the **server** codebase: build `platform-server`, configure `config/platform.env`, deploy to your VPS (or run locally). Keep it in the repo for development and releases.
- **Desktop clients** (Launcher + embedded PinchBot gateway) talk to that API using a **client base URL**, not by requiring a local `platform-server` binary. Set **`PICOCLAW_PLATFORM_API_BASE_URL`** (e.g. in `{install root}/config/platform.env` next to the `.app`) to your public API root, e.g. `http://your-host:18793`. When this points to a **non-loopback** host, the Launcher **does not** auto-start a local `platform-server`.

This module hosts the platform APIs for:

- app account login/signup via Supabase
- official model proxying
- wallet and recharge orders
- agreement and official model listing

Environment variables (server — `platform-server`):

- `PLATFORM_ADDR`
- `PLATFORM_DATABASE_URL`
- `PLATFORM_SUPABASE_URL`
- `PLATFORM_SUPABASE_ANON_KEY` (fill this with your Supabase publishable/anon key)
- `PLATFORM_SUPABASE_JWKS_URL`
- `PLATFORM_SUPABASE_JWT_SECRET`
- `PLATFORM_SUPABASE_AUDIENCE`
- `PLATFORM_ADMIN_EMAILS`
- `PLATFORM_RUNTIME_CONFIG_PATH`
- `PLATFORM_PAYMENT_PROVIDER`
- `PLATFORM_PUBLIC_BASE_URL`
- `PLATFORM_EASYPAY_BASE_URL`
- `PLATFORM_EASYPAY_PID`
- `PLATFORM_EASYPAY_KEY`
- `PLATFORM_EASYPAY_TYPE`
- `PLATFORM_ALIMPAY_BASE_URL`
- `PLATFORM_ALIMPAY_PID`
- `PLATFORM_ALIMPAY_KEY`
- `PLATFORM_ALIMPAY_TYPE`
- `PLATFORM_OFFICIAL_ROUTES_JSON`
- `PLATFORM_OFFICIAL_MODELS_JSON`
- `PLATFORM_PRICING_RULES_JSON`
- `PLATFORM_AGREEMENTS_JSON`

Desktop / embedded gateway (Launcher + PinchBot) — **client** side (often set in `{install root}/config/platform.env`):

- `PICOCLAW_PLATFORM_API_BASE_URL` — HTTP root of the deployed platform API (same host/port you curl for `/auth/login`, `/wallet`, etc.). Passed through to PinchBot `LoadConfig` via `env.Parse`, so it overrides `platform_api.base_url` in `config.json` when set.

Recommended database connection for this deployment:

- If `platform-server` runs on a normal IPv4 VPS/server, use the Supabase `Session Pooler` connection string as `PLATFORM_DATABASE_URL`
- Do not use the `Direct connection` string unless the server network supports IPv6 or you purchased the Supabase IPv4 add-on
- For your Japan-server layout, if the Supabase project is also in Tokyo, the Session Pooler host is typically `aws-0-ap-northeast-1.pooler.supabase.com`
- Current Supabase projects should prefer JWKS-based JWT verification. `PLATFORM_SUPABASE_JWT_SECRET` is only a legacy fallback and can be left empty when `PLATFORM_SUPABASE_URL`/`PLATFORM_SUPABASE_JWKS_URL` are set
- Before the first real startup, apply every SQL file in `migrations/` in filename order so `wallet_accounts`, `user_profiles`, `admin_users`, and related tables exist

At startup, `platform-server` auto-loads the explicit live env file when present:

- `config/platform.env`

The example file `config/platform.example.env` is documentation only. Copy it to `config/platform.env` before use if you want a local file-backed setup.

Admin UI:

- Open `http://127.0.0.1:18791/admin-v2` to preview the new React admin console
- Keep `http://127.0.0.1:18791/admin` as the legacy fallback during the transition
- Sign in with a Supabase account listed in `PLATFORM_ADMIN_EMAILS`
- The admin pages use `/admin/session/login` plus the protected `/admin/*` APIs
- Runtime config persists model routes, enabled models, pricing rules, and recharge agreements to `PLATFORM_RUNTIME_CONFIG_PATH`
- Release packages ship `config/runtime-config.example.json` as an example only. Copy it to `config/runtime-config.json` if you want to start from the sample official-model config.

Signup/login behavior:

- This desktop flow expects signup to produce a usable Supabase session immediately
- If `/auth/signup` returns an error mentioning `Disable Confirm email`, turn off **Confirm email** in Supabase Auth settings, or enable unverified-email sign-ins for this project
- The platform now returns an actionable error instead of silently storing an incomplete session when Supabase only creates the user record but does not issue a session

Run locally:

```powershell
$env:PLATFORM_DATABASE_URL='postgresql://postgres.<project-ref>:<db-password>@aws-0-ap-northeast-1.pooler.supabase.com:5432/postgres?sslmode=require'
$env:PLATFORM_SUPABASE_URL='https://<project-ref>.supabase.co'
$env:PLATFORM_SUPABASE_ANON_KEY='your-anon-key'
$env:PLATFORM_SUPABASE_JWT_SECRET=''
$env:PLATFORM_ADMIN_EMAILS='admin@example.com'
$env:PLATFORM_RUNTIME_CONFIG_PATH='.\config\runtime-config.json'
$env:PLATFORM_PAYMENT_PROVIDER='easypay'
$env:PLATFORM_PUBLIC_BASE_URL='https://api.your-domain.com'
$env:PLATFORM_EASYPAY_BASE_URL='https://pay.example.com'
$env:PLATFORM_EASYPAY_PID='10001'
$env:PLATFORM_EASYPAY_KEY='secret'
go run ./cmd/platform-server
```

AliMPay temporary integration notes:

- Set `PLATFORM_PAYMENT_PROVIDER=alimpay`
- Point `PLATFORM_ALIMPAY_BASE_URL` to your AliMPay deployment root, e.g. `https://pay.example.com`
- Fill `PLATFORM_ALIMPAY_PID` and `PLATFORM_ALIMPAY_KEY` with the merchant credentials generated by AliMPay after its first successful startup
- The platform publishes merchant callback endpoints at `/payments/alimpay/notify` and accepts both `GET` and `POST` callbacks because AliMPay sends querystring-based merchant notifications

Convenience local startup:

```powershell
.\scripts\start-local-platform.ps1
```

```bash
./scripts/start-local-platform.sh
```
