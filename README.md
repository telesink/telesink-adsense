# Telesink AdSense

A lightweight, production-ready Go service that periodically polls the [Google
AdSense Management API v2](https://developers.google.com/adsense/management) and
forwards real-time events to [Telesink](https://telesink.com) using the official
[`telesink-go` SDK](https://github.com/telesink/telesink-go).

Perfect for getting instant notifications about alerts, policy issues, site
status changes, payments, and daily earnings — all delivered cleanly into your
Telesink dashboard.

## Events sent to Telesink

| Event Name               | Description                                      | Emoji | Key Properties                                                        |
| ------------------------ | ------------------------------------------------ | ----- | --------------------------------------------------------------------- |
| `AdSense alert`          | Policy violations, payment holds, crawler errors | 🚨    | `severity`, `type`, `message`                                         |
| `AdSense policy issue`   | Active policy or regulatory issues               | ⚠️    | `action`, `site`                                                      |
| `AdSense site status`    | Site approval/deactivation changes               | 📍    | `domain`, `state`                                                     |
| `AdSense payment`        | New payments and balance updates                 | 💰    | `amount`, `currency`, `date`                                          |
| `AdSense daily earnings` | Yesterday's earnings + key metrics               | 📊    | `earnings`, `currency`, `clicks`, `impressions`, `page_views`, `date` |

## Quick start

1. Clone the repo:

   ```bash
   git clone https://github.com/telesink/telesink-adsense.git
   cd telesink-adsense
   ```

2. Edit `.env` with your credentials:

   ```bash
   cp .env.example .env
   ```

3. Start the service

   ```bash
   docker compose up -d
   ```

The poller will start immediately and continue running.

## Configuration

1. **Get your `ADSENSE_ACCOUNT_ID`**
   1. Go to https://www.google.com/adsense and sign in.
   2. In the left sidebar, click **Account** → **Settings** → **Account information**.
   3. Your **Publisher ID** will be displayed (looks like `pub-1234567890123456`).
   4. Prepend `accounts/` to it → this becomes your `ADSENSE_ACCOUNT_ID`.

      **Example:**<br>
      Publisher ID: `pub-1234567890123456`<br>
      → `ADSENSE_ACCOUNT_ID=accounts/pub-1234567890123456`

2. **Create Google Cloud Project & Enable AdSense Management API**
   1. Go to the [Google Cloud Console](https://console.cloud.google.com/)
   2. Click **Select a project** → **New Project**.
   3. Give it a name (e.g. `telesink-adsense-integration`) and click **Create**.
   4. Once the project is selected, go to **APIs & Services** → **Library**.
   5. Search for **"AdSense Management API"** and click **Enable**.

3. Create OAuth Client ID (`GOOGLE_CLIENT_ID` + `GOOGLE_CLIENT_SECRET`)
   1. In the same project, go to **APIs & Services** → **Credentials**.
   2. Click **+ Create Credentials** → **OAuth client ID**.
   3. Select **Application type** → **Web application**.
   4. Give it a name (e.g. `Telesink AdSense`).
   5. Under **Authorized redirect URIs**, click **+ Add URI** and enter exactly:

   `https://developers.google.com/oauthplayground` 6. Click **Create**. 7. Copy the **Client ID** (`GOOGLE_CLIENT_ID`) and **Client Secret** (`GOOGLE_CLIENT_SECRET`).

4. **Get your `GOOGLE_REFRESH_TOKEN` (using Google OAuth Playground)**
   1. Open the [Google OAuth 2.0 Playground](https://developers.google.com/oauthplayground).
   2. Click the gear icon (⚙️) in the top-right corner.
   3. Check the box **Use your own OAuth credentials**.
   4. Paste your **Client ID** and **Client Secret** from step 3.
   5. In **Step 1 — Select & authorize APIs**, paste this exact scope:

      `https://www.googleapis.com/auth/adsense.readonly`

   6. Click **Authorize APIs**.
   7. Sign in with the Google account that owns your AdSense account and grant the requested permissions.
   8. After authorization succeeds, click **Exchange authorization code for tokens**.
   9. Copy the **refresh_token** value (it is a long string starting with `1//` or similar).

      This is your `GOOGLE_REFRESH_TOKEN`.

      > **Important notes**
      >
      > - The refresh token never expires unless you revoke access or change your Google password.
      > - Keep `GOOGLE_CLIENT_SECRET` and `GOOGLE_REFRESH_TOKEN` private.
      > - If you ever get an `invalid_grant` error later, simply repeat step 4 to generate a fresh refresh token.

## State management

The application persists deduplication state in `state.json`. This file is
mounted as a Docker volume so events are not repeated on restarts. You can
safely delete `state.json` to reset all deduplication if needed.

## Development & local build

Install dependencies:

```bash
go mod tidy
```

Run locally:

```bash
go run .
```

Build standalone binary:

```bash
CGO_ENABLED=0 GOOS=linux go build -o telesink-adsense .
```

## License

MIT (see [LICENSE.md](/LICENSE.md)).
