-- ═══════════════════════════════════════════════════════════════════════════════
-- Local ZIP Connector Plugin
-- ═══════════════════════════════════════════════════════════════════════════════
-- A connector that reads data from a single ZIP archive.
-- Uses the revoco.zip module for handle-based ZIP access.

local revoco = require("revoco")

-- ── Plugin Metadata ────────────────────────────────────────────────────────────

Plugin = {
    id = "local-zip",
    name = "Local ZIP",
    description = "Read data from a ZIP archive",
    version = "1.0.0",
    author = "revoco",
    type = "connector",
    capabilities = {"read", "list"},
    data_types = {"photo", "video", "audio", "document", "unknown"},
}

-- ── Configuration Schema ───────────────────────────────────────────────────────

Config = {
    {
        id = "path",
        name = "ZIP File Path",
        description = "Path to the ZIP archive",
        type = "path",
        required = true,
    },
}

-- ── Internal State ─────────────────────────────────────────────────────────────

local state = {
    config = nil,
    handle = nil, -- ZIP handle from revoco.zip.open
    zip_path = "",
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

-- ── Connector Functions ────────────────────────────────────────────────────────

function ValidateConfig(config)
    if not config.path or config.path == "" then
        return false, "path is required"
    end
    if not revoco.exists(config.path) then
        return false, "ZIP file does not exist: " .. config.path
    end
    return true
end

function Initialize(config)
    local ok, err = ValidateConfig(config)
    if not ok then
        return err
    end

    state.config = config
    state.zip_path = config.path

    -- Open the ZIP file
    local handle, zerr = revoco.zip.open(config.path)
    if not handle then
        return "failed to open ZIP: " .. (zerr or "unknown")
    end
    state.handle = handle

    return true
end

function TestConnection(config)
    local ok, err = ValidateConfig(config)
    if not ok then
        return false, err
    end

    -- Try opening and listing
    local handle, zerr = revoco.zip.open(config.path)
    if not handle then
        return false, "failed to open ZIP: " .. (zerr or "unknown")
    end

    local entries = revoco.zip.list(handle)
    revoco.zip.close(handle)

    if not entries then
        return false, "failed to list ZIP contents"
    end

    return true, "ZIP contains " .. #entries .. " entries"
end

function List()
    if not state.handle then
        return nil, "connector not initialized"
    end

    local entries, err = revoco.zip.list(state.handle)
    if err then
        return nil, "failed to list ZIP: " .. err
    end

    local items = {}
    for _, entry in ipairs(entries) do
        if not entry.is_dir then
            table.insert(items, {
                id = entry.name,
                type = detect_data_type(entry.name),
                path = entry.name,
                source_path = state.zip_path,
                size = entry.size,
                metadata = {
                    mod_time = entry.mod_time,
                    compressed_size = entry.compressed_size,
                    zip_path = state.zip_path,
                    rel_path = entry.name,
                    file_name = revoco.basename(entry.name),
                },
            })
        end
    end

    return items
end

function Read(item)
    if not state.handle then
        return nil, "connector not initialized"
    end
    local content, err = revoco.zip.read(state.handle, item.id)
    if err then
        return nil, "failed to read from ZIP: " .. err
    end
    return content
end

function ReadTo(item, dest_path, mode)
    if not state.handle then
        return false, "connector not initialized"
    end

    -- Ensure parent directory
    revoco.mkdir(revoco.dirname(dest_path))

    -- Extract directly to disk (no Lua memory overhead)
    return revoco.zip.extract(state.handle, item.id, dest_path)
end

function Close()
    if state.handle then
        revoco.zip.close(state.handle)
        state.handle = nil
    end
    state.config = nil
    state.zip_path = ""
    return true
end
