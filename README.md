# TrueNAS Fan Commander

A small Go web application for TrueNAS SCALE that monitors Linux `hwmon` fan RPM/temperatures and controls writable PWM channels through sysfs. No IPMI/BMC is used.

## Features

- Auto-discovers `/sys/class/hwmon/hwmon*`
- Live fan RPM, PWM percentage/mode and temperature telemetry
- Quiet / Balanced / Performance / Full global profiles
- Per-fan manual PWM slider
- Persistent custom fan names / name overrides
- Automatically re-applies the last user-selected fan profile on app/container startup
- Stable fan IDs based on hwmon chip + resolved device identity + channel index, so names are not tied to `hwmon0`, `hwmon1`, etc.
- Remembers each fan's PWM mode before the application first changes it and can restore that mode during the same container run
- Safety floor prevents very low manual PWM values by default
- Single Go binary; frontend is embedded into it
- No JavaScript framework, database, or external service

## Important safety note

Linux motherboard fan control varies by hardware and driver. This application only writes to PWM files that the kernel actually exposes, but you should still verify airflow and temperatures after changing a profile.

The default minimum manual setting is 30%. Profiles are:

| Profile | PWM |
| --- | ---: |
| Quiet | 40% |
| Balanced | 55% |
| Performance | 75% |
| Full | 100% |

The values live in `/config/config.json` and can be edited while the container is stopped.

## Install on TrueNAS SCALE

Create a directory containing this project, then edit the persistent config path in `docker-compose.yml` if needed:

```yaml
- /mnt/nvme-apps/truenas-fan-ui:/config
```

Build and start:

```bash
docker compose up -d --build
```

Open:

```text
http://TRUENAS-IP:8188
```

Watch logs:

```bash
docker compose logs -f truenas-fan-ui
```

Stop:

```bash
docker compose down
```

## Why privileged mode?

The container binds the host `/sys` tree as `/host/sys`. Reading RPM and temperatures is harmless, but changing motherboard PWM channels requires writing to host sysfs. Most container runtimes protect sysfs unless the container is privileged.

This container therefore uses:

```yaml
privileged: true
volumes:
  - /sys:/host/sys:rw
```

Do **not** expose this web UI directly to the public Internet. Anyone who can access it can change fan speeds.

## Fan naming

Each discovered fan receives a stable ID generated from:

1. hwmon chip name
2. resolved kernel device path
3. fan/PWM channel number

Aliases are saved to:

```text
/config/config.json
```

Example:

```json
{
  "aliases": {
    "8d0f7d219bc440fa": "Front HDD Intake",
    "8b4fd25ea4b1c318": "Rear Exhaust"
  },
  "profiles": {
    "quiet": 40,
    "balanced": 55,
    "performance": 75,
    "full": 100
  },
  "last_profile": "balanced",
  "min_percent": 30
}
```

Clear a custom name in the UI and save it to return to the hardware/driver label.

## Startup profile restore

When you select a profile in the web UI, its name is saved as `last_profile` in `/config/config.json`. On the next container or TrueNAS restart, the app discovers the available PWM channels and automatically re-applies that saved profile.

If no profile has ever been selected, the app does **not** change fan control during startup. If the saved profile no longer exists or cannot be applied, the error is logged and the existing hardware fan state is left alone.

## API

Useful endpoints:

```text
GET  /api/status
POST /api/profile
POST /api/restore
POST /api/fans/<id>/name
POST /api/fans/<id>/pwm
POST /api/fans/<id>/restore
GET  /healthz
```

Profile example:

```bash
curl -X POST http://127.0.0.1:8188/api/profile \
  -H 'Content-Type: application/json' \
  -d '{"profile":"balanced"}'
```

Rename example:

```bash
curl -X POST http://127.0.0.1:8188/api/fans/FAN_ID/name \
  -H 'Content-Type: application/json' \
  -d '{"name":"Front HDD Intake"}'
```
