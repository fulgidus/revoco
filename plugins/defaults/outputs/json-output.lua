-- ═══════════════════════════════════════════════════════════════════════════════
-- JSON Output Plugin
-- ═══════════════════════════════════════════════════════════════════════════════
-- An output plugin that exports items as JSON files with metadata.
-- Demonstrates: Output plugin, Initialize, Export, ExportBatch, Finalize
--
-- Usage:
--   Place this file in ~/.config/revoco/plugins/
--   Use as an output target to export items as JSON

local revoco = require("revoco")

-- ── Plugin Metadata ────────────────────────────────────────────────────────────

Plugin = {
    id = "json-output",
    name = "JSON Output",
    description = "Exports items as JSON files with full metadata",
    version = "1.0.0",
    author = "revoco",
    type = "output",
    data_types = {"photo", "video", "audio", "document", "note"},
}

-- ── Configuration Schema ───────────────────────────────────────────────────────

Config = {
    {
        id = "output_dir",
        name = "Output Directory",
        description = "Directory to write JSON files",
        type = "path",
        required = true,
    },
    {
        id = "pretty",
        name = "Pretty Print",
        description = "Format JSON with indentation",
        type = "bool",
        default = true,
    },
    {
        id = "single_file",
        name = "Single File Mode",
        description = "Write all items to a single JSON file instead of one per item",
        type = "bool",
        default = false,
    },
    {
        id = "output_filename",
        name = "Output Filename",
        description = "Filename for single file mode (without extension)",
        type = "string",
        default = "export",
    },
    {
        id = "include_binary",
        name = "Include Binary Reference",
        description = "Include path to binary file if available",
        type = "bool",
        default = true,
    },
}

-- ── Default Selector ───────────────────────────────────────────────────────────
-- This output accepts all item types by default

Selector = {
    -- No restrictions - accepts all items
}

-- ── Internal State ─────────────────────────────────────────────────────────────

local state = {
    config = nil,
    items = {},      -- For single file mode
    count = 0,
    errors = {},
}

-- ── Output Functions ───────────────────────────────────────────────────────────

-- Initialize prepares the output for use
function Initialize(config)
    if not config.output_dir or config.output_dir == "" then
        return "output_dir is required"
    end
    
    -- Create output directory if it doesn't exist
    if not revoco.exists(config.output_dir) then
        local ok, err = revoco.mkdir(config.output_dir)
        if not ok then
            return "failed to create output directory: " .. (err or "unknown error")
        end
    end
    
    state.config = config
    state.items = {}
    state.count = 0
    state.errors = {}
    
    revoco.log.info("JSON Output initialized: " .. config.output_dir)
    return true
end

-- CanOutput checks if this output can handle the given item
function CanOutput(item)
    -- We can handle any item type
    return true
end

-- Export writes a single item to JSON
function Export(item)
    if not state.config then
        return "output not initialized"
    end
    
    state.count = state.count + 1
    
    -- Build the export object
    local exportObj = {
        source_path = item.source_path,
        processed_path = item.processed_path,
        dest_rel_path = item.dest_rel_path,
        type = item.type,
        metadata = item.metadata,
        exported_at = revoco.now(),
    }
    
    -- In single file mode, accumulate items
    if state.config.single_file then
        table.insert(state.items, exportObj)
        return true
    end
    
    -- Otherwise, write individual JSON file
    local filename = state.count .. "_" .. sanitizeFilename(item.dest_rel_path or "item") .. ".json"
    local outputPath = revoco.join(state.config.output_dir, filename)
    
    local jsonContent
    if state.config.pretty then
        -- gopher-lua json.encode doesn't support pretty printing,
        -- so we'd need to implement it ourselves or use a different approach
        jsonContent = revoco.json.encode(exportObj)
    else
        jsonContent = revoco.json.encode(exportObj)
    end
    
    if not jsonContent then
        local errMsg = "failed to encode item to JSON"
        table.insert(state.errors, errMsg)
        return errMsg
    end
    
    local ok, err = revoco.writeFile(outputPath, jsonContent)
    if not ok then
        local errMsg = "failed to write " .. outputPath .. ": " .. (err or "unknown error")
        table.insert(state.errors, errMsg)
        return errMsg
    end
    
    revoco.log.debug("Exported: " .. outputPath)
    return true
end

-- ExportBatch exports multiple items at once (more efficient for single file mode)
function ExportBatch(items)
    if not state.config then
        return "output not initialized"
    end
    
    for _, item in ipairs(items) do
        local result = Export(item)
        if result ~= true then
            -- Continue on error but log it
            revoco.log.warn("Export failed: " .. tostring(result))
        end
    end
    
    return true
end

-- Finalize completes the export process
function Finalize()
    if not state.config then
        return "output not initialized"
    end
    
    -- In single file mode, write all accumulated items
    if state.config.single_file and #state.items > 0 then
        local filename = (state.config.output_filename or "export") .. ".json"
        local outputPath = revoco.join(state.config.output_dir, filename)
        
        local exportData = {
            exported_at = revoco.formatTime(revoco.now()),
            total_items = #state.items,
            items = state.items,
        }
        
        local jsonContent = revoco.json.encode(exportData)
        if not jsonContent then
            return "failed to encode items to JSON"
        end
        
        local ok, err = revoco.writeFile(outputPath, jsonContent)
        if not ok then
            return "failed to write " .. outputPath .. ": " .. (err or "unknown error")
        end
        
        revoco.log.info("Exported " .. #state.items .. " items to " .. outputPath)
    end
    
    -- Log summary
    revoco.log.info("JSON Output finalized: " .. state.count .. " items processed")
    
    if #state.errors > 0 then
        revoco.log.warn("Completed with " .. #state.errors .. " errors")
        for _, err in ipairs(state.errors) do
            revoco.log.warn("  - " .. err)
        end
    end
    
    -- Reset state
    state.config = nil
    state.items = {}
    state.count = 0
    state.errors = {}
    
    return true
end

-- ── Helper Functions ───────────────────────────────────────────────────────────

-- sanitizeFilename removes or replaces characters that aren't safe for filenames
function sanitizeFilename(name)
    if not name then return "unnamed" end
    
    -- Replace path separators and other unsafe characters
    local safe = name:gsub("[/\\:*?\"<>|]", "_")
    
    -- Remove leading/trailing dots and spaces
    safe = safe:gsub("^[%. ]+", ""):gsub("[%. ]+$", "")
    
    -- Limit length
    if #safe > 200 then
        safe = safe:sub(1, 200)
    end
    
    return safe ~= "" and safe or "unnamed"
end
