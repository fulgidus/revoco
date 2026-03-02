-- ═══════════════════════════════════════════════════════════════════════════════
-- Local Multi-ZIP Connector Plugin
-- ═══════════════════════════════════════════════════════════════════════════════
-- A connector that reads data from multiple ZIP archives.
-- Useful for Google Takeout and other services that split exports into parts.

local revoco = require("revoco")

-- ── Plugin Metadata ────────────────────────────────────────────────────────────

Plugin = {
    id = "local-multi-zip",
    name = "Local Multi-ZIP",
    description = "Read data from multiple ZIP archives",
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
        name = "ZIP File Paths",
        description = "List of paths to ZIP archives (JSON array or comma-separated)",
        type = "string_list",
        required = true,
    },
}

-- ── Internal State ─────────────────────────────────────────────────────────────

local state = {
    config = nil,
    handles = {},  -- {path=string, handle=number}
    zip_paths = {},
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
    -- Comma-separated string fallback
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
        return false, "paths is required (provide a list of ZIP file paths)"
    end
    for _, p in ipairs(paths) do
        if not revoco.exists(p) then
            return false, "ZIP file does not exist: " .. p
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
    state.zip_paths = parse_paths(config)
    state.handles = {}

    -- Open all ZIP files
    for _, path in ipairs(state.zip_paths) do
        local handle, zerr = revoco.zip.open(path)
        if not handle then
            -- Close any already opened
            Close()
            return "failed to open ZIP " .. path .. ": " .. (zerr or "unknown")
        end
        table.insert(state.handles, {path = path, handle = handle})
    end

    return true
end

function TestConnection(config)
    local ok, err = ValidateConfig(config)
    if not ok then
        return false, err
    end

    local paths = parse_paths(config)
    local total_entries = 0

    for _, path in ipairs(paths) do
        local handle, zerr = revoco.zip.open(path)
        if not handle then
            return false, "failed to open " .. revoco.basename(path) .. ": " .. (zerr or "unknown")
        end
        local entries = revoco.zip.list(handle)
        revoco.zip.close(handle)
        if entries then
            total_entries = total_entries + #entries
        end
    end

    return true, #paths .. " ZIPs, " .. total_entries .. " total entries"
end

function List()
    if #state.handles == 0 then
        return nil, "connector not initialized"
    end

    local items = {}
    local seen = {} -- deduplicate across archives

    for _, zh in ipairs(state.handles) do
        local entries, err = revoco.zip.list(zh.handle)
        if err then
            revoco.log.warn("failed to list " .. zh.path .. ": " .. err)
            goto continue
        end

        for _, entry in ipairs(entries) do
            if not entry.is_dir and not seen[entry.name] then
                seen[entry.name] = true
                table.insert(items, {
                    id = entry.name,
                    type = detect_data_type(entry.name),
                    path = entry.name,
                    source_path = zh.path,
                    size = entry.size,
                    metadata = {
                        mod_time = entry.mod_time,
                        compressed_size = entry.compressed_size,
                        zip_path = zh.path,
                        rel_path = entry.name,
                        file_name = revoco.basename(entry.name),
                    },
                })
            end
        end

        ::continue::
    end

    return items
end

function Read(item)
    -- Find which ZIP handle contains this file
    local zip_path = item.source_path or (item.metadata and item.metadata.zip_path)
    if zip_path then
        -- Try the specific ZIP first
        for _, zh in ipairs(state.handles) do
            if zh.path == zip_path then
                local content, err = revoco.zip.read(zh.handle, item.id)
                if content then return content end
            end
        end
    end

    -- Fall back: search all ZIPs
    for _, zh in ipairs(state.handles) do
        local content, err = revoco.zip.read(zh.handle, item.id)
        if content then return content end
    end

    return nil, "file not found in any ZIP: " .. item.id
end

function ReadTo(item, dest_path, mode)
    -- Ensure parent directory
    revoco.mkdir(revoco.dirname(dest_path))

    -- Find which ZIP handle contains this file
    local zip_path = item.source_path or (item.metadata and item.metadata.zip_path)
    if zip_path then
        for _, zh in ipairs(state.handles) do
            if zh.path == zip_path then
                local ok, err = revoco.zip.extract(zh.handle, item.id, dest_path)
                if ok then return true end
            end
        end
    end

    -- Fall back: search all ZIPs
    for _, zh in ipairs(state.handles) do
        local ok, err = revoco.zip.extract(zh.handle, item.id, dest_path)
        if ok then return true end
    end

    return false, "file not found in any ZIP: " .. item.id
end

function Close()
    for _, zh in ipairs(state.handles) do
        revoco.zip.close(zh.handle)
    end
    state.handles = {}
    state.config = nil
    state.zip_paths = {}
    return true
end
