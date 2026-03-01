-- ═══════════════════════════════════════════════════════════════════════════════
-- CSV Connector Plugin
-- ═══════════════════════════════════════════════════════════════════════════════
-- A connector that reads data from CSV files.
-- Demonstrates: Plugin metadata, config schema, Initialize, List, Read
--
-- Usage:
--   Place this file in ~/.config/revoco/plugins/
--   Configure with a path to a directory containing CSV files

local revoco = require("revoco")

-- ── Plugin Metadata ────────────────────────────────────────────────────────────

Plugin = {
    id = "csv-connector",
    name = "CSV Connector",
    description = "Reads data from CSV files as structured items",
    version = "1.0.0",
    author = "revoco",
    type = "connector",
    capabilities = {"read"},
    data_types = {"document"},
}

-- ── Configuration Schema ───────────────────────────────────────────────────────

Config = {
    {
        id = "path",
        name = "CSV Directory",
        description = "Path to directory containing CSV files",
        type = "path",
        required = true,
    },
    {
        id = "recursive",
        name = "Recursive",
        description = "Search subdirectories for CSV files",
        type = "bool",
        default = false,
    },
    {
        id = "delimiter",
        name = "Delimiter",
        description = "CSV field delimiter",
        type = "select",
        options = {",", ";", "\t", "|"},
        default = ",",
    },
}

-- ── Internal State ─────────────────────────────────────────────────────────────

local state = {
    config = nil,
    files = {},
}

-- ── Connector Functions ────────────────────────────────────────────────────────

-- ValidateConfig checks that the provided configuration is valid
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

-- Initialize prepares the connector for use
function Initialize(config)
    local ok, err = ValidateConfig(config)
    if not ok then
        return err
    end
    
    state.config = config
    revoco.log.info("CSV Connector initialized with path: " .. config.path)
    return true
end

-- TestConnection verifies that the connector can access the configured path
function TestConnection(config)
    local ok, err = ValidateConfig(config)
    if not ok then
        return false, err
    end
    
    -- Try to list CSV files
    local pattern = "*.csv"
    local files = revoco.glob(config.path, pattern)
    
    if files == nil then
        return false, "failed to access directory"
    end
    
    return true, "Found " .. #files .. " CSV files"
end

-- List returns all items available from this connector
function List()
    if not state.config then
        return nil, "connector not initialized"
    end
    
    local items = {}
    local pattern = state.config.recursive and "**/*.csv" or "*.csv"
    
    local files, err = revoco.glob(state.config.path, pattern)
    if err then
        return nil, err
    end
    
    for i, filePath in ipairs(files or {}) do
        -- Read first line to get column headers
        local content, readErr = revoco.readFile(filePath)
        local columns = {}
        
        if content then
            local firstLine = content:match("([^\n]*)")
            if firstLine then
                local delimiter = state.config.delimiter or ","
                for col in firstLine:gmatch("[^" .. delimiter .. "]+") do
                    table.insert(columns, col:match("^%s*(.-)%s*$")) -- trim whitespace
                end
            end
        end
        
        local relPath = filePath:sub(#state.config.path + 2) -- Remove base path + /
        local name = revoco.basename(filePath)
        
        table.insert(items, {
            id = "csv:" .. revoco.hash.md5(filePath),
            name = name,
            type = "document",
            path = relPath,
            source_path = filePath,
            size = #(content or ""),
            metadata = {
                format = "csv",
                columns = columns,
                column_count = #columns,
                delimiter = state.config.delimiter or ",",
            },
        })
    end
    
    revoco.log.info("Found " .. #items .. " CSV files")
    return items
end

-- Read returns the content of a single item
function Read(item)
    if not item.source_path then
        return nil, "item has no source_path"
    end
    
    local content, err = revoco.readFile(item.source_path)
    if err then
        return nil, "failed to read file: " .. err
    end
    
    return content
end

-- Close cleans up any resources
function Close()
    state.config = nil
    state.files = {}
    revoco.log.info("CSV Connector closed")
    return true
end
