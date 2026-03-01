-- ═══════════════════════════════════════════════════════════════════════════════
-- EXIF Processor Plugin
-- ═══════════════════════════════════════════════════════════════════════════════
-- A processor that extracts EXIF metadata from images using exiftool.
-- Demonstrates: Plugin with binary dependencies, selectors, Process function
--
-- Requires: exiftool (https://exiftool.org/)
--
-- Usage:
--   Place this file in ~/.config/revoco/plugins/
--   Automatically processes photos to extract EXIF metadata

local revoco = require("revoco")

-- ── Plugin Metadata ────────────────────────────────────────────────────────────

Plugin = {
    id = "exif-processor",
    name = "EXIF Processor",
    description = "Extracts EXIF metadata from images using exiftool",
    version = "1.0.0",
    author = "revoco",
    type = "processor",
    data_types = {"photo"},
}

-- ── Binary Dependencies ────────────────────────────────────────────────────────

Dependencies = {
    {
        binary = "exiftool",
        check = "exiftool -ver",
        version_regex = "^([0-9.]+)",
        min_version = "12.0",
        install = {
            apt = "sudo apt install libimage-exiftool-perl",
            brew = "brew install exiftool",
            dnf = "sudo dnf install perl-Image-ExifTool",
            pacman = "sudo pacman -S perl-image-exiftool",
        },
    },
}

-- ── Configuration Schema ───────────────────────────────────────────────────────

Config = {
    {
        id = "extract_gps",
        name = "Extract GPS",
        description = "Extract GPS coordinates from images",
        type = "bool",
        default = true,
    },
    {
        id = "extract_camera",
        name = "Extract Camera Info",
        description = "Extract camera make/model information",
        type = "bool",
        default = true,
    },
    {
        id = "extract_dates",
        name = "Extract Dates",
        description = "Extract all date fields (DateTimeOriginal, CreateDate, etc.)",
        type = "bool",
        default = true,
    },
    {
        id = "preserve_raw",
        name = "Preserve Raw EXIF",
        description = "Store the complete raw EXIF output in metadata",
        type = "bool",
        default = false,
    },
}

-- ── Default Selector ───────────────────────────────────────────────────────────
-- This processor only runs on photo items with image extensions

Selector = {
    types = {"photo"},
    extensions = {".jpg", ".jpeg", ".png", ".tiff", ".tif", ".heic", ".heif", ".webp", ".raw", ".cr2", ".nef", ".arw"},
}

-- ── Processor Functions ────────────────────────────────────────────────────────

-- CanProcess checks if this processor can handle the given item
function CanProcess(item)
    -- Already handled by selector, but we can add extra logic here
    if not item.source_path then
        return false
    end
    
    -- Check if file exists and is readable
    if not revoco.exists(item.source_path) then
        return false
    end
    
    return true
end

-- Process extracts EXIF metadata from an image
function Process(item, config)
    if not item.source_path then
        return nil, "item has no source_path"
    end
    
    -- Run exiftool to extract metadata as JSON
    local result = revoco.exec("exiftool", {"-json", "-n", item.source_path})
    
    if result.exit_code ~= 0 then
        revoco.log.warn("exiftool failed for " .. item.source_path .. ": " .. (result.stderr or "unknown error"))
        return item -- Return unchanged item on failure
    end
    
    -- Parse JSON output
    local exifData, err = revoco.json.decode(result.stdout)
    if err then
        revoco.log.warn("failed to parse exiftool output: " .. err)
        return item
    end
    
    -- exiftool returns an array, get first element
    local exif = exifData[1]
    if not exif then
        return item
    end
    
    -- Initialize metadata if needed
    if not item.metadata then
        item.metadata = {}
    end
    
    -- Extract GPS coordinates if enabled
    if config.extract_gps ~= false then
        if exif.GPSLatitude and exif.GPSLongitude then
            item.metadata.gps = {
                latitude = exif.GPSLatitude,
                longitude = exif.GPSLongitude,
                altitude = exif.GPSAltitude,
            }
            revoco.log.debug("Extracted GPS: " .. exif.GPSLatitude .. ", " .. exif.GPSLongitude)
        end
    end
    
    -- Extract camera info if enabled
    if config.extract_camera ~= false then
        item.metadata.camera = {
            make = exif.Make,
            model = exif.Model,
            lens = exif.LensModel or exif.Lens,
            serial = exif.SerialNumber,
        }
        
        -- Exposure settings
        item.metadata.exposure = {
            aperture = exif.FNumber or exif.Aperture,
            shutter_speed = exif.ExposureTime or exif.ShutterSpeed,
            iso = exif.ISO,
            focal_length = exif.FocalLength,
            focal_length_35mm = exif.FocalLengthIn35mmFormat,
            exposure_mode = exif.ExposureMode,
            metering_mode = exif.MeteringMode,
        }
    end
    
    -- Extract dates if enabled
    if config.extract_dates ~= false then
        -- Try various date fields in order of preference
        local dateOriginal = exif.DateTimeOriginal or exif.CreateDate or exif.ModifyDate
        
        if dateOriginal then
            -- Parse the EXIF date format (YYYY:MM:DD HH:MM:SS)
            local parsedDate = revoco.parseTime(dateOriginal)
            if parsedDate then
                item.metadata.date_taken = parsedDate.iso
                item.metadata.date_taken_unix = parsedDate.unix
                
                -- Also set the item's created_at if not already set
                if not item.created_at or item.created_at == 0 then
                    item.created_at = parsedDate.unix
                end
            end
        end
        
        -- Store all date fields
        item.metadata.dates = {
            original = exif.DateTimeOriginal,
            created = exif.CreateDate,
            modified = exif.ModifyDate,
            digitized = exif.DateTimeDigitized,
        }
    end
    
    -- Store image dimensions
    item.metadata.dimensions = {
        width = exif.ImageWidth,
        height = exif.ImageHeight,
        orientation = exif.Orientation,
    }
    
    -- Store raw EXIF if enabled
    if config.preserve_raw then
        item.metadata.exif_raw = exif
    end
    
    -- Mark as processed
    item.metadata.exif_processed = true
    item.metadata.exif_processor_version = Plugin.version
    
    revoco.log.info("Processed EXIF for: " .. item.name)
    
    return item
end

-- ProcessBatch can be implemented for efficiency, but the default
-- behavior (calling Process for each item) works fine for exiftool
-- which is already quite fast per-file.
