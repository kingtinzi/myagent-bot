# Platform Backend

This module hosts the platform APIs for:

- app account login/signup via Supabase
- official model proxying
- wallet and recharge orders
- agreement and official model listing

Environment variables:

- `PLATFORM_ADDR`
- `PLATFORM_DATABASE_URL`
- `PLATFORM_SUPABASE_URL`
- `PLATFORM_SUPABASE_ANON_KEY`
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
- `PLATFORM_OFFICIAL_ROUTES_JSON`
- `PLATFORM_OFFICIAL_MODELS_JSON`
- `PLATFORM_PRICING_RULES_JSON`
- `PLATFORM_AGREEMENTS_JSON`

At startup, `platform-server` auto-loads the explicit live env file when present:

- `config/platform.env`

The example file `config/platform.example.env` is documentation only. Copy it to `config/platform.env` before use if you want a local file-backed setup.

Admin UI:

- Open `http://127.0.0.1:18791/admin`
- Sign in with a Supabase account listed in `PLATFORM_ADMIN_EMAILS`
- The admin page uses `/auth/login` plus the protected `/admin/runtime-config` and `/admin/models` APIs
- Runtime config persists model routes, enabled models, pricing rules, and recharge agreements to `PLATFORM_RUNTIME_CONFIG_PATH`
- Release packages ship `config/runtime-config.example.json` as an example only. Copy it to `config/runtime-config.json` if you want to start from the sample official-model config.

Run locally:

```powershell
$env:PLATFORM_SUPABASE_URL='https://your-project.supabase.co'
$env:PLATFORM_SUPABASE_ANON_KEY='your-anon-key'
$env:PLATFORM_SUPABASE_JWT_SECRET='your-local-jwt-secret'
$env:PLATFORM_ADMIN_EMAILS='admin@example.com'
$env:PLATFORM_RUNTIME_CONFIG_PATH='.\config\runtime-config.json'
$env:PLATFORM_PAYMENT_PROVIDER='easypay'
$env:PLATFORM_PUBLIC_BASE_URL='http://127.0.0.1:18791'
$env:PLATFORM_EASYPAY_BASE_URL='https://pay.example.com'
$env:PLATFORM_EASYPAY_PID='10001'
$env:PLATFORM_EASYPAY_KEY='secret'
go run ./cmd/platform-server
```

Convenience local startup:

```powershell
.\scripts\start-local-platform.ps1
```

```bash
./scripts/start-local-platform.sh
```
