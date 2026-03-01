-- Dropbox Output
-- Export processed media directly to Dropbox

Plugin = {
    id = "dropbox-output",
    name = "Dropbox Output",
    description = "Export processed media directly to Dropbox",
    version = "0.1.0",
    author = "revoco",
    
    -- Output capabilities
    capabilities = {"write", "mkdir", "auth"},
    
    -- Requires OAuth2 authentication
    requires_auth = true,
    auth_type = "oauth2",
    
    -- Configuration
    config = {
        {
            id = "access_token",
            name = "Access Token",
            type = "password",
            required = true,
            description = "Dropbox API access token"
        },
        {
            id = "base_path",
            name = "Base Path",
            type = "path",
            default = "/revoco-export",
            description = "Base path in Dropbox for exports"
        },
        {
            id = "preserve_structure",
            name = "Preserve Structure",
            type = "bool",
            default = true,
            description = "Preserve original folder structure"
        }
    }
}

-- Initialize the output
function init(config)
    Plugin.config_values = config
    
    if not config.access_token or config.access_token == "" then
        return false, "Access token is required"
    end
    
    return true
end

-- Write a file to Dropbox
function write(item, dest_path)
    local config = Plugin.config_values
    local full_path = config.base_path .. "/" .. dest_path
    
    revoco.log("info", "Uploading to Dropbox: " .. full_path)
    
    -- Read file content
    local content = revoco.read_file(item.path)
    if not content then
        return false, "Failed to read source file"
    end
    
    -- Upload via Dropbox API (placeholder)
    -- In real implementation, would use HTTP client to call Dropbox API
    local result = dropbox_upload(full_path, content, config.access_token)
    
    if result.success then
        return true, {
            dropbox_path = full_path,
            rev = result.rev
        }
    else
        return false, result.error
    end
end

-- Create a directory in Dropbox
function mkdir(path)
    local config = Plugin.config_values
    local full_path = config.base_path .. "/" .. path
    
    revoco.log("info", "Creating Dropbox folder: " .. full_path)
    
    -- Placeholder - would call Dropbox API
    return true
end

-- Test connection
function test()
    local config = Plugin.config_values
    
    -- Placeholder - would verify token with Dropbox API
    return true, "Connected to Dropbox"
end

-- Helper function (placeholder)
function dropbox_upload(path, content, token)
    -- This would be implemented using revoco.http or similar
    return {
        success = true,
        rev = "abc123"
    }
end
