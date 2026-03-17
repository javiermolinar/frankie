# Frankie

<p align="center">
  <img src="assets/logo.png" alt="Frankie logo" />
</p>

Frankie is a self-hosted Stremio add-on server powered by **Prowlarr** + **AllDebrid**.

## Disclaimer

This project is provided for **educational and research purposes only**.

By using this software, you are solely responsible for how you use it and for complying with all applicable laws and terms of service in your country.

The author and contributors do **not** endorse or accept responsibility for any illegal, abusive, or unauthorized use.

---

## Requirements

- Prowlarr instance
- AllDebrid API key
- Stremio app (desktop recommended for `stremio://` install button)

---

## Quick start (Docker Compose)

Use the included compose file:

```bash
docker compose up -d
```

Services:
- Frankie: `http://localhost:3593`
- Prowlarr: `http://localhost:9696`

> Compose uses **named volumes** (`frankie_config`, `prowlarr_config`) to avoid host-folder permission issues.

---

## Configuration process (`/configure`)

Open:

- `http://localhost:3593/configure`

Then:

1. Fill **Prowlarr endpoint** (with compose default, use `http://prowlarr:9696`)
2. Enter **Prowlarr API key**
3. Enter **AllDebrid API key**
4. (Optional) Pick your **Primary** and **Secondary** language preference
5. Click one of:
   - **Save configuration** (save and redirect)
   - **Test connections** (save + test Prowlarr and AllDebrid immediately)

### What you should see

- `Saved key: ********` means a key is stored
- `Saved key: no` means no key stored
- You can remove saved keys with:
  - `Clear saved Prowlarr API key`
  - `Clear saved AllDebrid API key`

### Connection tests

- Prowlarr check: `GET /api/v1/system/status` on your configured Prowlarr URL
- AllDebrid check: `GET https://api.alldebrid.com/v4/user`

### Install in Stremio

From `/configure`:
- Click **Install in Stremio**

Or manually:
1. Stremio → Add-ons → Install from URL
2. Paste:
   - `http://<your-host>:3593/manifest.json`

---

## Environment variables

- `PORT` (default `3593`)
- `CONFIG_FILE` (default `config.json`; docker compose sets `/config/config.json`)
- `PUBLIC_URL` (optional; for manifest asset URLs)
- `PROWLARR_URL` (optional)
- `PROWLARR_API_KEY` (optional)
- `ALLDEBRID_API_KEY` (optional)
- `PRIMARY_LANGUAGE` (optional; must match one of the configure page language options)
- `SECONDARY_LANGUAGE` (optional; same, and must be different from primary)

Startup precedence:
1. load config file
2. override with env vars

---

## Notes

- You need to add your indexers in prowlarr to have this working https://wiki.servarr.com/prowlarr/indexers
