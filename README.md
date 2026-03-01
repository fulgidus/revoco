# Revoco Plugin Registry

This branch contains the plugin registry for revoco.

## Structure

```
index.json          # Plugin registry index
connectors/         # Connector plugins
  <plugin-id>/
    plugin.lua      # Lua plugin file
    meta.json       # Optional metadata
processors/         # Processor plugins
outputs/            # Output plugins
```

## Adding Plugins

To submit a plugin to the registry:

1. Fork this repository
2. Switch to the `plugins` branch
3. Add your plugin to the appropriate directory
4. Update `index.json` with your plugin entry
5. Submit a pull request

## Plugin Entry Format

```json
{
  "id": "my-plugin",
  "name": "My Plugin",
  "description": "Description of what the plugin does",
  "author": "Your Name",
  "version": "1.0.0",
  "type": "connector|processor|output",
  "tier": "lua|external",
  "tags": ["tag1", "tag2"],
  "path": "connectors/my-plugin/plugin.lua",
  "platforms": ["linux", "darwin", "windows"],
  "min_version": "0.1.0"
}
```

## Installing Plugins

```bash
# Search for plugins
revoco plugins search csv

# Install a plugin
revoco plugins install my-plugin

# Update plugins
revoco plugins update --all
```
