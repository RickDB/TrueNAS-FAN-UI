# Only Fans - TrueNAS Edition

Control your fans. Keep your disks cool.


A small Go web application for TrueNAS SCALE that monitors Linux `hwmon` fan RPM/temperatures and controls writable PWM channels through sysfs. No IPMI/BMC is used.





## Version 0.2.3

- Fixes IT87 failing after a clean TrueNAS reboot with `Unknown symbol vid_from_reg` / `vid_which_vrm`.
- The `it87-loader` container now automatically loads the host kernel `hwmon-vid` dependency before inserting `it87.ko`.
- No host-side `modprobe hwmon-vid` command is required after reboot.
- The loader health check now requires both the `it87` module and an actual `it86*`/`it87*` hwmon device.
- The main UI now waits for the IT87 loader to become healthy before starting.

## Version 0.2.2

- Adds AMD Ryzen Threadripper 1920X-aware temperature handling.
- Detects the host CPU model from `/proc/cpuinfo`.
- Treats `k10temp` `Tctl` correctly as a control temperature with the 1920X's +27°C offset.
- Uses kernel-provided `Tdie` when present. If `Tdie` is not exported, the UI derives `Tdie = Tctl - 27°C` and labels it as corrected.
- Raw `Tctl` no longer wins the "Highest temperature" tile or triggers a false high-temperature border.
- `Tccd*` remains a separate CCD reading and uses an 85°C CPU warning threshold.
- HDD `drivetemp` cards warn at 50°C and NVMe at 70°C.
- Includes all v0.2.1 fan ordering and v0.2.0 slider-state fixes.

## Version 0.2.1

- Controllable PWM fans are now listed first.
- Monitor-only/non-controllable fans are automatically moved to the end of the fan array.
- Existing ordering within each group remains stable by chip name and channel number.

## Version 0.2.0

- Fixes custom PWM sliders snapping back during the 2-second telemetry refresh.
- Unsaved slider edits now remain local until **Apply & save** is pressed.
- Live PWM/RPM telemetry continues updating independently.
- Includes Only Fans - TrueNAS Edition branding, IT87 runtime loader, per-fan saved profiles and per-fan startup restore.

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

## Per-fan custom profiles

Each writable fan can save its own PWM percentage. Enable **Restore this custom profile on app startup** for any fan that should override the global startup profile.

Startup order:

1. Apply the last saved global profile, if one exists.
2. Apply opted-in per-fan custom profiles as overrides.

This lets most fans follow a global profile while, for example, HDD intake fans keep a higher fixed airflow.

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

## GitHub Container Registry / GHCR

The repository includes `.github/workflows/docker-publish.yml`. Every push to `main` builds and publishes a multi-architecture image for `linux/amd64` and `linux/arm64`.

The normal TrueNAS deployment image is:

```text
ghcr.io/rickdb/truenas-fan-ui:latest
```

A Git tag such as `v0.1.0` also produces versioned tags such as `0.1.0` and `0.1`.

The included production `docker-compose.yml` pulls the GHCR image directly and intentionally contains no `build:` entry.

After pushing the initial repository contents, check **GitHub → Actions → Build and publish Docker image**. Once that job completes successfully, deploy or refresh TrueNAS with:

```bash
docker compose pull
docker compose up -d
```

If GHCR returns `permission_denied: write_package`, open **Repository Settings → Actions → General → Workflow permissions** and enable **Read and write permissions** for `GITHUB_TOKEN`.

## Integrated IT87 runtime driver loader

Some desktop/workstation motherboards do not expose their fan headers through the stock TrueNAS kernel. The Gigabyte **X399 DESIGNARE EX** is one such case: `sensors-detect` identifies an **ITE IT8686E** at `0xa40` and an **ITE IT8792E** at `0xa60`, while the stock TrueNAS hwmon tree may only expose unrelated devices such as `i915`.

The production Compose stack therefore includes an optional-but-enabled `it87-loader` helper container. It uses the maintained `frankcrawford/it87` driver source baked into its GHCR image, then at container startup:

1. reads the running TrueNAS kernel version with `uname -r`;
2. verifies the matching host headers at `/lib/modules/<kernel>/build`;
3. compiles `it87.ko` **inside the container**, not on the TrueNAS host;
4. caches the compiled module under `/mnt/nvme-apps/truenas-fan-ui/it87-cache`;
5. loads it into the shared host kernel with `ignore_resource_conflict=1`;
6. stays alive so Docker's restart policy causes it to load the module again after a TrueNAS reboot.

The helper image is published as:

```text
ghcr.io/rickdb/truenas-fan-ui-it87-loader:latest
```

The normal UI remains:

```text
ghcr.io/rickdb/truenas-fan-ui:latest
```

The UI container waits up to 90 seconds for `it87` before starting. This prevents the saved startup fan profile from being applied before the motherboard PWM channels exist. If the driver fails to build/load, the timeout expires and the UI still starts for diagnostics.

### Required TrueNAS host paths

The loader expects the host to provide matching kernel headers, for example:

```text
/lib/modules/6.12.95-production+truenas/build -> /usr/src/linux-headers-truenas-production-amd64
```

The Compose mounts `/lib/modules` and `/usr/src` read-only. No compiler, `make`, DKMS, or development packages are installed on the TrueNAS host.

### Check the loader

```bash
docker compose logs -f it87-loader
```

Then verify the host sees the ITE controller(s):

```bash
for h in /sys/class/hwmon/hwmon*; do
  echo "===== $h / $(cat "$h/name" 2>/dev/null) ====="
  ls -1 "$h" | grep -E '^(fan|pwm)'
done
```

Once `it86*` / `it87*` devices appear, Only Fans - TrueNAS Edition discovers the new `fanN_input` and `pwmN` channels automatically.

### Safety

Loading an out-of-tree kernel module is inherently more invasive than ordinary container workloads. The helper therefore requires `privileged: true`. `ignore_resource_conflict=1` is enabled for this Gigabyte/ITE use case because firmware resource reservations can otherwise block the driver. If the driver behaves incorrectly on different hardware, stop the stack and unload it with `sudo modprobe -r it87` (provided nothing is using it).