-- ═══════════════════════════════════════════════════════════════════════════════
-- Local Folder Connector Plugin
-- ═══════════════════════════════════════════════════════════════════════════════
-- A connector that reads and writes data from/to local filesystem directories.
-- Supports copy, move, and reference (symlink) import modes.

local revoco = require("revoco")

-- ── Plugin Metadata ────────────────────────────────────────────────────────────

Plugin = {
    id = "local-folder",
    name = "Local Folder",
    description = "Read/write data from local filesystem directories",
    version = "1.0.0",
    author = "revoco",
    type = "connector",
    capabilities = {"read", "write", "list", "delete"},
    data_types = {"photo", "video", "audio", "document", "unknown"},
}

-- ── Configuration Schema ───────────────────────────────────────────────────────

Config = {
    {
        id = "path",
        name = "Folder Path",
        description = "Path to the local folder",
        type = "path",
        required = true,
    },
    {
        id = "recursive",
        name = "Recursive",
        description = "Include files in subdirectories",
        type = "bool",
        default = true,
    },
    {
        id = "extensions",
        name = "File Extensions",
        description = "Comma-separated list of extensions to include (empty = all)",
        type = "string",
        default = "",
    },
}

-- ── Internal State ─────────────────────────────────────────────────────────────

local state = {
    config = nil,
    root_path = "",
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

local function matches_extension(path, extensions)
    if not extensions or #extensions == 0 then
        return true
    end
    local ext = string.lower(revoco.extname(path))
    for _, e in ipairs(extensions) do
        if ext == e then return true end
    end
    return false
end

local function parse_extensions(ext_str)
    if not ext_str or ext_str == "" then return {} end
    local exts = {}
    for ext in ext_str:gmatch("[^,]+") do
        ext = ext:match("^%s*(.-)%s*$") -- trim
        if not ext:match("^%.") then ext = "." .. ext end
        table.insert(exts, ext:lower())
    end
    return exts
end

-- ── Connector Functions ────────────────────────────────────────────────────────

function ValidateConfig(config)
    if not config.path or config.path == "" then
        return false, "path is required"
    end
    if not revoco.exists(config.path) then
        return false, "path does not exist: " .. config.path
    end
    if not revoco.isDir(config.path) then
        return false, "path is not a directory: " .. config.path
    end
    return true
end

function Initialize(config)
    local ok, err = ValidateConfig(config)
    if not ok then
        return err
    end
    state.config = config
    state.root_path = config.path
    return true
end

function TestConnection(config)
    local ok, err = ValidateConfig(config)
    if not ok then
        return false, err
    end
    local entries = revoco.listDir(config.path)
    if entries == nil then
        return false, "failed to access directory"
    end
    return true, "Accessible, " .. #entries .. " entries"
end

function List()
    if not state.config then
        return nil, "connector not initialized"
    end

    local recursive = true
    if state.config.recursive == false then
        recursive = false
    end

    local extensions = parse_extensions(state.config.extensions)
    local items = {}

    if recursive then
        local entries, err = revoco.walk(state.root_path)
        if err then
            return nil, "walk failed: " .. err
        end
        for _, entry in ipairs(entries) do
            if not entry.is_dir and matches_extension(entry.path, extensions) then
                -- Compute relative path
                local rel_path = entry.path:sub(#state.root_path + 2)
                table.insert(items, {
                    id = rel_path,
                    type = detect_data_type(entry.path),
                    path = entry.path,
                    source_path = entry.path,
                    size = entry.size,
                    metadata = {
                        mod_time = entry.mod_time,
                        rel_path = rel_path,
                        file_name = entry.name,
                    },
                })
            end
        end
    else
        local entries, err = revoco.listDir(state.root_path)
        if err then
            return nil, "listDir failed: " .. err
        end
        for _, entry in ipairs(entries) do
            if not entry.is_dir then
                local full_path = revoco.join(state.root_path, entry.name)
                if matches_extension(full_path, extensions) then
                    table.insert(items, {
                        id = entry.name,
                        type = detect_data_type(full_path),
                        path = full_path,
                        source_path = full_path,
                        size = entry.size,
                        metadata = {
                            mod_time = entry.mod_time,
                            rel_path = entry.name,
                            file_name = entry.name,
                        },
                    })
                end
            end
        end
    end

    return items
end

function Read(item)
    local path = item.source_path or item.path
    if not path or path == "" then
        path = revoco.join(state.root_path, item.id)
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
        src_path = revoco.join(state.root_path, item.id)
    end

    -- Ensure parent directory
    revoco.mkdir(revoco.dirname(dest_path))

    if mode == "move" then
        local ok, err = revoco.move(src_path, dest_path)
        if not ok then
            -- Fall back to copy + delete
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
        -- Default: copy
        return revoco.copy(src_path, dest_path)
    end
end

function Write(item, content)
    local dest_path = revoco.join(state.root_path, item.id)
    revoco.mkdir(revoco.dirname(dest_path))
    return revoco.writeFile(dest_path, content)
end

function WriteFrom(item, source_path)
    local dest_path = revoco.join(state.root_path, item.id)
    revoco.mkdir(revoco.dirname(dest_path))
    return revoco.copy(source_path, dest_path)
end

function Delete(item)
    local path = item.path
    if not path or path == "" then
        path = revoco.join(state.root_path, item.id)
    end
    return revoco.remove(path)
end

function Close()
    state.config = nil
    state.root_path = ""
    return true
end
