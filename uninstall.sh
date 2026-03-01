#!/bin/sh
# revoco uninstaller script
# Usage: curl -fsSL https://raw.githubusercontent.com/fulgidus/revoco/main/uninstall.sh | bash
#
# Options:
#   --yes       Skip all prompts and remove everything
#   --help      Show help message
#
# Note: When piped (non-interactive), --yes is required to proceed.

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m' # No Color

# Paths
CONFIG_DIR="$HOME/.config/revoco"
SESSIONS_DIR="$HOME/.revoco/sessions"
INSTALL_INFO="$CONFIG_DIR/install.json"

# Detect cache directory
detect_cache_dir() {
    case "$(uname -s)" in
        Darwin*|Linux*)
            echo "$HOME/.cache/revoco"
            ;;
        CYGWIN*|MINGW*|MSYS*)
            echo "$LOCALAPPDATA/revoco"
            ;;
        *)
            echo "$HOME/.cache/revoco"
            ;;
    esac
}

CACHE_DIR=$(detect_cache_dir)

# Options
SKIP_PROMPTS=false
SHOW_HELP=false

info() {
    printf "${BLUE}[INFO]${NC} %s\n" "$1"
}

success() {
    printf "${GREEN}[OK]${NC} %s\n" "$1"
}

warn() {
    printf "${YELLOW}[WARN]${NC} %s\n" "$1"
}

error() {
    printf "${RED}[ERROR]${NC} %s\n" "$1"
    exit 1
}

show_help() {
    cat << 'EOF'
revoco uninstaller

Usage: uninstall.sh [OPTIONS]

Options:
  --yes     Skip all prompts and remove everything (binary, config, plugins, sessions, cache)
  --help    Show this help message

Examples:
  # Interactive uninstall (will prompt for each component)
  ./uninstall.sh

  # Non-interactive full removal
  ./uninstall.sh --yes

  # Via curl (interactive - requires TTY)
  curl -fsSL https://raw.githubusercontent.com/fulgidus/revoco/main/uninstall.sh | bash

  # Via curl (non-interactive full removal)
  curl -fsSL https://raw.githubusercontent.com/fulgidus/revoco/main/uninstall.sh | bash -s -- --yes

Components that may be removed:
  - Binary:       The revoco executable (location from install.json)
  - Shell config: PATH configuration added to your shell profile
  - Config:       ~/.config/revoco/ (config files)
  - Plugins:      ~/.config/revoco/plugins/ (installed plugins)
  - Sessions:     ~/.revoco/sessions/ (your work sessions - DATA LOSS WARNING)
  - Cache:        ~/.cache/revoco/ (cached binary tools like exiftool)

EOF
}

# Ask a yes/no question, return 0 for yes, 1 for no
# Reads from /dev/tty if stdin is not a terminal (e.g., when piped via curl)
ask() {
    prompt="$1"
    default="$2"  # "y" or "n"
    
    if [ "$SKIP_PROMPTS" = true ]; then
        return 0  # Always yes when --yes flag
    fi
    
    if [ "$default" = "y" ]; then
        prompt_suffix="[Y/n]"
    else
        prompt_suffix="[y/N]"
    fi
    
    printf "${BOLD}%s${NC} %s " "$prompt" "$prompt_suffix"
    
    # Read from /dev/tty if available (works when piped), otherwise stdin
    if [ ! -t 0 ] && [ -e /dev/tty ]; then
        read -r answer < /dev/tty
    else
        read -r answer
    fi
    
    case "$answer" in
        [Yy]|[Yy][Ee][Ss])
            return 0
            ;;
        [Nn]|[Nn][Oo])
            return 1
            ;;
        "")
            if [ "$default" = "y" ]; then
                return 0
            else
                return 1
            fi
            ;;
        *)
            return 1
            ;;
    esac
}

# Get binary path from install.json or return empty
get_binary_path() {
    if [ -f "$INSTALL_INFO" ]; then
        # Parse JSON manually (portable)
        grep '"binary_path"' "$INSTALL_INFO" 2>/dev/null | \
            sed 's/.*"binary_path"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' || true
    fi
}

# Get shell config file from install.json or return empty
get_shell_config() {
    if [ -f "$INSTALL_INFO" ]; then
        grep '"shell_config_file"' "$INSTALL_INFO" 2>/dev/null | \
            sed 's/.*"shell_config_file"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/' || true
    fi
}

# Remove PATH configuration from shell config file
# Returns 0 on success, 1 if not found or error
remove_path_config() {
    shell_config="$1"
    
    if [ -z "$shell_config" ] || [ ! -f "$shell_config" ]; then
        return 1
    fi
    
    # Check if our markers exist
    if ! grep -q "# Added by revoco installer" "$shell_config" 2>/dev/null; then
        return 1
    fi
    
    # Create backup
    cp "$shell_config" "${shell_config}.revoco-backup"
    
    # Remove lines between markers (inclusive) using sed
    # This handles the block: # Added by revoco installer ... # End revoco
    if sed -i.bak '/# Added by revoco installer/,/# End revoco/d' "$shell_config" 2>/dev/null; then
        rm -f "${shell_config}.bak"
        
        # Clean up trailing empty lines at end of file
        # Use a temp file approach for portability
        while [ "$(tail -c 1 "$shell_config" 2>/dev/null | wc -l)" -eq 0 ] && \
              [ "$(tail -n 1 "$shell_config" 2>/dev/null)" = "" ] && \
              [ "$(wc -l < "$shell_config")" -gt 0 ]; do
            # Remove last empty line
            sed -i.bak '$ { /^$/d }' "$shell_config" 2>/dev/null || break
            rm -f "${shell_config}.bak"
        done
        
        rm -f "${shell_config}.revoco-backup"
        return 0
    else
        # Restore backup on failure
        mv "${shell_config}.revoco-backup" "$shell_config"
        return 1
    fi
}

# Check if we can run interactively
# Returns true if stdin is a TTY, or if /dev/tty is available (piped via curl)
can_interact() {
    [ -t 0 ] || [ -e /dev/tty ]
}

# Calculate directory size
dir_size() {
    dir="$1"
    if [ -d "$dir" ]; then
        du -sh "$dir" 2>/dev/null | cut -f1 || echo "unknown"
    else
        echo "0"
    fi
}

# Count files in directory
file_count() {
    dir="$1"
    if [ -d "$dir" ]; then
        find "$dir" -type f 2>/dev/null | wc -l | tr -d ' '
    else
        echo "0"
    fi
}

# Parse arguments
parse_args() {
    while [ $# -gt 0 ]; do
        case "$1" in
            --yes|-y)
                SKIP_PROMPTS=true
                ;;
            --help|-h)
                SHOW_HELP=true
                ;;
            *)
                warn "Unknown option: $1"
                ;;
        esac
        shift
    done
}

# Main uninstallation
main() {
    parse_args "$@"
    
    if [ "$SHOW_HELP" = true ]; then
        show_help
        exit 0
    fi
    
    echo ""
    info "revoco uninstaller"
    echo ""
    
    # Check if we can interact with user
    if ! can_interact && [ "$SKIP_PROMPTS" = false ]; then
        error "Cannot read user input. Use --yes flag to proceed without prompts.

Example:
  curl -fsSL .../uninstall.sh | bash -s -- --yes"
    fi
    
    # Get binary path
    BINARY_PATH=$(get_binary_path)
    
    if [ -z "$BINARY_PATH" ]; then
        warn "No install.json found - revoco may have been installed manually"
        # Try to find it
        if command -v revoco >/dev/null 2>&1; then
            BINARY_PATH=$(command -v revoco)
            info "Found revoco at: $BINARY_PATH"
        else
            warn "Could not locate revoco binary"
        fi
    else
        info "Installation record found: $BINARY_PATH"
    fi
    
    # Get shell config file
    SHELL_CONFIG_FILE=$(get_shell_config)
    
    # Show what will be examined
    echo ""
    echo "Components found:"
    echo ""
    
    # Binary
    if [ -n "$BINARY_PATH" ] && [ -f "$BINARY_PATH" ]; then
        printf "  Binary:       %s\n" "$BINARY_PATH"
        HAVE_BINARY=true
    else
        printf "  Binary:       ${YELLOW}not found${NC}\n"
        HAVE_BINARY=false
    fi
    
    # Shell config
    if [ -n "$SHELL_CONFIG_FILE" ] && [ -f "$SHELL_CONFIG_FILE" ] && \
       grep -q "# Added by revoco installer" "$SHELL_CONFIG_FILE" 2>/dev/null; then
        printf "  Shell config: %s\n" "$SHELL_CONFIG_FILE"
        HAVE_SHELL_CONFIG=true
    else
        printf "  Shell config: ${YELLOW}not found${NC}\n"
        HAVE_SHELL_CONFIG=false
    fi
    
    # Config
    if [ -d "$CONFIG_DIR" ]; then
        config_size=$(dir_size "$CONFIG_DIR")
        printf "  Config:       %s (%s)\n" "$CONFIG_DIR" "$config_size"
        HAVE_CONFIG=true
    else
        printf "  Config:       ${YELLOW}not found${NC}\n"
        HAVE_CONFIG=false
    fi
    
    # Plugins (subdirectory of config)
    PLUGINS_DIR="$CONFIG_DIR/plugins"
    if [ -d "$PLUGINS_DIR" ]; then
        plugins_count=$(file_count "$PLUGINS_DIR")
        plugins_size=$(dir_size "$PLUGINS_DIR")
        printf "  Plugins:      %s (%s files, %s)\n" "$PLUGINS_DIR" "$plugins_count" "$plugins_size"
        HAVE_PLUGINS=true
    else
        printf "  Plugins:      ${YELLOW}not found${NC}\n"
        HAVE_PLUGINS=false
    fi
    
    # Sessions
    if [ -d "$SESSIONS_DIR" ]; then
        sessions_count=$(ls -1 "$SESSIONS_DIR" 2>/dev/null | wc -l | tr -d ' ')
        sessions_size=$(dir_size "$SESSIONS_DIR")
        printf "  Sessions:     %s (%s sessions, %s)\n" "$SESSIONS_DIR" "$sessions_count" "$sessions_size"
        HAVE_SESSIONS=true
    else
        printf "  Sessions:     ${YELLOW}not found${NC}\n"
        HAVE_SESSIONS=false
    fi
    
    # Cache
    if [ -d "$CACHE_DIR" ]; then
        cache_size=$(dir_size "$CACHE_DIR")
        printf "  Cache:        %s (%s)\n" "$CACHE_DIR" "$cache_size"
        HAVE_CACHE=true
    else
        printf "  Cache:        ${YELLOW}not found${NC}\n"
        HAVE_CACHE=false
    fi
    
    echo ""
    
    # Check if anything to remove
    if [ "$HAVE_BINARY" = false ] && [ "$HAVE_SHELL_CONFIG" = false ] && \
       [ "$HAVE_CONFIG" = false ] && [ "$HAVE_SESSIONS" = false ] && [ "$HAVE_CACHE" = false ]; then
        success "Nothing to uninstall - revoco is not installed"
        exit 0
    fi
    
    # Track what to remove
    REMOVE_BINARY=false
    REMOVE_SHELL_CONFIG=false
    REMOVE_CONFIG=false
    REMOVE_PLUGINS=false
    REMOVE_SESSIONS=false
    REMOVE_CACHE=false
    
    # Ask about each component
    if [ "$SKIP_PROMPTS" = true ]; then
        REMOVE_BINARY=$HAVE_BINARY
        REMOVE_SHELL_CONFIG=$HAVE_SHELL_CONFIG
        REMOVE_CONFIG=$HAVE_CONFIG
        REMOVE_PLUGINS=$HAVE_PLUGINS
        REMOVE_SESSIONS=$HAVE_SESSIONS
        REMOVE_CACHE=$HAVE_CACHE
    else
        echo "Select components to remove:"
        echo ""
        
        # Binary
        if [ "$HAVE_BINARY" = true ]; then
            if ask "Remove revoco binary?" "y"; then
                REMOVE_BINARY=true
            fi
        fi
        
        # Shell config
        if [ "$HAVE_SHELL_CONFIG" = true ]; then
            if ask "Remove PATH configuration from $SHELL_CONFIG_FILE?" "y"; then
                REMOVE_SHELL_CONFIG=true
            fi
        fi
        
        # Config (excluding plugins - ask separately)
        if [ "$HAVE_CONFIG" = true ]; then
            if ask "Remove configuration files?" "y"; then
                REMOVE_CONFIG=true
            fi
        fi
        
        # Plugins
        if [ "$HAVE_PLUGINS" = true ]; then
            if ask "Remove installed plugins?" "y"; then
                REMOVE_PLUGINS=true
            fi
        fi
        
        # Sessions - strong warning
        if [ "$HAVE_SESSIONS" = true ]; then
            echo ""
            printf "${RED}${BOLD}WARNING:${NC} Sessions contain your work data (imported archives, processed files, etc.)\n"
            printf "         This action ${RED}CANNOT be undone${NC}.\n"
            echo ""
            if ask "Remove ALL sessions and their data?" "n"; then
                REMOVE_SESSIONS=true
            fi
        fi
        
        # Cache
        if [ "$HAVE_CACHE" = true ]; then
            if ask "Remove cached tools (exiftool, etc.)?" "y"; then
                REMOVE_CACHE=true
            fi
        fi
    fi
    
    echo ""
    
    # Summary of what will be removed
    echo "Will remove:"
    [ "$REMOVE_BINARY" = true ] && echo "  - Binary: $BINARY_PATH"
    [ "$REMOVE_SHELL_CONFIG" = true ] && echo "  - Shell config: PATH entry in $SHELL_CONFIG_FILE"
    [ "$REMOVE_CONFIG" = true ] && echo "  - Config files"
    [ "$REMOVE_PLUGINS" = true ] && echo "  - Plugins"
    [ "$REMOVE_SESSIONS" = true ] && printf "  - ${RED}Sessions (DATA WILL BE LOST)${NC}\n"
    [ "$REMOVE_CACHE" = true ] && echo "  - Cache"
    
    if [ "$REMOVE_BINARY" = false ] && [ "$REMOVE_SHELL_CONFIG" = false ] && \
       [ "$REMOVE_CONFIG" = false ] && [ "$REMOVE_PLUGINS" = false ] && \
       [ "$REMOVE_SESSIONS" = false ] && [ "$REMOVE_CACHE" = false ]; then
        info "Nothing selected for removal"
        exit 0
    fi
    
    echo ""
    
    # Final confirmation (unless --yes)
    if [ "$SKIP_PROMPTS" = false ]; then
        if ! ask "Proceed with uninstallation?" "y"; then
            info "Uninstallation cancelled"
            exit 0
        fi
    fi
    
    echo ""
    info "Removing components..."
    echo ""
    
    # Perform removal
    ERRORS=0
    
    # Remove binary
    if [ "$REMOVE_BINARY" = true ]; then
        if rm -f "$BINARY_PATH" 2>/dev/null; then
            success "Removed binary: $BINARY_PATH"
        else
            warn "Failed to remove binary: $BINARY_PATH (may need sudo)"
            ERRORS=$((ERRORS + 1))
        fi
    fi
    
    # Remove shell config PATH entry
    if [ "$REMOVE_SHELL_CONFIG" = true ]; then
        if remove_path_config "$SHELL_CONFIG_FILE"; then
            success "Removed PATH config from: $SHELL_CONFIG_FILE"
            info "Restart your terminal for changes to take effect"
        else
            warn "Failed to remove PATH config from: $SHELL_CONFIG_FILE"
            ERRORS=$((ERRORS + 1))
        fi
    fi
    
    # Remove plugins (before config, as it's inside config dir)
    if [ "$REMOVE_PLUGINS" = true ]; then
        if rm -rf "$PLUGINS_DIR" 2>/dev/null; then
            success "Removed plugins: $PLUGINS_DIR"
        else
            warn "Failed to remove plugins: $PLUGINS_DIR"
            ERRORS=$((ERRORS + 1))
        fi
    fi
    
    # Remove config
    if [ "$REMOVE_CONFIG" = true ]; then
        # If not removing plugins, just remove config files
        if [ "$REMOVE_PLUGINS" = false ] && [ -d "$PLUGINS_DIR" ]; then
            # Remove only config files, keep plugins
            rm -f "$CONFIG_DIR/config.json" 2>/dev/null
            rm -f "$CONFIG_DIR/plugins.json" 2>/dev/null
            rm -f "$CONFIG_DIR/install.json" 2>/dev/null
            success "Removed config files (kept plugins)"
        else
            # Remove entire config directory
            if rm -rf "$CONFIG_DIR" 2>/dev/null; then
                success "Removed config: $CONFIG_DIR"
            else
                warn "Failed to remove config: $CONFIG_DIR"
                ERRORS=$((ERRORS + 1))
            fi
        fi
    fi
    
    # Remove sessions
    if [ "$REMOVE_SESSIONS" = true ]; then
        if rm -rf "$SESSIONS_DIR" 2>/dev/null; then
            success "Removed sessions: $SESSIONS_DIR"
            # Also remove parent .revoco if empty
            rmdir "$HOME/.revoco" 2>/dev/null || true
        else
            warn "Failed to remove sessions: $SESSIONS_DIR"
            ERRORS=$((ERRORS + 1))
        fi
    fi
    
    # Remove cache
    if [ "$REMOVE_CACHE" = true ]; then
        if rm -rf "$CACHE_DIR" 2>/dev/null; then
            success "Removed cache: $CACHE_DIR"
        else
            warn "Failed to remove cache: $CACHE_DIR"
            ERRORS=$((ERRORS + 1))
        fi
    fi
    
    echo ""
    
    # Final summary
    if [ $ERRORS -eq 0 ]; then
        success "Uninstallation complete!"
    else
        warn "Uninstallation completed with $ERRORS error(s)"
        echo ""
        echo "Some components may require manual removal or elevated privileges."
    fi
    
    # Show what remains
    echo ""
    REMAINING=""
    [ "$HAVE_BINARY" = true ] && [ "$REMOVE_BINARY" = false ] && REMAINING="$REMAINING binary"
    [ "$HAVE_SHELL_CONFIG" = true ] && [ "$REMOVE_SHELL_CONFIG" = false ] && REMAINING="$REMAINING shell-config"
    [ "$HAVE_CONFIG" = true ] && [ "$REMOVE_CONFIG" = false ] && REMAINING="$REMAINING config"
    [ "$HAVE_PLUGINS" = true ] && [ "$REMOVE_PLUGINS" = false ] && REMAINING="$REMAINING plugins"
    [ "$HAVE_SESSIONS" = true ] && [ "$REMOVE_SESSIONS" = false ] && REMAINING="$REMAINING sessions"
    [ "$HAVE_CACHE" = true ] && [ "$REMOVE_CACHE" = false ] && REMAINING="$REMAINING cache"
    
    if [ -n "$REMAINING" ]; then
        info "Kept:$REMAINING"
    fi
}

main "$@"
