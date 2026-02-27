#!/usr/bin/env bash
#
# process-takeout.sh — Google Photos Takeout processor
#
# Processes a Google Photos Takeout export:
#   1. Matches JSON metadata to their corresponding media files
#   2. Resolves album membership and deduplicates by content hash
#   3. Copies/moves files to a clean output directory
#   4. Converts .MP (Motion Photo) files to .mp4
#   5. Applies EXIF metadata from JSON (date, GPS, description, people)
#   6. Falls back to filename-based date extraction when no JSON exists
#   7. Generates a report of missing/orphan files
#
# Usage: ./process-takeout.sh [OPTIONS]
#   --dry-run         Show what would be done without doing it
#   --move            Use mv instead of cp (default: cp)
#   --source DIR      Source directory (default: ./Takeout)
#   --dest DIR        Destination directory (default: ./processed)
#   --help            Show this help
#
set -euo pipefail

###############################################################################
# Globals & Defaults
###############################################################################
DRY_RUN=0
USE_MOVE=0
SOURCE_DIR="./Takeout"
DEST_DIR="./processed"
GOOGLE_FOTO_SUBDIR="Google Foto"   # Italian locale; auto-detected
LOG_FILE=""
WORK_DIR=""                         # Temp dir for intermediate files
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Counters
STAT_MEDIA_FOUND=0
STAT_JSON_MATCHED=0
STAT_JSON_ORPHAN=0
STAT_DUPLICATES_REMOVED=0
STAT_FILES_COPIED=0
STAT_MP_CONVERTED=0
STAT_EXIF_APPLIED=0
STAT_DATE_FROM_FILENAME=0
STAT_CONFLICTS_RESOLVED=0
STAT_ERRORS=0

###############################################################################
# Logging
###############################################################################
log() {
    local level="$1"; shift
    local msg="$*"
    local ts
    ts="$(date '+%Y-%m-%d %H:%M:%S')"
    local line="[$ts] [$level] $msg"
    echo "$line" >> "$LOG_FILE"
    case "$level" in
        ERROR)   echo -e "\033[31m$line\033[0m" >&2 ;;
        WARN)    echo -e "\033[33m$line\033[0m" ;;
        OK)      echo -e "\033[32m$line\033[0m" ;;
        DRY)     echo -e "\033[36m$line\033[0m" ;;
        INFO)    echo "$line" ;;
        DEBUG)   ;; # only to log file
    esac
}

# --- Progress with ETA ---
# Call progress_start "Label" <total> before the loop
# Call progress_tick inside the loop (increments automatically)
# Call progress_end after the loop
declare -g _PROG_LABEL="" _PROG_TOTAL=0 _PROG_CURRENT=0 _PROG_START=0 _PROG_LAST_DRAW=0

progress_start() {
    _PROG_LABEL="$1"
    _PROG_TOTAL="$2"
    _PROG_CURRENT=0
    _PROG_START=$(date +%s)
    _PROG_LAST_DRAW=0
    printf "\r  [  0%%] %s: 0/%d | ETA: --:--    " "$_PROG_LABEL" "$_PROG_TOTAL" >&2
}

progress_tick() {
    (( _PROG_CURRENT++ )) || true
    local now
    now=$(date +%s)
    # Throttle redraws to at most once per second (avoid overhead)
    if (( now == _PROG_LAST_DRAW && _PROG_CURRENT != _PROG_TOTAL )); then
        return
    fi
    _PROG_LAST_DRAW=$now

    local pct=0 elapsed eta_s eta_str=""
    if (( _PROG_TOTAL > 0 )); then
        pct=$(( _PROG_CURRENT * 100 / _PROG_TOTAL ))
    fi
    elapsed=$(( now - _PROG_START ))

    if (( _PROG_CURRENT > 0 && elapsed > 0 && _PROG_CURRENT < _PROG_TOTAL )); then
        local remaining=$(( _PROG_TOTAL - _PROG_CURRENT ))
        # items/sec so far
        eta_s=$(( remaining * elapsed / _PROG_CURRENT ))
        if (( eta_s >= 3600 )); then
            eta_str="$(printf '%dh%02dm%02ds' $((eta_s/3600)) $((eta_s%3600/60)) $((eta_s%60)))"
        elif (( eta_s >= 60 )); then
            eta_str="$(printf '%dm%02ds' $((eta_s/60)) $((eta_s%60)))"
        else
            eta_str="${eta_s}s"
        fi
    elif (( _PROG_CURRENT >= _PROG_TOTAL )); then
        eta_str="done"
    else
        eta_str="--:--"
    fi

    # Build a small bar (20 chars wide)
    local bar_width=20
    local filled=$(( pct * bar_width / 100 ))
    local empty=$(( bar_width - filled ))
    local bar
    bar="$(printf '%0.s#' $(seq 1 $filled 2>/dev/null))$(printf '%0.s-' $(seq 1 $empty 2>/dev/null))"

    printf "\r  [%s] %3d%% %s: %d/%d | ETA: %s    " \
        "$bar" "$pct" "$_PROG_LABEL" "$_PROG_CURRENT" "$_PROG_TOTAL" "$eta_str" >&2
}

progress_end() {
    local now elapsed elapsed_str
    now=$(date +%s)
    elapsed=$(( now - _PROG_START ))
    if (( elapsed >= 3600 )); then
        elapsed_str="$(printf '%dh%02dm%02ds' $((elapsed/3600)) $((elapsed%3600/60)) $((elapsed%60)))"
    elif (( elapsed >= 60 )); then
        elapsed_str="$(printf '%dm%02ds' $((elapsed/60)) $((elapsed%60)))"
    else
        elapsed_str="${elapsed}s"
    fi

    local bar
    bar="$(printf '%0.s#' $(seq 1 20))"
    printf "\r  [%s] 100%% %s: %d/%d | Done in %s    \n" \
        "$bar" "$_PROG_LABEL" "$_PROG_TOTAL" "$_PROG_TOTAL" "$elapsed_str" >&2
}

# Spinner for indeterminate operations (parallel batch jobs)
declare -g _SPIN_PID=""
spinner_start() {
    local label="$1"
    (
        local chars='|/-\'
        local i=0
        while true; do
            printf "\r  [%c] %s...    " "${chars:i%4:1}" "$label" >&2
            (( i++ )) || true
            sleep 0.2
        done
    ) &
    _SPIN_PID=$!
    disown "$_SPIN_PID" 2>/dev/null
}

spinner_stop() {
    if [[ -n "$_SPIN_PID" ]]; then
        kill "$_SPIN_PID" 2>/dev/null || true
        wait "$_SPIN_PID" 2>/dev/null || true
        _SPIN_PID=""
        printf "\r%80s\r" "" >&2   # clear the line
    fi
}

###############################################################################
# Argument Parsing
###############################################################################
parse_args() {
    while [[ $# -gt 0 ]]; do
        case "$1" in
            --dry-run)   DRY_RUN=1; shift ;;
            --move)      USE_MOVE=1; shift ;;
            --source)    SOURCE_DIR="$2"; shift 2 ;;
            --dest)      DEST_DIR="$2"; shift 2 ;;
            --help|-h)   usage; exit 0 ;;
            *)           echo "Unknown option: $1"; usage; exit 1 ;;
        esac
    done
}

usage() {
    cat <<'USAGE'
Usage: ./process-takeout.sh [OPTIONS]

Options:
  --dry-run         Show what would be done without doing it
  --move            Use mv instead of cp (default: cp)
  --source DIR      Source Takeout directory (default: ./Takeout)
  --dest DIR        Destination directory (default: ./processed)
  --help, -h        Show this help

Prerequisites:
  exiftool, jq, ffmpeg, md5sum
USAGE
}

###############################################################################
# Cleanup handler
###############################################################################
cleanup() {
    # Kill any running spinner subprocess
    if [[ -n "${_SPIN_PID:-}" ]]; then
        kill "$_SPIN_PID" 2>/dev/null || true
        wait "$_SPIN_PID" 2>/dev/null || true
        _SPIN_PID=""
        printf "\r%80s\r" "" >&2
    fi
    if [[ -n "$WORK_DIR" ]] && [[ -d "$WORK_DIR" ]]; then
        rm -rf "$WORK_DIR"
    fi
}
trap cleanup EXIT

###############################################################################
# Phase 0: Validation & Setup
###############################################################################
phase0_setup() {
    log INFO "=== Phase 0: Validation & Setup ==="

    # Check prerequisites
    local missing=()
    for cmd in exiftool jq ffmpeg md5sum bc; do
        if ! command -v "$cmd" &>/dev/null; then
            missing+=("$cmd")
        fi
    done
    if (( ${#missing[@]} > 0 )); then
        log ERROR "Missing required commands: ${missing[*]}"
        log ERROR "Install with: sudo apt install libimage-exiftool-perl jq ffmpeg coreutils bc"
        exit 1
    fi

    # Validate source
    if [[ ! -d "$SOURCE_DIR" ]]; then
        log ERROR "Source directory not found: $SOURCE_DIR"
        exit 1
    fi

    # Detect Google Photos subfolder name (handles locale variants)
    local gfoto_path="$SOURCE_DIR/$GOOGLE_FOTO_SUBDIR"
    if [[ ! -d "$gfoto_path" ]]; then
        for variant in "Google Photos" "Google Fotos" "Google Foto"; do
            if [[ -d "$SOURCE_DIR/$variant" ]]; then
                GOOGLE_FOTO_SUBDIR="$variant"
                gfoto_path="$SOURCE_DIR/$variant"
                break
            fi
        done
    fi
    if [[ ! -d "$gfoto_path" ]]; then
        log ERROR "Cannot find Google Photos folder in $SOURCE_DIR"
        log ERROR "Expected one of: Google Foto, Google Photos, Google Fotos"
        exit 1
    fi
    log INFO "Found photo folder: $GOOGLE_FOTO_SUBDIR"

    # Create temp working directory
    WORK_DIR="$(mktemp -d /tmp/takeout-proc.XXXXXX)"
    log DEBUG "Working temp dir: $WORK_DIR"

    # Create output directories
    if (( DRY_RUN )); then
        log DRY "Would create: $DEST_DIR/"
        log DRY "Would create: $DEST_DIR/.metadata/"
        # Still need dest dir for log file in dry-run
        mkdir -p "$DEST_DIR"
    else
        mkdir -p "$DEST_DIR/.metadata"
    fi

    log OK "Phase 0 complete. Source: $SOURCE_DIR, Dest: $DEST_DIR"
    if (( DRY_RUN )); then
        log DRY "DRY RUN MODE — no files will be modified"
    fi
    if (( USE_MOVE )); then
        log WARN "MOVE mode enabled — source files will be moved, not copied"
    fi
}

###############################################################################
# Phase 1: Index all files and match JSON <-> Media
#
# Strategy: Use file-based indexes instead of per-file jq calls.
# 1. find all files once, classify into media vs json lists
# 2. Batch-extract titles from all JSONs with a single jq invocation per file
#    but using xargs for parallelism
# 3. Build lookup tables from flat files, then load into bash arrays
###############################################################################

# File-based indexes (paths) — ALL use null (\0) delimiters for path safety
# $WORK_DIR/all_media.lst     — null-delimited media file paths
# $WORK_DIR/all_json.lst      — null-delimited json file paths
# $WORK_DIR/json_titles.tsv   — null-delimited records: json_path \t title \0
# $WORK_DIR/media_by_name.tsv — null-delimited records: basename \t full_path \0
# $WORK_DIR/matched.tsv       — null-delimited records: media_path \t json_path \0
# $WORK_DIR/orphan_json.lst   — null-delimited orphan json paths
# $WORK_DIR/hashes.tsv        — null-delimited records: hash \t file_path \0
# $WORK_DIR/deduped_media.lst — null-delimited deduplicated media paths

declare -A MEDIA_TO_JSON=()     # media_path -> json_path
declare -A MEDIA_ALBUM=()      # media_path -> album name (empty = root)
declare -A MEDIA_DEST=()       # media_path -> destination path
declare -A MEDIA_HASH=()       # media_path -> md5 hash
declare -A ALBUM_DIRS=()        # album_name -> 1

phase1_index() {
    log INFO "=== Phase 1: Indexing files and matching JSON to media ==="

    local gfoto_path="$SOURCE_DIR/$GOOGLE_FOTO_SUBDIR"

    # --- Step 1a: Collect all files and classify ---
    log INFO "Scanning all files..."
    spinner_start "Scanning files"
    find "$gfoto_path" -type f -print0 | while IFS= read -r -d '' fpath; do
        local bn
        bn="${fpath##*/}"
        local lc_bn="${bn,,}"

        # Skip album-level and top-level special JSONs
        case "$bn" in
            metadati.json|commenti_album_condivisi.json|titoli-memoria-generati-da-utente.json)
                continue ;;
        esac

        if [[ "$lc_bn" == *.json ]]; then
            printf '%s\0' "$fpath"  >> "$WORK_DIR/all_json.lst"
        else
            printf '%s\0' "$fpath" >> "$WORK_DIR/all_media.lst"
        fi
    done
    spinner_stop

    # Ensure files exist even if empty
    touch "$WORK_DIR/all_json.lst" "$WORK_DIR/all_media.lst"

    STAT_MEDIA_FOUND=$(tr '\0' '\n' < "$WORK_DIR/all_media.lst" | wc -l)
    local json_total
    json_total=$(tr '\0' '\n' < "$WORK_DIR/all_json.lst" | wc -l)
    log INFO "Found $STAT_MEDIA_FOUND media files, $json_total JSON metadata files"

    # --- Step 1b: Build media lookup index by basename ---
    log INFO "Building media filename index..."
    while IFS= read -r -d '' mpath; do
        local mbn="${mpath##*/}"
        printf '%s\t%s\0' "$mbn" "$mpath"
    done < "$WORK_DIR/all_media.lst" > "$WORK_DIR/media_by_name.tsv"

    # --- Step 1c: Batch extract titles from all JSONs ---
    log INFO "Extracting titles from $json_total JSON files (batch)..."

    # Use xargs -0 + jq to extract titles in parallel batches
    # Output: null-delimited records of json_path<TAB>title
    cat > "$WORK_DIR/extract_title.sh" << 'EXTRACT'
#!/bin/bash
for f in "$@"; do
    title=$(jq -r '.title // empty' "$f" 2>/dev/null)
    printf '%s\t%s\0' "$f" "$title"
done
EXTRACT
    chmod +x "$WORK_DIR/extract_title.sh"

    spinner_start "Extracting JSON titles ($(nproc) threads)"
    < "$WORK_DIR/all_json.lst" xargs -0 -P "$(nproc)" -n 50 "$WORK_DIR/extract_title.sh" \
        > "$WORK_DIR/json_titles.tsv" 2>/dev/null
    spinner_stop

    log INFO "Titles extracted, matching to media files..."

    # --- Step 1d: Match JSON -> Media ---
    # For each JSON, try:
    #   1. Same directory + title from JSON
    #   2. Same directory + suffix-stripped filename
    #   3. Any directory + title (search by basename)

    # Pre-sort media_by_name for faster grep lookup (newline-delimited copy for grep)
    tr '\0' '\n' < "$WORK_DIR/media_by_name.tsv" > "$WORK_DIR/media_by_name_nl.tsv"

    touch "$WORK_DIR/matched.tsv" "$WORK_DIR/orphan_json.lst"

    local match_count=0
    local orphan_count=0

    # Supplemental suffix patterns to strip (longest first)
    # After removing .json, the remaining name ends with one of these truncated suffixes
    local strip_regex='\.supplemental-metadata$|\.supplemental-metadat$|\.supplemental-metada$|\.supplemental-metad$|\.supplemental-meta$|\.supplemental-met$|\.supplemental-me$|\.supplemental-m$|\.supplemental-$|\.supplemental$|\.supplementa$|\.supplement$|\.supplemen$|\.suppleme$|\.supplem$|\.supple$|\.suppl$|\.supp$|\.sup$|\.su$|\.s$'

    progress_start "Matching JSON" "$json_total"

    while IFS=$'\t' read -r -d '' json_path title; do
        progress_tick

        local json_dir="${json_path%/*}"
        local json_bn="${json_path##*/}"
        local matched=""

        # Method 1: title + same directory
        if [[ -n "$title" ]]; then
            local candidate="$json_dir/$title"
            if [[ -f "$candidate" ]]; then
                matched="$candidate"
            fi
        fi

        # Method 2: strip supplemental suffix from JSON filename
        if [[ -z "$matched" ]]; then
            local without_json="${json_bn%.json}"
            local media_name
            media_name="$(echo "$without_json" | sed -E "s/(${strip_regex})//")"
            if [[ -n "$media_name" ]] && [[ "$media_name" != "$without_json" ]]; then
                local candidate2="$json_dir/$media_name"
                if [[ -f "$candidate2" ]]; then
                    matched="$candidate2"
                fi
            fi
        fi

        # Method 3: search by title in any directory (using pre-built index)
        if [[ -z "$matched" ]] && [[ -n "$title" ]]; then
            # Grep for exact basename match in the media index (newline-delimited copy)
            local found
            found="$(grep -F -m1 "$title" "$WORK_DIR/media_by_name_nl.tsv" 2>/dev/null | head -1)" || true
            if [[ -n "$found" ]]; then
                # Verify the basename matches exactly (grep may match partial)
                local found_bn="${found%%	*}"
                if [[ "$found_bn" == "$title" ]]; then
                    matched="${found#*	}"
                fi
            fi
        fi

        if [[ -n "$matched" ]]; then
            printf '%s\t%s\0' "$matched" "$json_path" >> "$WORK_DIR/matched.tsv"
            (( match_count++ )) || true
        else
            printf '%s\0' "$json_path" >> "$WORK_DIR/orphan_json.lst"
            (( orphan_count++ )) || true
        fi
    done < "$WORK_DIR/json_titles.tsv"

    progress_end

    STAT_JSON_MATCHED=$match_count
    STAT_JSON_ORPHAN=$orphan_count

    # Load matched pairs into associative array (media -> json, first match wins)
    while IFS=$'\t' read -r -d '' media_path json_path; do
        if [[ -z "${MEDIA_TO_JSON[$media_path]+x}" ]]; then
            MEDIA_TO_JSON["$media_path"]="$json_path"
        fi
    done < "$WORK_DIR/matched.tsv"

    log OK "Phase 1 complete: $STAT_JSON_MATCHED matched, $STAT_JSON_ORPHAN orphan JSONs"
}

###############################################################################
# Phase 2: Album resolution and destination assignment
###############################################################################
phase2_albums() {
    log INFO "=== Phase 2: Album resolution & destination assignment ==="

    local gfoto_path="$SOURCE_DIR/$GOOGLE_FOTO_SUBDIR"

    local chrono_pattern="^Foto da [0-9]{4}$"
    # Also handle English "Photos from YYYY"
    local chrono_pattern_en="^Photos from [0-9]{4}$"
    local unnamed_pattern="^Senza nome"
    local unnamed_pattern_en="^Untitled"

    while IFS= read -r -d '' subdir; do
        local dirname="${subdir##*/}"

        if [[ "$dirname" =~ $chrono_pattern ]] || [[ "$dirname" =~ $chrono_pattern_en ]]; then
            log DEBUG "Chronological folder: $dirname"
        elif [[ "$dirname" =~ $unnamed_pattern ]] || [[ "$dirname" =~ $unnamed_pattern_en ]]; then
            log DEBUG "Unnamed album (dissolve): $dirname"
        else
            ALBUM_DIRS["$dirname"]=1
            log DEBUG "Named album: $dirname"
        fi
    done < <(find "$gfoto_path" -mindepth 1 -maxdepth 1 -type d -print0 2>/dev/null)

    log INFO "Found ${#ALBUM_DIRS[@]} named albums"

    # Assign album membership for each media file
    log INFO "Assigning album membership..."
    while IFS= read -r -d '' media_path; do
        local parent_dir="${media_path%/*}"
        local folder_name="${parent_dir##*/}"

        if [[ -n "${ALBUM_DIRS[$folder_name]+x}" ]]; then
            MEDIA_ALBUM["$media_path"]="$folder_name"
        else
            MEDIA_ALBUM["$media_path"]=""
        fi
    done < "$WORK_DIR/all_media.lst"

    log OK "Phase 2 complete: destinations assigned"
}

###############################################################################
# Phase 3: Deduplication by MD5 hash
###############################################################################
phase3_dedup() {
    log INFO "=== Phase 3: Deduplication by content hash ==="

    local total=$STAT_MEDIA_FOUND
    log INFO "Computing MD5 hashes for $total media files (parallel)..."

    # Batch md5sum with xargs -0 for parallelism (null-delimited input)
    spinner_start "Hashing $total files ($(nproc) threads)"
    < "$WORK_DIR/all_media.lst" xargs -0 -P "$(nproc)" -I{} md5sum "{}" \
        > "$WORK_DIR/hashes_raw.txt" 2>/dev/null
    spinner_stop

    # Parse into null-delimited hash<TAB>path format
    log INFO "Parsing hashes..."
    while IFS= read -r line; do
        local hash="${line%% *}"
        # md5sum output: "hash  path" (two spaces)
        local path="${line#*  }"
        printf '%s\t%s\0' "$hash" "$path"
    done < "$WORK_DIR/hashes_raw.txt" > "$WORK_DIR/hashes.tsv"

    log INFO "Identifying duplicates..."

    # Load hashes into array and deduplicate
    local -A hash_seen          # hash -> kept media_path
    local -A dedup_remove       # media_path -> 1

    while IFS=$'\t' read -r -d '' hash media_path; do
        MEDIA_HASH["$media_path"]="$hash"

        if [[ -n "${hash_seen[$hash]+x}" ]]; then
            local existing="${hash_seen[$hash]}"
            local existing_album="${MEDIA_ALBUM[$existing]:-}"
            local current_album="${MEDIA_ALBUM[$media_path]:-}"

            # Priority: album copy wins over root copy
            if [[ -n "$current_album" ]] && [[ -z "$existing_album" ]]; then
                dedup_remove["$existing"]=1
                hash_seen["$hash"]="$media_path"
            else
                dedup_remove["$media_path"]=1
            fi
            (( STAT_DUPLICATES_REMOVED++ )) || true
        else
            hash_seen["$hash"]="$media_path"
        fi
    done < "$WORK_DIR/hashes.tsv"

    # Write deduplicated file list (null-delimited)
    local remaining=0
    while IFS= read -r -d '' media_path; do
        if [[ -z "${dedup_remove[$media_path]+x}" ]]; then
            printf '%s\0' "$media_path"
            (( remaining++ )) || true
        fi
    done < "$WORK_DIR/all_media.lst" > "$WORK_DIR/deduped_media.lst"

    log OK "Phase 3 complete: $STAT_DUPLICATES_REMOVED duplicates removed, $remaining unique files remain"
}

###############################################################################
# Phase 4: Copy/Move files to destination
###############################################################################
phase4_transfer() {
    log INFO "=== Phase 4: Transferring files to $DEST_DIR ==="

    local total
    total=$(tr '\0' '\n' < "$WORK_DIR/deduped_media.lst" | wc -l)
    local -A dest_names_used

    progress_start "Transferring" "$total"

    while IFS= read -r -d '' media_path; do
        progress_tick

        local basename="${media_path##*/}"
        local album="${MEDIA_ALBUM[$media_path]:-}"
        local dest_subdir=""

        if [[ -n "$album" ]]; then
            dest_subdir="$album"
        fi

        local dest_path
        if [[ -n "$dest_subdir" ]]; then
            dest_path="$DEST_DIR/$dest_subdir/$basename"
        else
            dest_path="$DEST_DIR/$basename"
        fi

        # Handle name conflicts
        if [[ -n "${dest_names_used[$dest_path]+x}" ]] || { (( ! DRY_RUN )) && [[ -f "$dest_path" ]]; }; then
            local ext="${basename##*.}"
            local name="${basename%.*}"
            local hash_short="${MEDIA_HASH[$media_path]:0:6}"
            local new_basename="${name}_${hash_short}.${ext}"
            if [[ -n "$dest_subdir" ]]; then
                dest_path="$DEST_DIR/$dest_subdir/$new_basename"
            else
                dest_path="$DEST_DIR/$new_basename"
            fi
            (( STAT_CONFLICTS_RESOLVED++ )) || true
            log DEBUG "Name conflict resolved: $basename -> $new_basename"
        fi

        dest_names_used["$dest_path"]=1
        MEDIA_DEST["$media_path"]="$dest_path"

        # Record mapping for later phases
        printf '%s\t%s\n' "$media_path" "$dest_path" >> "$WORK_DIR/transfer_map.tsv"

        local dest_dir="${dest_path%/*}"

        if (( DRY_RUN )); then
            log DRY "$(if (( USE_MOVE )); then echo 'mv'; else echo 'cp'; fi) '${media_path##*/}' -> '$dest_path'"
        else
            mkdir -p "$dest_dir"
            if (( USE_MOVE )); then
                mv -- "$media_path" "$dest_path"
            else
                cp -- "$media_path" "$dest_path"
            fi
        fi
        (( STAT_FILES_COPIED++ )) || true

        # Transfer corresponding JSON to .metadata/
        if [[ -n "${MEDIA_TO_JSON[$media_path]+x}" ]]; then
            local json_src="${MEDIA_TO_JSON[$media_path]}"
            local dest_basename="${dest_path##*/}"

            local meta_dest
            if [[ -n "$dest_subdir" ]]; then
                meta_dest="$DEST_DIR/.metadata/$dest_subdir/${dest_basename}.json"
            else
                meta_dest="$DEST_DIR/.metadata/${dest_basename}.json"
            fi

            if (( DRY_RUN )); then
                log DRY "metadata: '${json_src##*/}' -> '$meta_dest'"
            else
                mkdir -p "${meta_dest%/*}"
                cp -- "$json_src" "$meta_dest"
            fi
        fi
    done < "$WORK_DIR/deduped_media.lst"

    progress_end

    log OK "Phase 4 complete: $STAT_FILES_COPIED files transferred, $STAT_CONFLICTS_RESOLVED conflicts resolved"
}

###############################################################################
# Phase 5: Convert .MP and .COVER files to .mp4
###############################################################################
phase5_convert_mp() {
    log INFO "=== Phase 5: Converting Motion Photo (.MP/.COVER) files to .mp4 ==="

    local converted=0
    local mp_total=0

    # Count MP/COVER files first
    for media_path in "${!MEDIA_DEST[@]}"; do
        local lc_dest="${MEDIA_DEST[$media_path],,}"
        if [[ "$lc_dest" =~ \.(mp|cover)$ ]]; then
            (( mp_total++ )) || true
        fi
    done

    if (( mp_total == 0 )); then
        log OK "Phase 5 complete: no Motion Photo files found"
        return
    fi

    progress_start "Converting MP" "$mp_total"

    for media_path in "${!MEDIA_DEST[@]}"; do
        local dest_path="${MEDIA_DEST[$media_path]}"
        local lc_dest="${dest_path,,}"

        # Match files ending in .mp or .cover (case-insensitive, but NOT .mp4, .mp3 etc.)
        if [[ "$lc_dest" =~ \.(mp|cover)$ ]]; then
            progress_tick
            local mp4_dest="${dest_path}.mp4"

            if (( DRY_RUN )); then
                log DRY "ffmpeg convert: '${dest_path##*/}' -> '${mp4_dest##*/}'"
            else
                if ffmpeg -y -i "$dest_path" -map 0:0 -c:v copy -movflags +faststart \
                    "$mp4_dest" </dev/null 2>>"$LOG_FILE"; then
                    (( converted++ )) || true
                else
                    log ERROR "Failed to convert: ${dest_path##*/}"
                    (( STAT_ERRORS++ )) || true
                fi
            fi
        fi
    done

    progress_end

    STAT_MP_CONVERTED=$converted
    log OK "Phase 5 complete: $converted/$mp_total Motion Photo files converted"
}

###############################################################################
# Phase 6: Apply EXIF metadata from JSON
#
# Optimized: extract all needed fields from JSON in a single jq call per file,
# and batch exiftool calls where possible.
###############################################################################
phase6_apply_exif() {
    log INFO "=== Phase 6: Applying EXIF metadata from JSON ==="

    # Build list of files that need EXIF from JSON (null-delimited)
    local exif_work="$WORK_DIR/exif_work.tsv"
    : > "$exif_work"

    for media_path in "${!MEDIA_DEST[@]}"; do
        if [[ -n "${MEDIA_TO_JSON[$media_path]+x}" ]]; then
            printf '%s\t%s\t%s\0' "$media_path" "${MEDIA_DEST[$media_path]}" "${MEDIA_TO_JSON[$media_path]}"
        fi
    done > "$exif_work"

    local total
    total=$(tr '\0' '\n' < "$exif_work" | wc -l)
    log INFO "Applying metadata to $total files..."

    local applied=0

    progress_start "Applying EXIF" "$total"

    while IFS=$'\t' read -r -d '' media_path dest_path json_path; do
        progress_tick

        # Single jq call to extract all needed fields
        local json_data
        json_data="$(jq -r '[
            (.photoTakenTime.timestamp // ""),
            ((.geoDataExif.latitude // .geoData.latitude) // ""),
            ((.geoDataExif.longitude // .geoData.longitude) // ""),
            ((.geoDataExif.altitude // .geoData.altitude) // ""),
            (.description // ""),
            ([.people[]?.name // empty] | join("|"))
        ] | @tsv' "$json_path" 2>/dev/null)" || true

        if [[ -z "$json_data" ]]; then
            continue
        fi

        IFS=$'\t' read -r photo_ts geo_lat geo_lon geo_alt description people_pipe <<< "$json_data"

        # Build exiftool arguments
        local -a exif_args=("-overwrite_original")

        # Date/time
        if [[ -n "$photo_ts" ]] && [[ "$photo_ts" != "0" ]]; then
            local exif_date
            exif_date="$(date -d "@$photo_ts" '+%Y:%m:%d %H:%M:%S' 2>/dev/null)" || true
            if [[ -n "$exif_date" ]]; then
                exif_args+=("-DateTimeOriginal=$exif_date" "-CreateDate=$exif_date" "-ModifyDate=$exif_date")
                local lc_dest="${dest_path,,}"
                if [[ "$lc_dest" =~ \.(mp4|mov|avi|3gp|m4v|mkv)$ ]]; then
                    exif_args+=("-MediaCreateDate=$exif_date" "-MediaModifyDate=$exif_date")
                    exif_args+=("-TrackCreateDate=$exif_date" "-TrackModifyDate=$exif_date")
                fi
            fi
        fi

        # GPS data (skip if both lat/lon are 0.0)
        if [[ -n "$geo_lat" ]] && [[ -n "$geo_lon" ]]; then
            local is_zero_lat=0 is_zero_lon=0
            [[ "$geo_lat" =~ ^-?0(\.0+)?$ ]] && is_zero_lat=1
            [[ "$geo_lon" =~ ^-?0(\.0+)?$ ]] && is_zero_lon=1
            if (( ! is_zero_lat || ! is_zero_lon )); then
                local lat_ref="N" lon_ref="E"
                if [[ "$geo_lat" == -* ]]; then
                    lat_ref="S"; geo_lat="${geo_lat#-}"
                fi
                if [[ "$geo_lon" == -* ]]; then
                    lon_ref="W"; geo_lon="${geo_lon#-}"
                fi
                exif_args+=("-GPSLatitude=$geo_lat" "-GPSLatitudeRef=$lat_ref")
                exif_args+=("-GPSLongitude=$geo_lon" "-GPSLongitudeRef=$lon_ref")
                if [[ -n "$geo_alt" ]] && [[ "$geo_alt" != "0" ]] && ! [[ "$geo_alt" =~ ^-?0(\.0+)?$ ]]; then
                    local alt_ref=0
                    if [[ "$geo_alt" == -* ]]; then
                        alt_ref=1; geo_alt="${geo_alt#-}"
                    fi
                    exif_args+=("-GPSAltitude=$geo_alt" "-GPSAltitudeRef=$alt_ref")
                fi
            fi
        fi

        # Description
        if [[ -n "$description" ]]; then
            exif_args+=("-ImageDescription=$description" "-Description=$description")
        fi

        # People as keywords (pipe-separated from jq)
        if [[ -n "$people_pipe" ]]; then
            IFS='|' read -ra people_arr <<< "$people_pipe"
            for person in "${people_arr[@]}"; do
                person="${person## }"; person="${person%% }"  # trim
                if [[ -n "$person" ]]; then
                    exif_args+=("-Keywords+=$person" "-Subject+=$person")
                fi
            done
        fi

        # Only apply if we have something beyond -overwrite_original
        if (( ${#exif_args[@]} <= 1 )); then
            continue
        fi

        if (( DRY_RUN )); then
            log DRY "exiftool [${#exif_args[@]} args] '${dest_path##*/}'"
        else
            if exiftool "${exif_args[@]}" "$dest_path" 2>>"$LOG_FILE" >/dev/null; then
                (( applied++ )) || true
            else
                log WARN "exiftool failed on: ${dest_path##*/}"
                (( STAT_ERRORS++ )) || true
            fi

            # Set filesystem time
            if [[ -n "$photo_ts" ]] && [[ "$photo_ts" != "0" ]]; then
                local touch_date
                touch_date="$(date -d "@$photo_ts" '+%Y%m%d%H%M.%S' 2>/dev/null)" || true
                if [[ -n "$touch_date" ]]; then
                    touch -t "$touch_date" "$dest_path" 2>/dev/null || true
                fi
            fi
        fi
    done < "$exif_work"

    progress_end

    STAT_EXIF_APPLIED=$applied
    log OK "Phase 6 complete: $applied files had EXIF metadata applied"
}

###############################################################################
# Phase 7: Fallback — extract date from filename
###############################################################################
phase7_date_from_filename() {
    log INFO "=== Phase 7: Extracting dates from filenames (for files without JSON) ==="

    local applied=0
    local attempted=0

    # Count files without JSON for progress
    local no_json_total=0
    for media_path in "${!MEDIA_DEST[@]}"; do
        if [[ -z "${MEDIA_TO_JSON[$media_path]+x}" ]]; then
            (( no_json_total++ )) || true
        fi
    done

    if (( no_json_total == 0 )); then
        log OK "Phase 7 complete: all files had JSON metadata, no filename fallback needed"
        return
    fi

    progress_start "Filename dates" "$no_json_total"

    for media_path in "${!MEDIA_DEST[@]}"; do
        # Skip files that already have JSON (handled in Phase 6)
        if [[ -n "${MEDIA_TO_JSON[$media_path]+x}" ]]; then
            continue
        fi

        progress_tick

        local dest_path="${MEDIA_DEST[$media_path]}"
        local basename="${dest_path##*/}"

        local extracted_date=""
        local extracted_ts=""

        # Pattern: PXL_YYYYMMDD_HHMMSSmmm
        if [[ "$basename" =~ ^PXL_([0-9]{4})([0-9]{2})([0-9]{2})_([0-9]{2})([0-9]{2})([0-9]{2}) ]]; then
            extracted_date="${BASH_REMATCH[1]}:${BASH_REMATCH[2]}:${BASH_REMATCH[3]} ${BASH_REMATCH[4]}:${BASH_REMATCH[5]}:${BASH_REMATCH[6]}"

        elif [[ "$basename" =~ ^IMG_([0-9]{4})([0-9]{2})([0-9]{2})_([0-9]{2})([0-9]{2})([0-9]{2}) ]]; then
            extracted_date="${BASH_REMATCH[1]}:${BASH_REMATCH[2]}:${BASH_REMATCH[3]} ${BASH_REMATCH[4]}:${BASH_REMATCH[5]}:${BASH_REMATCH[6]}"

        elif [[ "$basename" =~ ^IMG([0-9]{4})([0-9]{2})([0-9]{2})([0-9]{2})([0-9]{2})([0-9]{2}) ]]; then
            extracted_date="${BASH_REMATCH[1]}:${BASH_REMATCH[2]}:${BASH_REMATCH[3]} ${BASH_REMATCH[4]}:${BASH_REMATCH[5]}:${BASH_REMATCH[6]}"

        elif [[ "$basename" =~ ^VID_([0-9]{4})([0-9]{2})([0-9]{2})_([0-9]{2})([0-9]{2})([0-9]{2}) ]]; then
            extracted_date="${BASH_REMATCH[1]}:${BASH_REMATCH[2]}:${BASH_REMATCH[3]} ${BASH_REMATCH[4]}:${BASH_REMATCH[5]}:${BASH_REMATCH[6]}"

        elif [[ "$basename" =~ ^Screenshot_([0-9]{4})([0-9]{2})([0-9]{2})-([0-9]{2})([0-9]{2})([0-9]{2}) ]]; then
            extracted_date="${BASH_REMATCH[1]}:${BASH_REMATCH[2]}:${BASH_REMATCH[3]} ${BASH_REMATCH[4]}:${BASH_REMATCH[5]}:${BASH_REMATCH[6]}"

        elif [[ "$basename" =~ ^screen-([0-9]{4})([0-9]{2})([0-9]{2})-([0-9]{2})([0-9]{2})([0-9]{2}) ]]; then
            extracted_date="${BASH_REMATCH[1]}:${BASH_REMATCH[2]}:${BASH_REMATCH[3]} ${BASH_REMATCH[4]}:${BASH_REMATCH[5]}:${BASH_REMATCH[6]}"

        # WhatsApp — date only
        elif [[ "$basename" =~ ^IMG-([0-9]{4})([0-9]{2})([0-9]{2})-WA ]]; then
            extracted_date="${BASH_REMATCH[1]}:${BASH_REMATCH[2]}:${BASH_REMATCH[3]} 12:00:00"

        # Telegram: photo_YYYY-MM-DD HH.MM.SS
        elif [[ "$basename" =~ ^photo_([0-9]{4})-([0-9]{2})-([0-9]{2})\ ([0-9]{2})\.([0-9]{2})\.([0-9]{2}) ]]; then
            extracted_date="${BASH_REMATCH[1]}:${BASH_REMATCH[2]}:${BASH_REMATCH[3]} ${BASH_REMATCH[4]}:${BASH_REMATCH[5]}:${BASH_REMATCH[6]}"

        # Bare: YYYYMMDD_HHMMSS
        elif [[ "$basename" =~ ^([0-9]{4})([0-9]{2})([0-9]{2})_([0-9]{2})([0-9]{2})([0-9]{2}) ]]; then
            extracted_date="${BASH_REMATCH[1]}:${BASH_REMATCH[2]}:${BASH_REMATCH[3]} ${BASH_REMATCH[4]}:${BASH_REMATCH[5]}:${BASH_REMATCH[6]}"

        # YYYY-MM-DD HH.MM.SS
        elif [[ "$basename" =~ ^([0-9]{4})-([0-9]{2})-([0-9]{2})\ ([0-9]{2})\.([0-9]{2})\.([0-9]{2}) ]]; then
            extracted_date="${BASH_REMATCH[1]}:${BASH_REMATCH[2]}:${BASH_REMATCH[3]} ${BASH_REMATCH[4]}:${BASH_REMATCH[5]}:${BASH_REMATCH[6]}"

        # Facebook: FB_IMG_<unix_ms_timestamp>
        elif [[ "$basename" =~ ^FB_IMG_([0-9]{10,13}) ]]; then
            local fb_ts="${BASH_REMATCH[1]}"
            if (( ${#fb_ts} == 13 )); then
                fb_ts="${fb_ts:0:10}"
            fi
            extracted_date="$(date -d "@$fb_ts" '+%Y:%m:%d %H:%M:%S' 2>/dev/null)" || true
            extracted_ts="$fb_ts"
        fi

        if [[ -z "$extracted_date" ]]; then
            continue
        fi

        (( attempted++ )) || true

        # Compute unix timestamp for touch
        if [[ -z "$extracted_ts" ]]; then
            local date_for_parse="${extracted_date:0:4}-${extracted_date:5:2}-${extracted_date:8:2} ${extracted_date:11}"
            extracted_ts="$(date -d "$date_for_parse" '+%s' 2>/dev/null)" || true
        fi

        if (( DRY_RUN )); then
            log DRY "Date from filename: $basename -> $extracted_date"
            (( applied++ )) || true
        else
            local -a exif_args=(
                "-overwrite_original"
                "-DateTimeOriginal=$extracted_date"
                "-CreateDate=$extracted_date"
                "-ModifyDate=$extracted_date"
            )

            local lc_dest="${dest_path,,}"
            if [[ "$lc_dest" =~ \.(mp4|mov|avi|3gp|m4v|mkv)$ ]]; then
                exif_args+=("-MediaCreateDate=$extracted_date" "-MediaModifyDate=$extracted_date")
            fi

            if exiftool "${exif_args[@]}" "$dest_path" 2>>"$LOG_FILE" >/dev/null; then
                (( applied++ )) || true
            else
                log WARN "exiftool failed for filename-date: ${dest_path##*/}"
                (( STAT_ERRORS++ )) || true
            fi

            if [[ -n "$extracted_ts" ]]; then
                local touch_date
                touch_date="$(date -d "@$extracted_ts" '+%Y%m%d%H%M.%S' 2>/dev/null)" || true
                if [[ -n "$touch_date" ]]; then
                    touch -t "$touch_date" "$dest_path" 2>/dev/null || true
                fi
            fi
        fi
    done

    progress_end

    STAT_DATE_FROM_FILENAME=$applied
    log OK "Phase 7 complete: $applied/$attempted files had date applied from filename"
}

###############################################################################
# Phase 8: Generate missing-files.json and final report
###############################################################################
phase8_report() {
    log INFO "=== Phase 8: Generating reports ==="

    local missing_file="$DEST_DIR/missing-files.json"
    local orphan_file="$WORK_DIR/orphan_json.lst"

    if [[ -s "$orphan_file" ]]; then
        local orphan_count
        orphan_count=$(tr '\0' '\n' < "$orphan_file" | wc -l)
        log INFO "Building missing-files.json with $orphan_count orphan entries..."

        if (( DRY_RUN )); then
            log DRY "Would write $missing_file with $orphan_count entries"
        else
            # Batch extract all fields from orphan JSONs
            # Use a helper script for parallelism
            cat > "$WORK_DIR/extract_orphan.sh" << 'ORPHAN_EXTRACT'
#!/bin/bash
for f in "$@"; do
    jq -c --arg jp "$f" --arg sf "$(basename "$(dirname "$f")")" '{
        title: (.title // ""),
        url: (.url // ""),
        photoTakenTime: (.photoTakenTime.timestamp // ""),
        creationTime: (.creationTime.timestamp // ""),
        jsonPath: $jp,
        sourceFolder: $sf
    }' "$f" 2>/dev/null
done
ORPHAN_EXTRACT
            chmod +x "$WORK_DIR/extract_orphan.sh"

            # Extract data from all orphan JSONs (null-delimited input)
            < "$orphan_file" xargs -0 -P "$(nproc)" -n 20 "$WORK_DIR/extract_orphan.sh" \
                > "$WORK_DIR/orphan_data.jsonl" 2>/dev/null

            # Add photoTakenDate field and assemble final JSON array
            jq -s '[.[] | . + {
                photoTakenDate: (if .photoTakenTime != "" and .photoTakenTime != "0"
                    then (.photoTakenTime | tonumber | strftime("%Y-%m-%d %H:%M:%S"))
                    else "" end)
            }]' "$WORK_DIR/orphan_data.jsonl" > "$missing_file" 2>/dev/null

            log OK "Written $missing_file"
        fi
    else
        if (( DRY_RUN )); then
            log DRY "No orphan JSON files — no missing-files.json needed"
        else
            log OK "No orphan JSON files — no missing-files.json needed"
        fi
    fi

    # Final statistics
    echo ""
    log INFO "==========================================="
    log INFO "         PROCESSING COMPLETE               "
    log INFO "==========================================="
    log INFO "Media files found:        $STAT_MEDIA_FOUND"
    log INFO "JSON matched to media:    $STAT_JSON_MATCHED"
    log INFO "Orphan JSONs (no media):  $STAT_JSON_ORPHAN"
    log INFO "Duplicates removed:       $STAT_DUPLICATES_REMOVED"
    log INFO "Files transferred:        $STAT_FILES_COPIED"
    log INFO "Name conflicts resolved:  $STAT_CONFLICTS_RESOLVED"
    log INFO "Motion Photos converted:  $STAT_MP_CONVERTED"
    log INFO "EXIF applied from JSON:   $STAT_EXIF_APPLIED"
    log INFO "Date from filename:       $STAT_DATE_FROM_FILENAME"
    log INFO "Errors:                   $STAT_ERRORS"
    log INFO "Albums:                   ${#ALBUM_DIRS[@]}"
    log INFO "==========================================="
    log INFO "Log file:  $LOG_FILE"
    if [[ -f "$missing_file" ]]; then
        log INFO "Missing:   $missing_file"
    fi
    echo ""
}

###############################################################################
# Main
###############################################################################
main() {
    parse_args "$@"

    # Set up log file (always need the directory)
    mkdir -p "$DEST_DIR"
    LOG_FILE="$DEST_DIR/process.log"
    : > "$LOG_FILE"

    local start_time
    start_time=$(date +%s)

    log INFO "Google Photos Takeout Processor — started at $(date)"
    log INFO "Arguments: source=$SOURCE_DIR dest=$DEST_DIR dry_run=$DRY_RUN move=$USE_MOVE"

    phase0_setup
    phase1_index
    phase2_albums
    phase3_dedup
    phase4_transfer
    phase5_convert_mp
    phase6_apply_exif
    phase7_date_from_filename
    phase8_report

    local end_time elapsed
    end_time=$(date +%s)
    elapsed=$(( end_time - start_time ))
    log OK "All done in ${elapsed}s!"
}

main "$@"
