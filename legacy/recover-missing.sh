#!/usr/bin/env bash
#
# recover-missing.sh — Download missing Google Photos files using cookie-authenticated curl
#
# Usage:
#   ./recover-missing.sh [OPTIONS]
#
# Options:
#   --cookies FILE     Path to Netscape cookie jar (default: ./google-cookies.txt)
#   --input FILE       Path to missing-files.json (default: ./processed/missing-files.json)
#   --output DIR       Output directory (default: ./recovered)
#   --concurrency N    Parallel downloads (default: 3)
#   --delay SECS       Delay between batches in seconds (default: 1)
#   --retry N          Max retries per file (default: 3)
#   --start-from N     Resume from entry N (1-indexed, default: 1)
#   --dry-run          Show what would be downloaded without downloading
#   --refresh-cookies  Re-decrypt cookies from Chrome before starting
#   --help             Show this help
#
set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────────────
COOKIE_JAR="./google-cookies.txt"
INPUT_JSON="./processed/missing-files.json"
OUTPUT_DIR="./recovered"
CONCURRENCY=3
DELAY=1
MAX_RETRY=3
START_FROM=1
DRY_RUN=false
REFRESH_COOKIES=false
COOKIE_REFRESH_INTERVAL=180  # seconds — refresh cookies from Chrome every 3 minutes
LAST_COOKIE_REFRESH=0        # epoch timestamp of last cookie refresh

# ── Colors ────────────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

# ── Parse arguments ──────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --cookies)    COOKIE_JAR="$2"; shift 2 ;;
        --input)      INPUT_JSON="$2"; shift 2 ;;
        --output)     OUTPUT_DIR="$2"; shift 2 ;;
        --concurrency) CONCURRENCY="$2"; shift 2 ;;
        --delay)      DELAY="$2"; shift 2 ;;
        --retry)      MAX_RETRY="$2"; shift 2 ;;
        --start-from) START_FROM="$2"; shift 2 ;;
        --dry-run)    DRY_RUN=true; shift ;;
        --refresh-cookies) REFRESH_COOKIES=true; shift ;;
        --help)
            sed -n '2,/^$/s/^#//p' "$0"
            exit 0
            ;;
        *) echo -e "${RED}Unknown option: $1${NC}"; exit 1 ;;
    esac
done

# ── Logging ──────────────────────────────────────────────────────────────────
LOG_FILE="${OUTPUT_DIR}/recovery.log"
FAILED_FILE="${OUTPUT_DIR}/failed.json"
PROGRESS_FILE="${OUTPUT_DIR}/.progress"

log() {
    local ts
    ts=$(date '+%Y-%m-%d %H:%M:%S')
    echo "[$ts] $*" >> "$LOG_FILE"
}

info()  { echo -e "${CYAN}[INFO]${NC} $*";  log "INFO  $*"; }
ok()    { echo -e "${GREEN}[OK]${NC} $*";    log "OK    $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; log "WARN  $*"; }
fail()  { echo -e "${RED}[FAIL]${NC} $*";    log "FAIL  $*"; }
debug() { log "DEBUG $*"; }

# ── Preflight checks ────────────────────────────────────────────────────────
check_deps() {
    local missing=()
    for cmd in curl jq python3; do
        command -v "$cmd" &>/dev/null || missing+=("$cmd")
    done
    if [[ ${#missing[@]} -gt 0 ]]; then
        fail "Missing dependencies: ${missing[*]}"
        exit 1
    fi
}

check_cookies() {
    info "Verifying cookie authentication..."
    local http_code
    http_code=$(curl -s -b "$COOKIE_JAR" -o /dev/null -w "%{http_code}" \
        "https://photos.google.com/" 2>/dev/null || echo "000")
    if [[ "$http_code" != "200" ]]; then
        fail "Cookie authentication failed (HTTP $http_code)"
        if [[ -f "./decrypt-cookies.mjs" ]]; then
            warn "Try: node decrypt-cookies.mjs  OR  --refresh-cookies"
        fi
        exit 1
    fi
    ok "Cookies valid (HTTP 200)"
}

# ── Cookie refresh ───────────────────────────────────────────────────────────
refresh_cookies() {
    info "Refreshing cookies from Chrome..."
    if [[ ! -f "./decrypt-cookies.mjs" ]]; then
        fail "decrypt-cookies.mjs not found"
        exit 1
    fi
    node decrypt-cookies.mjs >/dev/null 2>&1
    LAST_COOKIE_REFRESH=$(date +%s)
    ok "Cookies refreshed"
}

# Auto-refresh cookies if they're older than COOKIE_REFRESH_INTERVAL
maybe_refresh_cookies() {
    local now
    now=$(date +%s)
    local age=$(( now - LAST_COOKIE_REFRESH ))
    if [[ "$age" -ge "$COOKIE_REFRESH_INTERVAL" ]]; then
        debug "Cookies are ${age}s old (limit: ${COOKIE_REFRESH_INTERVAL}s), refreshing..."
        refresh_cookies
    fi
}

# ── Video extension detection ────────────────────────────────────────────────
VIDEO_EXTS="mp4|mov|avi|3gp|mkv|wmv|flv|webm|m4v|mpg|mpeg"

is_video() {
    local filename="$1"
    local ext="${filename##*.}"
    ext="${ext,,}"  # lowercase
    [[ "$ext" =~ ^($VIDEO_EXTS)$ ]]
}

# ── Extract fife URL from Google Photos page ─────────────────────────────────
# Uses the data-media-key → data-url mapping in the page HTML
extract_fife_url() {
    local photo_id="$1"
    local html_file="$2"

    # Method 1: data-media-key="<id>" data-url="<fife_url>"
    local fife_url
    fife_url=$(grep -oP "data-media-key=\"${photo_id}\"[^>]*data-url=\"\K[^\"]+" "$html_file" 2>/dev/null | head -1)

    if [[ -n "$fife_url" ]]; then
        echo "$fife_url"
        return 0
    fi

    # Method 2: Look for data-url near the photo ID (attributes might be in different order)
    fife_url=$(grep -oP "data-url=\"\K[^\"]+(?=\"[^>]*data-media-key=\"${photo_id}\")" "$html_file" 2>/dev/null | head -1)

    if [[ -n "$fife_url" ]]; then
        echo "$fife_url"
        return 0
    fi

    # Method 3: Fallback — first unique fife base URL (no =s suffix) on the page
    fife_url=$(grep -oP 'https://photos\.fife\.usercontent\.google\.com/pw/[A-Za-z0-9_-]+(?=["\s=])' "$html_file" 2>/dev/null \
        | sort | uniq -c | sort -rn | head -1 | awk '{print $2}')

    if [[ -n "$fife_url" ]]; then
        echo "$fife_url"
        return 0
    fi

    return 1
}

# ── Download a single file ───────────────────────────────────────────────────
download_one() {
    local idx="$1"
    local title="$2"
    local url="$3"
    local photo_id="$4"
    local total="$5"

    local dest="${OUTPUT_DIR}/${title}"
    local tmp_html="${OUTPUT_DIR}/.tmp_page_${BASHPID}.html"
    local tmp_dl="${OUTPUT_DIR}/.tmp_dl_${BASHPID}"

    # Skip if already downloaded
    if [[ -f "$dest" ]] && [[ -s "$dest" ]]; then
        debug "[$idx/$total] Already exists: $title"
        echo "SKIP"
        return 0
    fi

    # Handle filename conflicts within recovered dir
    if [[ -f "$dest" ]]; then
        local base="${title%.*}"
        local ext="${title##*.}"
        local hash
        hash=$(echo -n "${photo_id}" | md5sum | cut -c1-6)
        dest="${OUTPUT_DIR}/${base}_${hash}.${ext}"
    fi

    debug "[$idx/$total] Fetching page for: $title"

    # Step 1: Fetch the Google Photos page
    local http_code
    http_code=$(curl -s -b "$COOKIE_JAR" -L \
        -o "$tmp_html" -w "%{http_code}" \
        "$url" 2>/dev/null || echo "000")

    if [[ "$http_code" != "200" ]]; then
        rm -f "$tmp_html"
        debug "[$idx/$total] Page fetch failed (HTTP $http_code): $title"
        echo "FAIL_PAGE:$http_code"
        return 1
    fi

    # Step 2: Extract fife URL
    local fife_url
    fife_url=$(extract_fife_url "$photo_id" "$tmp_html")
    rm -f "$tmp_html"

    if [[ -z "$fife_url" ]]; then
        debug "[$idx/$total] No fife URL found: $title"
        echo "FAIL_EXTRACT"
        return 1
    fi

    # Step 3: Download the actual file
    local dl_suffix="=d"
    if is_video "$title"; then
        dl_suffix="=dv"
    fi

    http_code=$(curl -s -b "$COOKIE_JAR" -L \
        -o "$tmp_dl" -w "%{http_code}" \
        "${fife_url}${dl_suffix}" 2>/dev/null || echo "000")

    if [[ "$http_code" != "200" ]]; then
        rm -f "$tmp_dl"
        debug "[$idx/$total] Download failed (HTTP $http_code): $title"
        echo "FAIL_DL:$http_code"
        return 1
    fi

    # Step 4: Validate the downloaded file
    local file_size
    file_size=$(stat -c%s "$tmp_dl" 2>/dev/null || echo "0")

    if [[ "$file_size" -lt 100 ]]; then
        rm -f "$tmp_dl"
        debug "[$idx/$total] File too small ($file_size bytes): $title"
        echo "FAIL_SIZE:$file_size"
        return 1
    fi

    # Check if we got HTML instead of media (auth failure)
    local file_type
    file_type=$(file -b --mime-type "$tmp_dl" 2>/dev/null || echo "unknown")
    if [[ "$file_type" == "text/html" ]]; then
        rm -f "$tmp_dl"
        debug "[$idx/$total] Got HTML instead of media: $title"
        echo "FAIL_HTML"
        return 1
    fi

    # Move to final destination
    mv "$tmp_dl" "$dest"
    debug "[$idx/$total] Downloaded: $title ($file_size bytes, $file_type)"
    echo "OK:$file_size:$file_type"
    return 0
}

# ── Progress display ─────────────────────────────────────────────────────────
show_progress() {
    local current="$1" total="$2" ok="$3" skip="$4" fail="$5" start_time="$6"

    local elapsed=$(( $(date +%s) - start_time ))
    local pct=0
    [[ "$total" -gt 0 ]] && pct=$(( current * 100 / total ))

    local rate="0"
    local eta="--:--"
    if [[ "$elapsed" -gt 0 ]] && [[ "$current" -gt 0 ]]; then
        rate=$(python3 -c "print(f'{$current/$elapsed:.1f}')" 2>/dev/null || echo "?")
        local remaining=$(( total - current ))
        local eta_secs
        eta_secs=$(python3 -c "print(int($remaining / ($current/$elapsed)))" 2>/dev/null || echo "0")
        local eta_m=$(( eta_secs / 60 ))
        local eta_s=$(( eta_secs % 60 ))
        eta=$(printf "%02d:%02d" "$eta_m" "$eta_s")
    fi

    # Progress bar
    local bar_len=30
    local filled=$(( pct * bar_len / 100 ))
    local empty=$(( bar_len - filled ))
    local bar
    bar=$(printf "%${filled}s" | tr ' ' '█')$(printf "%${empty}s" | tr ' ' '░')

    printf "\r${BOLD}[%s]${NC} %3d%% │ ${GREEN}%d ok${NC} ${YELLOW}%d skip${NC} ${RED}%d fail${NC} │ %s/s │ ETA %s   " \
        "$bar" "$pct" "$ok" "$skip" "$fail" "$rate" "$eta"
}

# ── Main ─────────────────────────────────────────────────────────────────────
main() {
    check_deps

    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║     Google Photos Missing File Recovery          ║${NC}"
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"
    echo

    # Validate inputs
    if [[ ! -f "$INPUT_JSON" ]]; then
        fail "Input file not found: $INPUT_JSON"
        exit 1
    fi
    if [[ ! -f "$COOKIE_JAR" ]]; then
        fail "Cookie jar not found: $COOKIE_JAR"
        exit 1
    fi

    # Create output directory
    mkdir -p "$OUTPUT_DIR"

    # Initialize log
    log "=== Recovery session started ==="
    log "Input:       $INPUT_JSON"
    log "Output:      $OUTPUT_DIR"
    log "Cookies:     $COOKIE_JAR"
    log "Concurrency: $CONCURRENCY"
    log "Start from:  $START_FROM"

    # Refresh cookies if requested
    if [[ "$REFRESH_COOKIES" == true ]]; then
        refresh_cookies
    fi

    # Always do an initial cookie refresh to ensure freshness
    if [[ "$LAST_COOKIE_REFRESH" -eq 0 ]]; then
        refresh_cookies
    fi

    # Verify authentication
    check_cookies

    # Parse the input JSON into a work list
    info "Loading missing files manifest..."
    local total
    total=$(jq 'length' "$INPUT_JSON")
    info "Total entries: $total (starting from #$START_FROM)"

    if [[ "$DRY_RUN" == true ]]; then
        echo -e "\n${YELLOW}=== DRY RUN ===${NC}\n"
    fi

    # Pre-extract all entries to a temp file for efficient processing
    local work_file="${OUTPUT_DIR}/.work_list.jsonl"
    jq -c '.[] | {
        title: .title,
        url: .url,
        photo_id: (.url | split("/") | last),
        photoTakenDate: .photoTakenDate,
        sourceFolder: .sourceFolder
    }' "$INPUT_JSON" > "$work_file"

    # Counters
    local count_ok=0 count_skip=0 count_fail=0 count_total=0
    local start_time
    start_time=$(date +%s)

    # Failed entries collector
    echo "[" > "$FAILED_FILE.tmp"
    local first_fail=true

    # Process entries
    local line_num=0
    while IFS= read -r entry; do
        line_num=$((line_num + 1))

        # Skip entries before start_from
        if [[ "$line_num" -lt "$START_FROM" ]]; then
            continue
        fi

        count_total=$((count_total + 1))
        local title url photo_id
        title=$(echo "$entry" | jq -r '.title')
        url=$(echo "$entry" | jq -r '.url')
        photo_id=$(echo "$entry" | jq -r '.photo_id')

        # Show progress
        show_progress "$count_total" "$((total - START_FROM + 1))" "$count_ok" "$count_skip" "$count_fail" "$start_time"

        if [[ "$DRY_RUN" == true ]]; then
            local dest="${OUTPUT_DIR}/${title}"
            if [[ -f "$dest" ]] && [[ -s "$dest" ]]; then
                echo -e "\n  ${YELLOW}SKIP${NC} $title (already exists)"
                count_skip=$((count_skip + 1))
            else
                local type_tag="IMG"
                is_video "$title" && type_tag="VID"
                echo -e "\n  ${BLUE}WOULD${NC} [$type_tag] $title"
                count_ok=$((count_ok + 1))
            fi
            continue
        fi

        # Auto-refresh cookies before download if interval expired
        maybe_refresh_cookies

        # Attempt download with retries
        local attempt=0 result=""
        while [[ $attempt -lt $MAX_RETRY ]]; do
            attempt=$((attempt + 1))

            result=$(download_one "$line_num" "$title" "$url" "$photo_id" "$total" 2>/dev/null || true)

            case "$result" in
                OK:*)
                    count_ok=$((count_ok + 1))
                    local size type_info
                    size=$(echo "$result" | cut -d: -f2)
                    type_info=$(echo "$result" | cut -d: -f3-)
                    # Human-readable size
                    local hr_size
                    hr_size=$(python3 -c "
s=$size
for u in ['B','KB','MB','GB']:
    if s < 1024: print(f'{s:.1f}{u}'); break
    s /= 1024
" 2>/dev/null || echo "${size}B")
                    debug "[$line_num/$total] OK: $title ($hr_size)"
                    break
                    ;;
                SKIP)
                    count_skip=$((count_skip + 1))
                    break
                    ;;
                FAIL_DL:403)
                    # 403 on fife download — likely stale OSID cookie
                    warn "[$line_num] 403 on download — refreshing cookies..."
                    refresh_cookies
                    if [[ $attempt -lt $MAX_RETRY ]]; then
                        sleep 1
                        continue
                    fi
                    count_fail=$((count_fail + 1))
                    if [[ "$first_fail" == true ]]; then first_fail=false; else echo "," >> "$FAILED_FILE.tmp"; fi
                    echo "$entry" | jq -c '. + {error: "'"$result"'"}' >> "$FAILED_FILE.tmp"
                    break
                    ;;
                FAIL_HTML)
                    # Authentication likely expired
                    warn "Got HTML response — cookies may have expired"
                    refresh_cookies
                    if [[ $attempt -lt $MAX_RETRY ]]; then
                        continue
                    fi
                    count_fail=$((count_fail + 1))
                    if [[ "$first_fail" == true ]]; then first_fail=false; else echo "," >> "$FAILED_FILE.tmp"; fi
                    echo "$entry" | jq -c '. + {error: "'"$result"'"}' >> "$FAILED_FILE.tmp"
                    break
                    ;;
                FAIL_PAGE:*|FAIL_EXTRACT|FAIL_DL:*|FAIL_SIZE:*)
                    if [[ $attempt -lt $MAX_RETRY ]]; then
                        debug "[$line_num/$total] Retry $attempt/$MAX_RETRY: $title ($result)"
                        sleep $((attempt * 2))
                        continue
                    fi
                    count_fail=$((count_fail + 1))
                    if [[ "$first_fail" == true ]]; then first_fail=false; else echo "," >> "$FAILED_FILE.tmp"; fi
                    echo "$entry" | jq -c '. + {error: "'"$result"'"}' >> "$FAILED_FILE.tmp"
                    break
                    ;;
                *)
                    count_fail=$((count_fail + 1))
                    if [[ "$first_fail" == true ]]; then first_fail=false; else echo "," >> "$FAILED_FILE.tmp"; fi
                    echo "$entry" | jq -c '. + {error: "unknown"}' >> "$FAILED_FILE.tmp"
                    break
                    ;;
            esac
        done

        # Save progress checkpoint
        echo "$line_num" > "$PROGRESS_FILE"

        # Rate limiting: small delay between requests
        if [[ "$DRY_RUN" != true ]] && [[ "$result" != SKIP ]]; then
            sleep "$DELAY"
        fi

    done < "$work_file"

    # Finalize failed file
    echo "]" >> "$FAILED_FILE.tmp"
    # Make it valid JSON (remove trailing comma issue)
    python3 -c "
import json, sys
with open('${FAILED_FILE}.tmp') as f:
    content = f.read()
try:
    data = json.loads(content)
    with open('${FAILED_FILE}', 'w') as f:
        json.dump(data, f, indent=2)
except:
    # If JSON is malformed, just move it
    with open('${FAILED_FILE}', 'w') as f:
        f.write(content)
" 2>/dev/null
    rm -f "${FAILED_FILE}.tmp"

    # Clean up
    rm -f "$work_file" "${OUTPUT_DIR}"/.tmp_page_* "${OUTPUT_DIR}"/.tmp_dl_*

    # Final report
    local elapsed=$(( $(date +%s) - start_time ))
    local elapsed_m=$(( elapsed / 60 ))
    local elapsed_s=$(( elapsed % 60 ))

    echo -e "\n"
    echo -e "${BOLD}╔══════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}║              Recovery Complete                   ║${NC}"
    echo -e "${BOLD}╠══════════════════════════════════════════════════╣${NC}"
    printf  "${BOLD}║${NC}  %-22s %24s ${BOLD}║${NC}\n" "Downloaded:" "${GREEN}${count_ok}${NC}"
    printf  "${BOLD}║${NC}  %-22s %24s ${BOLD}║${NC}\n" "Skipped (existing):" "${YELLOW}${count_skip}${NC}"
    printf  "${BOLD}║${NC}  %-22s %24s ${BOLD}║${NC}\n" "Failed:" "${RED}${count_fail}${NC}"
    printf  "${BOLD}║${NC}  %-22s %15s ${BOLD}║${NC}\n" "Time:" "$(printf '%02d:%02d' $elapsed_m $elapsed_s)"
    echo -e "${BOLD}╠══════════════════════════════════════════════════╣${NC}"
    if [[ -d "$OUTPUT_DIR" ]]; then
        local total_size
        total_size=$(du -sh "$OUTPUT_DIR" 2>/dev/null | cut -f1)
        printf  "${BOLD}║${NC}  %-22s %15s ${BOLD}║${NC}\n" "Output dir:" "$OUTPUT_DIR"
        printf  "${BOLD}║${NC}  %-22s %15s ${BOLD}║${NC}\n" "Total size:" "$total_size"
    fi
    if [[ "$count_fail" -gt 0 ]]; then
        printf  "${BOLD}║${NC}  %-22s %15s ${BOLD}║${NC}\n" "Failed list:" "$FAILED_FILE"
    fi
    echo -e "${BOLD}╚══════════════════════════════════════════════════╝${NC}"

    log "=== Recovery complete: ok=$count_ok skip=$count_skip fail=$count_fail time=${elapsed_m}m${elapsed_s}s ==="

    # Exit with error if all failed
    [[ "$count_fail" -lt "$total" ]]
}

main "$@"
