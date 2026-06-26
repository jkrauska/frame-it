# frame-it

Upload images to a Samsung Frame TV over your local network.

There is no official Samsung API for Art Mode. This tool uses the same unofficial WebSocket protocol reverse-engineered by the community ([samsung-tv-ws-api](https://github.com/xchwarze/samsung-tv-ws-api), [frame-tv-art-manager](https://github.com/MikeO7/frame-tv-art-manager), and others).

## Requirements

- Go 1.24+
- Samsung "The Frame" TV on the same LAN as this computer
- Static IP or DHCP reservation for the TV (optional — auto-discovery works for a single TV)
- TV firmware with Art Mode API access (most Frame models 2018+)

## Build

```bash
make all      # build ./frame-it
make clean    # remove ./frame-it
make run      # build and run (pass ARGS= for CLI flags)
```

Examples:

```bash
make run ARGS="discover"
make run ARGS="wallpaper mountains --download-only --save ./art.jpg"
```

Or build manually:

```bash
go build -o frame-it ./cmd/frame-it
```

Install from source:

```bash
go install github.com/jkrauska/frame-it/cmd/frame-it@latest
```

## Quick start

1. Discover your TV (mDNS + SSDP — no IP needed):

```bash
frame-it discover
```

2. Pair once — the TV shows an authorization prompt; accept with the remote:

```bash
frame-it pair
```

3. Upload an image (3840×2160 JPG recommended):

```bash
frame-it upload ./my-photo.jpg
```

If you have one Frame TV on the LAN, `--host` is optional — the IP is saved to `~/.frame-it/config.json` after the first successful discovery. With multiple TVs, use `discover` and pass `--host`.

4. Fetch wallpaper art and display it:

```bash
frame-it wallpaper --source unsplash mountains
frame-it wallpaper --source pixabay "alpine lake"
frame-it wallpaper --source wallhaven nature
frame-it wallpaper --source unsplash --download-only --save ./art.jpg
```

Get free API keys from [Unsplash Developers](https://unsplash.com/developers) and [Pixabay API](https://pixabay.com/api/docs/). Wallhaven keys are optional for SFW content.

## Commands

| Command | Description |
|---------|-------------|
| `discover` | Scan LAN for Samsung TVs (mDNS + SSDP) |
| `pair` | Save an auth token (accept TV prompt) |
| `upload <files...>` | Upload JPG/PNG and display the last one |
| `wallpaper [query]` | Fetch 4K wallpaper art and upload to TV |
| `list` | List uploaded images on the TV |
| `select <content-id>` | Show an existing image |
| `delete <content-id...>` | Remove images from the TV |

## Options

| Flag | Default | Description |
|------|---------|-------------|
| `--host` | | TV IP (`$FRAME_IT_HOST`, then saved config, then auto-discover) |
| `--token-dir` | `~/.frame-it` | Auth tokens and `config.json` |
| `--name` | `frame-it` | Name shown on TV authorization prompt |
| `--matte` | `none` | Matte style ID (`shadowbox_polar`, `modern_apricot`, …) |
| `--no-matte` | | Same as `--matte none` |
| `--show` | `true` | After upload, switch the TV to the new image |
| `--no-show` | | Upload without switching display |
| `--mdns-only` | | Discover via mDNS only |
| `--ssdp-only` | | Discover via SSDP only |
| `-v` | | Verbose debug logging |

### Wallpaper options

| Flag | Default | Description |
|------|---------|-------------|
| `--source` | `wallhaven` | `wallhaven`, `unsplash`, or `pixabay` |
| `--api-key` | | Override API key for the selected source |
| `--id` | | Use a specific wallpaper/photo ID |
| `--sort` | `random` | Wallhaven: `random`, `toplist`, `date_added`, `views`, `favorites`; Unsplash: `random` or `search`; Pixabay: `popular` (default) or `latest` |
| `--save` | | Save the downloaded image locally |
| `--download-only` | | Download only; do not upload to the TV |
| `--no-replace` | | Keep previous wallpaper uploads (default replaces the last one) |

| Source | API key env var | Notes |
|--------|-----------------|-------|
| [Wallhaven](https://wallhaven.cc/help/api) | `$WALLHAVEN_API_KEY` | Optional for SFW; digital art + photos |
| [Unsplash](https://unsplash.com/documentation) | `$UNSPLASH_ACCESS_KEY` | Best nature photography; cropped to 3840×2160 |
| [Pixabay](https://pixabay.com/api/docs/) | `$PIXABAY_API_KEY` | Nature category; min 3840×2160 filter |

All sources target 4K landscape images suitable for Frame TVs. Unsplash and Pixabay default to a `nature` query when none is given. Pixabay may return `imageURL` (full resolution) when your account has full API access; otherwise it falls back to the best available CDN size.

## Environment

| Variable | Used by | Description |
|----------|---------|-------------|
| `FRAME_IT_HOST` | all commands | TV IP (overrides saved config; `--host` takes precedence) |
| `WALLHAVEN_API_KEY` | `wallpaper --source wallhaven` | Optional for SFW content |
| `UNSPLASH_ACCESS_KEY` | `wallpaper --source unsplash` | Required for Unsplash |
| `PIXABAY_API_KEY` | `wallpaper --source pixabay` | Required for Pixabay |

## Configuration

frame-it stores state in `--token-dir` (default `~/.frame-it`):

| File | Purpose |
|------|---------|
| `config.json` | Saved TV IP (`host`) and last wallpaper content ID (`last_wallpaper_id`) |
| `tv_<IP>.txt` | Authorization token for that TV (dots in the IP become underscores) |

Run `pair` once per TV. After pairing, `upload`, `list`, and other commands reuse the saved token.

## Discovery

frame-it finds TVs by scanning the LAN:

- **mDNS** — `_samsungmsf._tcp` (Smart View) and `_samsungctl._tcp` (remote control)
- **SSDP** — UPnP broadcasts (often more reliable on Samsung TVs)

Samsung TVs do not always answer mDNS queries reliably, so SSDP is enabled by default alongside mDNS. If multicast discovery finds nothing, frame-it also probes port 8001 on the local subnet. Use `frame-it discover` to see what was found before pairing.

After the first successful connection, the TV IP is stored in `~/.frame-it/config.json` so later commands skip discovery. Override with `--host` or `$FRAME_IT_HOST`.

## Prior art

This project builds on community reverse engineering:

| Project | Language | Notes |
|---------|----------|-------|
| [xchwarze/samsung-tv-ws-api](https://github.com/xchwarze/samsung-tv-ws-api) | Python | Canonical library + CLI (`samsungtv art-upload`) |
| [NickWaterton/samsung-tv-ws-api](https://github.com/NickWaterton/samsung-tv-ws-api) | Python | Fork with fixes for newer models |
| [MikeO7/frame-tv-art-manager](https://github.com/MikeO7/frame-tv-art-manager) | Go | Full art manager; Go client informed this code |
| [foxxyz/samsung-frame-connect](https://github.com/foxxyz/samsung-frame-connect) | Node.js | Selective API for 2024 models |
| [kohlerryan/samsung-tv-art-uploader](https://github.com/kohlerryan/samsung-tv-art-uploader) | Python/Docker | Rotation + Home Assistant MQTT |
| [ow/samsung-frame-art](https://github.com/ow/samsung-frame-art) | Python | Simple folder → TV script |

Samsung does not document this API. Behavior varies by model year and firmware.

## Troubleshooting

- **Authorization prompt every time** — Run `pair` and accept the prompt; token is saved in `--token-dir`.
- **TV storage filling up** — Each `upload` adds images permanently. `wallpaper` replaces the previous upload by default (tracked in `config.json`). Use `--no-replace` to keep a collection, or `frame-it list` / `frame-it delete` to clean up manually.
- **Connection refused** — TV must be on and on the same subnet (WebSockets do not cross VLANs).
- **Upload fails on older TVs** — API 0.97 (2018–2019) uses a different binary upload path; frame-it detects this automatically.
- **Images look wrong** — Resize to 3840×2160 before upload; the TV expects 4K landscape frames.
- **No wallpapers matched** — Try a different query, source, or `--id`.
- **Unsplash/Pixabay API key** — Set `$UNSPLASH_ACCESS_KEY` or `$PIXABAY_API_KEY`, or pass `--api-key`.

## License

Apache 2.0 — see [LICENSE](LICENSE).
