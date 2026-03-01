-- HEIC Processor
-- Convert HEIC images to JPEG/PNG with metadata preservation

Plugin = {
    id = "heic-processor",
    name = "HEIC Processor",
    description = "Convert HEIC images to JPEG/PNG with metadata preservation",
    version = "0.1.0",
    author = "revoco",
    
    -- Selector for matching items
    selector = "ext:heic,heif",
    
    -- Binary dependencies
    dependencies = {
        {
            binary = "convert",
            min_version = "6.0",
            install = "Install ImageMagick: apt install imagemagick / brew install imagemagick"
        }
    },
    
    -- Configuration
    config = {
        {
            id = "output_format",
            name = "Output Format",
            type = "select",
            options = {"jpeg", "png"},
            default = "jpeg",
            description = "Format to convert HEIC files to"
        },
        {
            id = "quality",
            name = "Quality",
            type = "int",
            default = 90,
            description = "Output quality (1-100, for JPEG)"
        },
        {
            id = "preserve_metadata",
            name = "Preserve Metadata",
            type = "bool",
            default = true,
            description = "Copy EXIF metadata to converted file"
        }
    }
}

-- Check if this processor can handle the item
function can_process(item)
    local ext = item.extension or ""
    ext = ext:lower()
    return ext == "heic" or ext == "heif"
end

-- Process an item
function process(item, config)
    local output_format = config.output_format or "jpeg"
    local quality = config.quality or 90
    
    revoco.log("info", "Converting HEIC to " .. output_format .. ": " .. item.path)
    
    -- Build output path
    local output_path = item.path:gsub("%.[^.]+$", "." .. output_format)
    
    -- Build ImageMagick command
    local cmd = string.format(
        "convert '%s' -quality %d '%s'",
        item.path,
        quality,
        output_path
    )
    
    -- Execute conversion
    local result = revoco.exec(cmd)
    if not result.success then
        return nil, "Conversion failed: " .. (result.error or "unknown error")
    end
    
    -- Copy metadata if requested
    if config.preserve_metadata then
        local exif_cmd = string.format(
            "exiftool -TagsFromFile '%s' -all:all '%s' -overwrite_original",
            item.path,
            output_path
        )
        revoco.exec(exif_cmd)
    end
    
    -- Return modified item
    return {
        path = output_path,
        extension = output_format,
        converted_from = item.path,
        metadata = item.metadata
    }
end
