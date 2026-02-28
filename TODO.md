# Revoco Roadmap

> Your escape pod from walled gardens and big tech lock-in.

---

## Legend

- [ ] Not started
- [x] Completed
- [~] In progress
- [-] Blocked/Deferred

---

## Core Features

### Session Management
- [x] Create/delete sessions
- [x] V1 to V2 migration
- [x] Connector-based architecture
- [ ] Session templates (quick-start presets)
- [ ] Session export/import (share configurations)
- [ ] Scheduled/automated runs

### Processing Pipeline
- [x] Parallel data retrieval
- [x] Cancellable operations (mid-step)
- [x] Progress tracking with live UI
- [ ] Resume interrupted operations
- [ ] Incremental sync (only changed files)
- [ ] Conflict resolution strategies (configurable)
- [ ] Dry-run mode for all operations

### Data Management
- [x] Deduplication (hash-based)
- [x] Metadata preservation (EXIF, etc.)
- [ ] Data validation/integrity checks
- [ ] Encryption at rest
- [ ] Compression options
- [ ] File filtering (by type, date, size)

---

## Connectors

### Local Storage
- [x] Folder
- [x] ZIP archive (single & multi)
- [x] TGZ archive (single & multi)
- [ ] RAR archive
- [ ] 7z archive
- [ ] ISO image

### Cloud Storage
| Service | Input | Output | Status |
|---------|:-----:|:------:|--------|
| Google Drive | [x] | [ ] | **input ready** |
| OneDrive | [ ] | [ ] | planned |
| iCloud Drive | [ ] | [ ] | planned |
| Dropbox | [ ] | [ ] | planned |
| Box | [ ] | [ ] | planned |
| Nextcloud/WebDAV | [ ] | [ ] | planned |
| S3-compatible | [ ] | [ ] | planned |
| Backblaze B2 | [ ] | [ ] | planned |
| MEGA | [ ] | [ ] | planned |

### Remote/Network
| Protocol | Input | Output | Status |
|----------|:-----:|:------:|--------|
| FTP | [ ] | [ ] | planned |
| SFTP | [ ] | [ ] | planned |
| SMB/CIFS | [ ] | [ ] | planned |
| NFS | [ ] | [ ] | planned |
| rclone (universal) | [ ] | [ ] | planned |

---

## Service Processors

### Photos & Media
| Service | Import | Export | Metadata | Status |
|---------|:------:|:------:|:--------:|--------|
| Google Photos | [x] | [ ] | [x] | active |
| iCloud Photos | [ ] | [ ] | [ ] | planned |
| Amazon Photos | [ ] | [ ] | [ ] | planned |
| Flickr | [ ] | [ ] | [ ] | planned |
| SmugMug | [ ] | [ ] | [ ] | planned |
| Immich | [ ] | [ ] | [ ] | planned |
| PhotoPrism | [ ] | [ ] | [ ] | planned |

### Music & Audio
| Service | Import | Export | Playlists | Status |
|---------|:------:|:------:|:---------:|--------|
| YouTube Music | [~] | [ ] | [ ] | active |
| Spotify | [ ] | [ ] | [ ] | planned |
| Apple Music | [ ] | [ ] | [ ] | planned |
| Amazon Music | [ ] | [ ] | [ ] | planned |
| Deezer | [ ] | [ ] | [ ] | planned |
| Tidal | [ ] | [ ] | [ ] | planned |
| Bandcamp | [ ] | [ ] | [ ] | planned |
| SoundCloud | [ ] | [ ] | [ ] | planned |
| Navidrome | [ ] | [ ] | [ ] | planned |
| Jellyfin | [ ] | [ ] | [ ] | planned |

### Video
| Service | Import | Export | Metadata | Status |
|---------|:------:|:------:|:--------:|--------|
| YouTube | [ ] | [ ] | [ ] | planned |
| Vimeo | [ ] | [ ] | [ ] | planned |
| Twitch | [ ] | [ ] | [ ] | planned |

---

## Email Connectors

### Import/Export
| Provider | Import | Export | Settings | Status |
|----------|:------:|:------:|:--------:|--------|
| Gmail | [ ] | [ ] | [ ] | planned |
| Proton Mail | [ ] | [ ] | [ ] | planned |
| Fastmail | [ ] | [ ] | [ ] | planned |
| Tuta (Tutanota) | [ ] | [ ] | [ ] | planned |
| Mailbox.org | [ ] | [ ] | [ ] | planned |
| Outlook/Hotmail | [ ] | [ ] | [ ] | planned |
| iCloud Mail | [ ] | [ ] | [ ] | planned |
| Yahoo Mail | [ ] | [ ] | [ ] | planned |

### Protocols
| Protocol | Input | Output | Status |
|----------|:-----:|:------:|--------|
| IMAP | [ ] | [ ] | planned |
| POP3 | [ ] | - | planned |
| SMTP relay | - | [ ] | planned |
| JMAP | [ ] | [ ] | planned |
| mbox format | [ ] | [ ] | planned |
| EML files | [ ] | [ ] | planned |
| Maildir | [ ] | [ ] | planned |

---

## Documents & Office Suites

> **Critical for data sovereignty**: Cloud-only documents (Google Docs, Office Online) 
> don't sync as real files - losing account access means losing everything!

### Cloud Office Suites
| Service | Import | Export | Versions | Comments | Status |
|---------|:------:|:------:|:--------:|:--------:|--------|
| Google Docs | [x] | [ ] | [ ] | [ ] | **export via Drive** |
| Google Sheets | [x] | [ ] | [ ] | [ ] | **export via Drive** |
| Google Slides | [x] | [ ] | [ ] | [ ] | **export via Drive** |
| Google Drawings | [x] | [ ] | [ ] | [ ] | **export via Drive** |
| Microsoft Word Online | [ ] | [ ] | [ ] | [ ] | planned |
| Microsoft Excel Online | [ ] | [ ] | [ ] | [ ] | planned |
| Microsoft PowerPoint Online | [ ] | [ ] | [ ] | [ ] | planned |
| Apple Pages (iCloud) | [ ] | [ ] | [ ] | [ ] | planned |
| Apple Numbers (iCloud) | [ ] | [ ] | [ ] | [ ] | planned |
| Apple Keynote (iCloud) | [ ] | [ ] | [ ] | [ ] | planned |
| Dropbox Paper | [ ] | [ ] | [ ] | [ ] | planned |
| Zoho Docs | [ ] | [ ] | [ ] | [ ] | planned |
| Notion | [ ] | [ ] | [ ] | [ ] | planned |
| Coda | [ ] | [ ] | [ ] | [ ] | planned |
| Airtable | [ ] | [ ] | [ ] | [ ] | planned |
| Quip | [ ] | [ ] | [ ] | [ ] | planned |

### Export Formats (per document type)

**Word Processing:**
- [ ] DOCX (Microsoft Word)
- [ ] ODT (OpenDocument)
- [ ] PDF
- [ ] Markdown
- [ ] HTML
- [ ] Plain Text
- [ ] RTF
- [ ] EPUB

**Spreadsheets:**
- [ ] XLSX (Microsoft Excel)
- [ ] ODS (OpenDocument)
- [ ] CSV
- [ ] TSV
- [ ] PDF
- [ ] JSON

**Presentations:**
- [ ] PPTX (Microsoft PowerPoint)
- [ ] ODP (OpenDocument)
- [ ] PDF
- [ ] Images (PNG per slide)
- [ ] HTML

**Diagrams/Drawings:**
- [ ] SVG
- [ ] PNG
- [ ] PDF

### Document Features
- [ ] Preserve folder/directory structure
- [ ] Export revision history
- [ ] Export comments & suggestions
- [ ] Export sharing permissions (as metadata)
- [ ] Batch export (entire Drive/folder)
- [ ] Incremental sync (changed docs only)
- [ ] Automatic format conversion on change
- [ ] Scheduled backup to local/NAS

### Self-Hosted Alternatives (Output Targets)
| Service | Documents | Spreadsheets | Presentations | Status |
|---------|:---------:|:------------:|:-------------:|--------|
| Nextcloud + Collabora | [ ] | [ ] | [ ] | planned |
| Nextcloud + OnlyOffice | [ ] | [ ] | [ ] | planned |
| Synology Office | [ ] | [ ] | [ ] | planned |
| CryptPad | [ ] | [ ] | [ ] | planned |
| ONLYOFFICE Workspace | [ ] | [ ] | [ ] | planned |

---

## Social & Messaging

### Social Networks
| Service | Posts | Media | Contacts | Status |
|---------|:-----:|:-----:|:--------:|--------|
| Facebook | [ ] | [ ] | [ ] | planned |
| Instagram | [ ] | [ ] | [ ] | planned |
| Twitter/X | [ ] | [ ] | [ ] | planned |
| LinkedIn | [ ] | [ ] | [ ] | planned |
| TikTok | [ ] | [ ] | [ ] | planned |
| Reddit | [ ] | [ ] | [ ] | planned |
| Bluesky | [ ] | [ ] | [ ] | planned |
| Mastodon | [ ] | [ ] | [ ] | planned |
| Threads | [ ] | [ ] | [ ] | planned |

### Messaging
| Service | Messages | Media | Status |
|---------|:--------:|:-----:|--------|
| WhatsApp | [ ] | [ ] | planned |
| Telegram | [ ] | [ ] | planned |
| Signal | [ ] | [ ] | planned |
| Facebook Messenger | [ ] | [ ] | planned |
| Discord | [ ] | [ ] | planned |
| Slack | [ ] | [ ] | planned |
| iMessage | [ ] | [ ] | planned |
| Matrix | [ ] | [ ] | planned |

---

## PIM (Personal Information Management)

### Contacts
| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| Google Contacts | [ ] | [ ] | planned |
| iCloud Contacts | [ ] | [ ] | planned |
| Outlook Contacts | [ ] | [ ] | planned |
| CardDAV | [ ] | [ ] | planned |
| vCard (.vcf) | [ ] | [ ] | planned |

### Calendar
| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| Google Calendar | [ ] | [ ] | planned |
| iCloud Calendar | [ ] | [ ] | planned |
| Outlook Calendar | [ ] | [ ] | planned |
| CalDAV | [ ] | [ ] | planned |
| iCal (.ics) | [ ] | [ ] | planned |

### Notes & Documents
| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| Google Keep | [ ] | [ ] | planned |
| Apple Notes | [ ] | [ ] | planned |
| Notion | [ ] | [ ] | planned |
| Evernote | [ ] | [ ] | planned |
| OneNote | [ ] | [ ] | planned |
| Obsidian | [ ] | [ ] | planned |
| Standard Notes | [ ] | [ ] | planned |

### Tasks/Reminders
| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| Google Tasks | [ ] | [ ] | planned |
| Apple Reminders | [ ] | [ ] | planned |
| Todoist | [ ] | [ ] | planned |
| Trello | [ ] | [ ] | planned |

---

## Security & Identity

### Passwords
| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| Google Passwords | [ ] | [ ] | planned |
| iCloud Keychain | [ ] | [ ] | planned |
| 1Password | [ ] | [ ] | planned |
| Bitwarden | [ ] | [ ] | planned |
| LastPass | [ ] | [ ] | planned |
| KeePass (.kdbx) | [ ] | [ ] | planned |
| Dashlane | [ ] | [ ] | planned |

### Browser Data
| Browser | Bookmarks | History | Passwords | Status |
|---------|:---------:|:-------:|:---------:|--------|
| Chrome | [ ] | [ ] | [ ] | planned |
| Firefox | [ ] | [ ] | [ ] | planned |
| Safari | [ ] | [ ] | [ ] | planned |
| Edge | [ ] | [ ] | [ ] | planned |
| Brave | [ ] | [ ] | [ ] | planned |

### 2FA/Authenticator
| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| Google Authenticator | [ ] | [ ] | planned |
| Authy | [ ] | [ ] | planned |
| TOTP (otpauth://) | [ ] | [ ] | planned |

---

## Health & Fitness

| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| Google Fit | [ ] | [ ] | planned |
| Apple Health | [ ] | [ ] | planned |
| Fitbit | [ ] | [ ] | planned |
| Garmin Connect | [ ] | [ ] | planned |
| Strava | [ ] | [ ] | planned |
| Samsung Health | [ ] | [ ] | planned |

---

## Reading & Learning

| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| Kindle (highlights/notes) | [ ] | [ ] | planned |
| Goodreads | [ ] | [ ] | planned |
| Pocket | [ ] | [ ] | planned |
| Instapaper | [ ] | [ ] | planned |
| Audible | [ ] | [ ] | planned |

---

## Gaming

| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| Steam | [ ] | [ ] | planned |
| GOG | [ ] | [ ] | planned |
| Epic Games | [ ] | [ ] | planned |
| PlayStation Network | [ ] | [ ] | planned |
| Xbox/Microsoft | [ ] | [ ] | planned |
| Nintendo | [ ] | [ ] | planned |

---

## Developer Tools

| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| GitHub | [ ] | [ ] | planned |
| GitLab | [ ] | [ ] | planned |
| Bitbucket | [ ] | [ ] | planned |
| Gists/Snippets | [ ] | [ ] | planned |

---

## Smart Home & IoT

| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| Google Home | [ ] | [ ] | planned |
| Amazon Alexa | [ ] | [ ] | planned |
| Apple HomeKit | [ ] | [ ] | planned |
| Home Assistant | [ ] | [ ] | planned |

---

## Location & Maps

| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| Google Maps (timeline) | [ ] | [ ] | planned |
| Google Maps (saved places) | [ ] | [ ] | planned |
| Apple Maps | [ ] | [ ] | planned |
| GPX files | [ ] | [ ] | planned |
| KML/KMZ files | [ ] | [ ] | planned |

---

## Financial

| Service | Import | Export | Status |
|---------|:------:|:------:|--------|
| Google Pay | [ ] | [ ] | planned |
| Apple Pay | [ ] | [ ] | planned |
| PayPal | [ ] | [ ] | planned |
| Mint | [ ] | [ ] | planned |
| YNAB | [ ] | [ ] | planned |

---

## Universal Formats (Priority)

These should be implemented early as they enable interoperability:

- [ ] **CardDAV/CalDAV** - Universal contacts/calendar sync
- [ ] **WebDAV** - Universal file storage
- [ ] **IMAP** - Universal email access
- [ ] **RSS/OPML** - Feed subscriptions
- [ ] **vCard/iCal** - Standard PIM formats
- [ ] **MBOX/EML** - Email archives
- [ ] **GPX/KML** - Location data
- [ ] **CSV/JSON** - Generic data export
- [ ] **Office Open XML** - DOCX/XLSX/PPTX
- [ ] **OpenDocument** - ODT/ODS/ODP

---

## NAS & Self-Hosted Integration

> Common pain point: Cloud-native documents (Google Docs, Office Online) 
> are NOT real files and don't sync to NAS devices!

### Synology
- [ ] Synology Drive integration
- [ ] Synology Office import
- [ ] Hyper Backup integration
- [ ] Active Backup for Google Workspace

### Other NAS
- [ ] QNAP integration
- [ ] TrueNAS integration
- [ ] Unraid integration
- [ ] Generic SMB/NFS share

### Backup Strategies
- [ ] Scheduled document export (cron-style)
- [ ] Watch for changes (webhook/polling)
- [ ] Versioned backups (keep N versions)
- [ ] Deduplication across backups
- [ ] Compression (zip/tar.gz per export)

---

## Architecture Goals

- [ ] Plugin system for community connectors
- [ ] OAuth2 flow for authenticated services
- [ ] API key management
- [ ] Rate limiting & retry logic
- [ ] Webhook support for real-time sync
- [ ] CLI mode for automation/scripting
- [ ] Headless/daemon mode
- [ ] Docker deployment
- [ ] Multi-platform builds (Linux, macOS, Windows)

---

## Contributing

Want to help? Pick an unchecked item and submit a PR!

**Priority areas:**
1. **Google Docs/Sheets/Slides export** - Most requested, high lock-in risk
2. Universal protocols (IMAP, WebDAV, CardDAV)
3. Popular services with data export (Google, Apple, Meta)
4. Self-hosted alternatives (Nextcloud, Immich, Navidrome)
5. NAS integration (Synology, QNAP)

**High-impact quick wins:**
- Office format converters (DOCX, XLSX, PPTX)
- OpenDocument support (ODT, ODS, ODP)
- PDF export for any document type
- Markdown export for notes/docs
