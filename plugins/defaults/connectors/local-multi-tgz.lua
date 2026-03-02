-- ═══════════════════════════════════════════════════════════════════════════════
-- Local Multi-TGZ Connector Plugin
-- ═══════════════════════════════════════════════════════════════════════════════
-- A connector that reads data from multiple .tar.gz archives.
-- Each archive is extracted on Initialize, files are served from temp dirs.

local revoco = require("revoco")

-- ── Plugin Metadata ────────────────────────────────────────────────────────────

Plugin = {
    id = "local-multi-tgz",
    name = "Local Multi-TGZ",
    description = "Read data from multiple .tar.gz archives",
    version = "1.0.0",
    author = "revoco",
    type = "connector",
    capabilities = {"read", "list"},
    data_types = {"photo", "video", "audio", "document", "unknown"},
}

-- ── Configuration Schema ───────────────────────────────────────────────────────

Config = {
    {
        id = "paths",
        name = "TGZ File Paths",
        description = "List of paths to .tar.gz archives (JSON array or comma-separated)",
        type = "string_list",
        required = true,
    },
    {
        id = "extract_dir",
        name = "Extraction Directory",
        description = "Base directory for extraction (default: auto temp dir)",
        type = "path",
        required = false,
    },
}

-- ── Internal State ─────────────────────────────────────────────────────────────

local state = {
    config = nil,
    tgz_paths = {},
    extract_dirs = {},  -- [{tgz_path, extract_dir, owns}]
    all_files = {},     -- [{name, path, size, mod_time, tgz_path}]
}

-- ── Data Type Detection ────────────────────────────────────────────────────────

local photo_exts = {
    [".jpg"]=true, [".jpeg"]=true, [".png"]=true, [".gif"]=true,
    [".webp"]=true, [".heic"]=true, [".heif"]=true, [".raw"]=true,
    [".cr2"]=true, [".nef"]=true, [".arw"]=true, [".dng"]=true,
    [".tiff"]=true, [".tif"]=true, [".bmp"]=true,
}
local video_exts = {
    [".mp4"]=true, [".mov"]=true, [".avi"]=true, [".mkv"]=true,
    [".webm"]=true, [".m4v"]=true, [".wmv"]=true, [".flv"]=true,
    [".3gp"]=true, [".mts"]=true, [".m2ts"]=true,
}
local audio_exts = {
    [".mp3"]=true, [".m4a"]=true, [".wav"]=true, [".flac"]=true,
    [".aac"]=true, [".ogg"]=true, [".wma"]=true, [".opus"]=true,
}
local doc_exts = {
    [".pdf"]=true, [".doc"]=true, [".docx"]=true, [".txt"]=true,
    [".md"]=true, [".json"]=true, [".xml"]=true, [".html"]=true,
    [".xls"]=true, [".xlsx"]=true, [".ppt"]=true, [".pptx"]=true,
}

local function detect_data_type(path)
    local ext = string.lower(revoco.extname(path))
    if photo_exts[ext] then return "photo" end
    if video_exts[ext] then return "video" end
    if audio_exts[ext] then return "audio" end
    if doc_exts[ext] then return "document" end
    return "unknown"
end

-- ── Helper: parse paths from config ────────────────────────────────────────────

local function parse_paths(config)
    local paths = config.paths
    if type(paths) == "table" then
        return paths
    end
    if type(paths) == "string" then
        local result = {}
        for p in paths:gmatch("[^,]+") do
            local trimmed = p:match("^%s*(.-)%s*$")
            if trimmed ~= "" then
                table.insert(result, trimmed)
            end
        end
        return result
    end
    return {}
end

-- ── Connector Functions ────────────────────────────────────────────────────────

function ValidateConfig(config)
    local paths = parse_paths(config)
    if #paths == 0 then
        return false, "paths is required (provide a list of .tar.gz file paths)"
    end
    for _, p in ipairs(paths) do
        if not revoco.exists(p) then
            return false, "TGZ file does not exist: " .. p
        end
    end
    return true
end

function Initialize(config)
    local ok, err = ValidateConfig(config)
    if not ok then
        return err
    end

    state.config = config
    state.tgz_paths = parse_paths(config)
    state.extract_dirs = {}
    state.all_files = {}

    local seen = {} -- deduplicate filenames across archives

    for _, tgz_path in ipairs(state.tgz_paths) do
        -- Determine extraction directory
        local extract_dir
        local owns = false

        if config.extract_dir and config.extract_dir ~= "" then
            -- Use subdirectory per archive
            local base = revoco.basename(tgz_path):gsub("%.tar%.gz$", ""):gsub("%.tgz$", "")
            extract_dir = revoco.join(config.extract_dir, base)
            revoco.mkdir(extract_dir)
        else
            local tmp = revoco.tempfile("revoco-mtgz")
            revoco.remove(tmp)
            revoco.mkdir(tmp)
            extract_dir = tmp
            owns = true
        end

        table.insert(state.extract_dirs, {
            tgz_path = tgz_path,
            extract_dir = extract_dir,
            owns = owns,
        })

        -- Extract
        revoco.log.info("Extracting " .. revoco.basename(tgz_path) .. " to " .. extract_dir)
        local files, terr = revoco.tar.extractAll(tgz_path, extract_dir)
        if terr then
            revoco.log.warn("Failed to extract " .. tgz_path .. ": " .. terr)
            goto continue
        end

        for _, f in ipairs(files or {}) do
            if not seen[f.name] then
                seen[f.name] = true
                f.tgz_path = tgz_path
                table.insert(state.all_files, f)
            end
        end

        ::continue::
    end

    revoco.log.info("Extracted " .. #state.all_files .. " files from " .. #state.tgz_paths .. " archives")
    return true
end

function TestConnection(config)
    local ok, err = ValidateConfig(config)
    if not ok then
        return false, err
    end

    local paths = parse_paths(config)
    local total_files = 0

    for _, path in ipairs(paths) do
        local entries, terr = revoco.tar.list(path)
        if terr then
            return false, "failed to read " .. revoco.basename(path) .. ": " .. terr
        end
        for _, e in ipairs(entries) do
            if not e.is_dir then total_files = total_files + 1 end
        end
    end

    return true, #paths .. " archives, " .. total_files .. " total files"
end

function List()
    if not state.config then
        return nil, "connector not initialized"
    end

    local items = {}
    for _, f in ipairs(state.all_files) do
        table.insert(items, {
            id = f.name,
            type = detect_data_type(f.name),
            path = f.path,
            source_path = f.path,
            size = f.size,
            metadata = {
                mod_time = f.mod_time,
                tgz_path = f.tgz_path,
                rel_path = f.name,
                file_name = revoco.basename(f.name),
            },
        })
    end

    return items
end

function Read(item)
    local path = item.source_path or item.path
    if not path or path == "" then
        -- Try to find the file in extraction directories
        for _, ed in ipairs(state.extract_dirs) do
            local try_path = revoco.join(ed.extract_dir, item.id)
            if revoco.exists(try_path) then
                path = try_path
                break
            end
        end
    end

    if not path or path == "" then
        return nil, "file not found: " .. item.id
    end

    local content, err = revoco.readFile(path)
    if err then
        return nil, "failed to read: " .. err
    end
    return content
end

function ReadTo(item, dest_path, mode)
    local src_path = item.source_path or item.path
    if not src_path or src_path == "" then
        for _, ed in ipairs(state.extract_dirs) do
            local try_path = revoco.join(ed.extract_dir, item.id)
            if revoco.exists(try_path) then
                src_path = try_path
                break
            end
        end
    end

    if not src_path or src_path == "" then
        return false, "file not found: " .. item.id
    end

    revoco.mkdir(revoco.dirname(dest_path))

    if mode == "move" then
        local ok, err = revoco.move(src_path, dest_path)
        if not ok then
            ok, err = revoco.copy(src_path, dest_path)
            if not ok then
                return false, "copy failed: " .. (err or "unknown")
            end
            revoco.remove(src_path)
        end
        return true
    elseif mode == "reference" then
        return revoco.symlink(src_path, dest_path)
    else
        return revoco.copy(src_path, dest_path)
    end
end

function Close()
    -- Clean up temp extraction directories
    for _, ed in ipairs(state.extract_dirs) do
        if ed.owns and ed.extract_dir ~= "" then
            revoco.remove(ed.extract_dir)
        end
    end
    state.config = nil
    state.tgz_paths = {}
    state.extract_dirs = {}
    state.all_files = {}
    return true
end
