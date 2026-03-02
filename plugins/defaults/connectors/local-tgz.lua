-- ═══════════════════════════════════════════════════════════════════════════════
-- Local TGZ Connector Plugin
-- ═══════════════════════════════════════════════════════════════════════════════
-- A connector that reads data from a single .tar.gz archive.
-- Extracts to a temp directory on Initialize, then serves files from there.

local revoco = require("revoco")

-- ── Plugin Metadata ────────────────────────────────────────────────────────────

Plugin = {
    id = "local-tgz",
    name = "Local TGZ",
    description = "Read data from a .tar.gz archive",
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
        name = "TGZ File Path",
        description = "Path to the .tar.gz archive",
        type = "path",
        required = true,
    },
    {
        id = "extract_dir",
        name = "Extraction Directory",
        description = "Directory to extract files to (default: auto temp dir)",
        type = "path",
        required = false,
    },
}

-- ── Internal State ─────────────────────────────────────────────────────────────

local state = {
    config = nil,
    tgz_path = "",
    extract_dir = "",
    extracted_files = {}, -- [{name, path, size, mod_time}]
    owns_extract_dir = false, -- whether we created the temp dir
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
        return false, "TGZ file does not exist: " .. config.path
    end
    return true
end

function Initialize(config)
    local ok, err = ValidateConfig(config)
    if not ok then
        return err
    end

    state.config = config
    state.tgz_path = config.path

    -- Determine extraction directory
    if config.extract_dir and config.extract_dir ~= "" then
        state.extract_dir = config.extract_dir
        state.owns_extract_dir = false
    else
        local tmp = revoco.tempfile("revoco-tgz")
        -- tempfile creates a file, we need a directory
        revoco.remove(tmp)
        revoco.mkdir(tmp)
        state.extract_dir = tmp
        state.owns_extract_dir = true
    end

    -- Extract all files
    revoco.log.info("Extracting " .. revoco.basename(config.path) .. " to " .. state.extract_dir)
    local files, terr = revoco.tar.extractAll(config.path, state.extract_dir)
    if terr then
        return "failed to extract TGZ: " .. terr
    end

    state.extracted_files = files or {}
    revoco.log.info("Extracted " .. #state.extracted_files .. " files")

    return true
end

function TestConnection(config)
    local ok, err = ValidateConfig(config)
    if not ok then
        return false, err
    end

    -- List without extracting
    local entries, terr = revoco.tar.list(config.path)
    if terr then
        return false, "failed to read TGZ: " .. terr
    end

    local file_count = 0
    for _, e in ipairs(entries) do
        if not e.is_dir then file_count = file_count + 1 end
    end

    return true, "TGZ contains " .. file_count .. " files"
end

function List()
    if not state.config then
        return nil, "connector not initialized"
    end

    local items = {}
    for _, f in ipairs(state.extracted_files) do
        table.insert(items, {
            id = f.name,
            type = detect_data_type(f.name),
            path = f.path,
            source_path = f.path,
            size = f.size,
            metadata = {
                mod_time = f.mod_time,
                tgz_path = state.tgz_path,
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
        path = revoco.join(state.extract_dir, item.id)
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
        src_path = revoco.join(state.extract_dir, item.id)
    end

    -- Ensure parent directory
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
    -- Clean up temp extraction directory if we created it
    if state.owns_extract_dir and state.extract_dir ~= "" then
        revoco.remove(state.extract_dir)
    end
    state.config = nil
    state.tgz_path = ""
    state.extract_dir = ""
    state.extracted_files = {}
    state.owns_extract_dir = false
    return true
end
