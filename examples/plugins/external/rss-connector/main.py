#!/usr/bin/env python3
"""
RSS Feed Connector - External Plugin for revoco

This connector reads items from RSS and Atom feeds using the JSON-RPC protocol.
It demonstrates how to implement an external plugin in Python.

Communication:
- Reads JSON-RPC requests from stdin (one per line)
- Writes JSON-RPC responses to stdout (one per line)
- Logs and errors go to stderr
"""

import json
import sys
import hashlib
from datetime import datetime
from typing import Any, Optional

try:
    import feedparser
    import requests
except ImportError:
    print(
        "Error: Required packages not installed. Run: pip install feedparser requests",
        file=sys.stderr,
    )
    sys.exit(1)


class RSSConnector:
    """RSS/Atom feed connector implementation."""

    def __init__(self):
        self.config: dict = {}
        self.feed: Optional[feedparser.FeedParserDict] = None

    def initialize(self, config: dict) -> tuple[bool, Optional[str]]:
        """Initialize the connector with configuration."""
        self.config = config

        feed_url = config.get("feed_url", "")
        if not feed_url:
            return False, "feed_url is required"

        try:
            # Fetch and parse the feed
            self.feed = feedparser.parse(feed_url)

            if self.feed.bozo and self.feed.bozo_exception:
                return False, f"Feed parse error: {self.feed.bozo_exception}"

            log(f"Initialized with feed: {self.feed.feed.get('title', feed_url)}")
            return True, None

        except Exception as e:
            return False, f"Failed to fetch feed: {str(e)}"

    def test_connection(self, config: dict) -> tuple[bool, Optional[str]]:
        """Test if the feed is accessible."""
        feed_url = config.get("feed_url", "")
        if not feed_url:
            return False, "feed_url is required"

        try:
            response = requests.head(feed_url, timeout=10, allow_redirects=True)
            if response.status_code == 200:
                return True, f"Feed accessible: {feed_url}"
            else:
                return False, f"HTTP {response.status_code}"
        except requests.RequestException as e:
            return False, str(e)

    def list(self) -> tuple[list[dict], Optional[str]]:
        """List all items from the feed."""
        if not self.feed:
            return [], "Connector not initialized"

        items = []
        max_items = self.config.get("max_items", 100)
        include_content = self.config.get("include_content", True)

        entries = self.feed.entries
        if max_items > 0:
            entries = entries[:max_items]

        for entry in entries:
            # Generate a stable ID from the entry
            entry_id = entry.get("id") or entry.get("link") or entry.get("title", "")
            item_id = f"rss:{hashlib.md5(entry_id.encode()).hexdigest()}"

            # Parse dates
            published = None
            if hasattr(entry, "published_parsed") and entry.published_parsed:
                published = datetime(*entry.published_parsed[:6]).isoformat()
            elif hasattr(entry, "updated_parsed") and entry.updated_parsed:
                published = datetime(*entry.updated_parsed[:6]).isoformat()

            # Build metadata
            metadata = {
                "title": entry.get("title", ""),
                "link": entry.get("link", ""),
                "author": entry.get("author", ""),
                "published": published,
                "feed_title": self.feed.feed.get("title", ""),
                "feed_link": self.feed.feed.get("link", ""),
            }

            # Include content if requested
            if include_content:
                content = ""
                if hasattr(entry, "content") and entry.content:
                    content = entry.content[0].get("value", "")
                elif hasattr(entry, "summary"):
                    content = entry.summary
                metadata["content"] = content
                metadata["content_type"] = "html"

            # Include categories/tags
            if hasattr(entry, "tags"):
                metadata["tags"] = [tag.get("term", "") for tag in entry.tags]

            # Include enclosures (media attachments)
            if hasattr(entry, "enclosures") and entry.enclosures:
                metadata["enclosures"] = [
                    {
                        "url": enc.get("href", ""),
                        "type": enc.get("type", ""),
                        "length": enc.get("length", 0),
                    }
                    for enc in entry.enclosures
                ]

            item = {
                "id": item_id,
                "name": entry.get("title", "Untitled"),
                "type": "document",
                "path": entry.get("link", ""),
                "source_path": entry.get("link", ""),
                "metadata": metadata,
            }

            items.append(item)

        log(f"Listed {len(items)} items from feed")
        return items, None

    def read(self, item: dict) -> tuple[Optional[str], Optional[str]]:
        """Read the content of a single item."""
        # For RSS items, we return the content/summary as the "file" content
        metadata = item.get("metadata", {})

        content = metadata.get("content", "")
        if not content:
            content = metadata.get("title", "")

        return content, None

    def close(self) -> tuple[bool, Optional[str]]:
        """Clean up resources."""
        self.feed = None
        self.config = {}
        log("Connector closed")
        return True, None


# ═══════════════════════════════════════════════════════════════════════════════
# JSON-RPC Protocol Implementation
# ═══════════════════════════════════════════════════════════════════════════════

connector = RSSConnector()


def log(message: str):
    """Log a message to stderr."""
    print(f"[rss-connector] {message}", file=sys.stderr)


def handle_request(request: dict) -> dict:
    """Handle a single JSON-RPC request."""
    method = request.get("method", "")
    params = request.get("params", {})
    request_id = request.get("id")

    try:
        result = dispatch_method(method, params)
        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "result": result,
        }
    except Exception as e:
        return {
            "jsonrpc": "2.0",
            "id": request_id,
            "error": {
                "code": -32000,
                "message": str(e),
            },
        }


def dispatch_method(method: str, params: dict) -> Any:
    """Dispatch a method call to the appropriate handler."""

    if method == "Initialize":
        config = params.get("config", {})
        ok, err = connector.initialize(config)
        return {"success": ok, "error": err}

    elif method == "TestConnection":
        config = params.get("config", {})
        ok, msg = connector.test_connection(config)
        return {"success": ok, "message": msg}

    elif method == "List":
        items, err = connector.list()
        return {"items": items, "error": err}

    elif method == "Read":
        item = params.get("item", {})
        content, err = connector.read(item)
        return {"content": content, "error": err}

    elif method == "Close":
        ok, err = connector.close()
        return {"success": ok, "error": err}

    elif method == "Ping":
        return {"pong": True}

    else:
        raise ValueError(f"Unknown method: {method}")


def main():
    """Main entry point - reads JSON-RPC requests from stdin."""
    log("Starting RSS connector...")

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        try:
            request = json.loads(line)
            response = handle_request(request)
            print(json.dumps(response), flush=True)
        except json.JSONDecodeError as e:
            error_response = {
                "jsonrpc": "2.0",
                "id": None,
                "error": {
                    "code": -32700,
                    "message": f"Parse error: {str(e)}",
                },
            }
            print(json.dumps(error_response), flush=True)
        except Exception as e:
            error_response = {
                "jsonrpc": "2.0",
                "id": None,
                "error": {
                    "code": -32603,
                    "message": f"Internal error: {str(e)}",
                },
            }
            print(json.dumps(error_response), flush=True)


if __name__ == "__main__":
    main()
