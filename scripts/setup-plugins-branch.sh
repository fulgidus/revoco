#!/bin/bash
# Script to create/update the orphan 'plugins' branch for the plugin registry
# Run this from the revoco repository root

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
PLUGINS_DIR="/tmp/revoco-plugins-branch-$$"

echo "Creating plugins branch content in $PLUGINS_DIR"

# Create temp directory
mkdir -p "$PLUGINS_DIR"
cd "$PLUGINS_DIR"

# Initialize git repo
git init
git checkout -b plugins

# Copy registry files from template
if [ -d "$REPO_ROOT/.plugins-branch-template" ]; then
    cp -r "$REPO_ROOT/.plugins-branch-template"/* .
fi

# Create directory structure if not exists
mkdir -p connectors processors outputs

# Create or update index.json
cat > index.json << 'EOF'
{
  "version": "1",
  "last_updated": "2026-03-01T00:00:00Z",
  "plugins": {}
}
EOF

# Create README
cat > README.md << 'EOF'
# Revoco Plugin Registry

This branch contains the plugin registry for revoco.

## Structure

```
index.json          # Plugin registry index
connectors/         # Connector plugins
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

## Installing Plugins

```bash
revoco plugins search csv
revoco plugins install <plugin-id>
revoco plugins update --all
```
EOF

# Commit
git add .
git commit -m "Initialize plugin registry"

echo ""
echo "Plugins branch created in $PLUGINS_DIR"
echo ""
echo "To push to GitHub, run:"
echo "  cd $PLUGINS_DIR"
echo "  git remote add origin git@github.com:fulgidus/revoco.git"
echo "  git push -u origin plugins"
echo ""
echo "Or to update existing branch:"
echo "  git push -f origin plugins"
