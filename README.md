# Frankie

<p align="center">
  <img src="assets/logo.png" alt="Frankie logo" width="120" />
</p>

Frankie is a self-hosted Stremio add-on server baked by prowlarr and alldebrid.

This project is intended strictly for educational and research purposes. It was created to demonstrate concepts and techniques related to the technology used in this repository.
The author does not encourage, support, or condone any illegal, unethical, or malicious use of this software.

---

## Requirements

- A running **Prowlarr** instance
- A valid **AllDebrid API key**
- Stremio desktop app (recommended for `stremio://` install button)

---

## Installation

### Option 1: Docker Compose (recommended)
 
Use the included `docker-compose.yml`:

```bash
docker compose up -d
```

This starts:
- `frankie` on `http://localhost:3593`
- `prowlarr` on `http://localhost:9696`

### Option 2: Docker image directly

```bash
docker run -d \
  --name frankie \
  -p 3593:3593 \
  -v $(pwd)/config:/config \
  javimolinar/frankie:latest
```

Then open `http://localhost:3593/configure`.

### Option 3: Run from source

```bash
go test ./...
go run ./src
```

Server default: `http://localhost:3593`

---

## Configuration

Open:

- `http://localhost:3593/configure`

Fill in:
- **Prowlarr endpoint** (example: `http://prowlarr:9696` in Docker Compose)
- **Prowlarr API key**
- **AllDebrid API key**

Click **Save configuration**.

### Where config is saved

- In Docker image: `/config/config.json`
- Local default: `./config.json`
- Override path with env var:
  - `CONFIG_FILE=/path/to/config.json`

---

## Install in Stremio

From `/configure`:
- Click **Install in Stremio**

Or manually in Stremio:
1. Open Add-ons
2. Install from URL
3. Paste:
   - `http://<your-host>:3593/manifest.json`

> If Stremio runs on another device, use a host/IP reachable from that device.

---

## Environment variables

- `PORT` (default: `3593`)
- `CONFIG_FILE` (default: `config.json`, Docker sets `/config/config.json`)
- `PUBLIC_URL` (optional; used for manifest logo/background URLs)
- `PROWLARR_URL` (optional, can be saved in config file)
- `PROWLARR_API_KEY` (optional, can be saved in config file)
- `ALLDEBRID_API_KEY` (optional, can be saved in config file)

### Precedence

At startup:
1. Config file is loaded
2. Environment variables override file values

---

## Notes

- Keep your API keys private.

