#!/bin/sh
set -eu

log() {
    printf '[it87-loader] %s\n' "$*"
}

KERNEL="$(uname -r)"
BUILD_DIR="/lib/modules/${KERNEL}/build"
SOURCE_DIR="/opt/it87"
SOURCE_REV="$(cat "${SOURCE_DIR}/.source-rev" 2>/dev/null || printf unknown)"
CACHE_ROOT="${IT87_CACHE_DIR:-/module-cache}"
CACHE_DIR="${CACHE_ROOT}/${KERNEL}/${SOURCE_REV}"
CACHE_MODULE="${CACHE_DIR}/it87.ko"
IGNORE_CONFLICT="${IT87_IGNORE_RESOURCE_CONFLICT:-1}"

log "host kernel: ${KERNEL}"
log "it87 source revision: ${SOURCE_REV}"

if [ ! -e "${BUILD_DIR}/Makefile" ]; then
    log "ERROR: matching kernel headers are unavailable at ${BUILD_DIR}"
    log "TrueNAS must expose /lib/modules/${KERNEL}/build and /usr/src to this container."
    exit 1
fi

ite_hwmon_present() {
    for hw in /sys/class/hwmon/hwmon*; do
        [ -r "${hw}/name" ] || continue
        name="$(cat "${hw}/name" 2>/dev/null || true)"
        case "${name}" in
            it86*|it87*) return 0 ;;
        esac
    done
    return 1
}

# If an it87 module is already active and has created an ITE hwmon device,
# leave it alone rather than unloading a working controller underneath the UI.
if grep -q '^it87 ' /proc/modules 2>/dev/null; then
    if ite_hwmon_present; then
        log "it87 is already loaded and exposing an ITE hwmon device; keeping it."
        exec tail -f /dev/null
    fi

    log "it87 is loaded but no ITE hwmon device is visible; attempting to replace it."
    modprobe -r it87 2>/dev/null || rmmod it87 2>/dev/null || {
        log "ERROR: existing it87 module could not be unloaded."
        exit 1
    }
fi

mkdir -p "${CACHE_DIR}"

if [ -s "${CACHE_MODULE}" ]; then
    log "using cached module ${CACHE_MODULE}"
else
    log "building it87 against ${BUILD_DIR}"
    cd "${SOURCE_DIR}"
    make clean
    make TARGET="${KERNEL}"

    if [ ! -s "${SOURCE_DIR}/it87.ko" ]; then
        log "ERROR: build completed without producing it87.ko"
        exit 1
    fi

    cp "${SOURCE_DIR}/it87.ko" "${CACHE_MODULE}"
    log "cached module at ${CACHE_MODULE}"
fi

log "module vermagic: $(modinfo -F vermagic "${CACHE_MODULE}" 2>/dev/null || printf unknown)"

log "loading it87 (ignore_resource_conflict=${IGNORE_CONFLICT})"
if ! insmod "${CACHE_MODULE}" "ignore_resource_conflict=${IGNORE_CONFLICT}"; then
    log "ERROR: failed to load it87. Recent kernel messages follow:"
    dmesg 2>/dev/null | tail -40 || true
    exit 1
fi

sleep 1

log "detected ITE hwmon devices:"
found=0
for hw in /sys/class/hwmon/hwmon*; do
    [ -r "${hw}/name" ] || continue
    name="$(cat "${hw}/name" 2>/dev/null || true)"
    case "${name}" in
        it86*|it87*)
            found=1
            printf '  %s: %s\n' "${hw}" "${name}"
            find "${hw}" -maxdepth 1 -type f \( -name 'fan*_input' -o -name 'pwm[0-9]*' \) -printf '    %f\n' 2>/dev/null | sort || true
            ;;
    esac
done

if [ "${found}" -eq 0 ]; then
    log "WARNING: module loaded, but no it86*/it87* hwmon device appeared."
fi

log "ready; keeping helper alive so it is re-run after host/container restarts."
exec tail -f /dev/null
