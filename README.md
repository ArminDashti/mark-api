# mark-api

Gin + PostgreSQL CDN for company (or any) logos and icons. Serves a transparent PNG at any size.

Version: **1.0.0**

## Public URL

```
GET /m/:kind/:slug?size=128
```

- `kind`: `logo` or `icon`
- `size`: 16–2048 (default 128)
- Response: `image/png` (contain-fit in a square, transparent padding)

Example: `/m/logo/github?size=64`

## Admin API

JWT except login and public image routes.

| Method | Path | Auth |
|--------|------|------|
| GET | `/health` | no |
| POST | `/api/v1/auth/login` | no |
| GET | `/api/v1/marks?kind=` | yes |
| POST | `/api/v1/marks` | yes (multipart) |
| PUT | `/api/v1/marks/:id` | yes |
| DELETE | `/api/v1/marks/:id` | yes |

Multipart fields: `file`, `name`, `slug`, `kind`.

## Local run

```bash
docker compose -f docker-compose.local.yml up -d
copy .env.example .env
go run ./cmd/server
```

- API: `http://127.0.0.1:8130`
- Postgres: `localhost:5437`
- Default login: `armin` / `dopadopa123`

Uploads: PNG, WebP, SVG (rasterized on serve). JPEG is accepted but kept without invented transparency.
