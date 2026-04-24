# 🎯 GoManager v3 — Advanced Go Package Management Suite

**GoManager** is a professional-grade CLI tool for managing Go packages with **zero friction**. Delete, search, analyze, and optimize your GOPATH with an intuitive interactive TUI. Built for developers who want full control over their Go environment.

---

## ✨ Premium Features

### 🔍 **Smart Package Discovery**
- Find all installed packages recursively from your `GOPATH`
- Parallel scanning for blazing-fast initialization
- Package size calculation (byte-level accuracy)
- Modification time tracking

### 🎨 **Interactive Browser (Best-in-Class TUI)**
- Beautiful terminal UI with color-coded output
- Live search filtering (type to filter in real-time)
- Multi-select with visual feedback
- Instant package size display
- Full keyboard navigation

### 📊 **Advanced Statistics & Insights**
- **Total packages & GOPATH size** at a glance
- **Largest packages** (identify space hogs)
- **Oldest packages** (find stale dependencies)
- Freed space calculation on delete
- Deletion history logging (JSON format)

### ☑️ **Bulk Operations**
- Select multiple packages → delete all at once
- Select all filtered packages with one keypress
- Real-time freed space preview
- Undo-safe deletion log (`~/.gomanager/deletion_log.json`)

### 🗑️ **Intelligent Deletion**
- Atomic deletion with rollback on error
- Auto cache cleanup (`go clean -cache`)
- Freed space reporting
- Deletion logging with timestamps

### 💾 **Auto Cache Management**
- Automatic Go build cache cleanup
- Optional manual cache clearing
- Space reclamation on each delete operation

### 🎨 **Beautiful CLI**
- Color-coded output (green=success, red=error, yellow=warnings)
- Unicode icons (✅, ❌, 🎉, 💾)
- Progressive status updates
- Real-time filtering with match counts

### 🔧 **CLI Flags for Automation**
- Silent mode (`-nc` skip confirmations)
- Specific package deletion (`-pkg "name"`)
- Package installation (`-pkgi "name"`)
- Cache control (`-ncl` skip cleanup)

### 📝 **Deletion Logging**
- JSON-formatted deletion history
- Timestamp + freed space tracking
- Perfect for audits & recovery
- Located at: `~/.gomanager/deletion_log.json`

### ⚡ **Performance**
- Parallel directory scanning with goroutines
- Fast size calculation
- Asynchronous operations (non-blocking)
- Responsive TUI with real-time updates

---

## 📦 Installation

### Prerequisites
- Go 1.20 or later
- Git

### Build from Source

```bash
git clone https://github.com/kathuluman/gomanager.git
cd gomanager
go build -o gomanager .
```

### Install with `go install`

```bash
go install github.com/kathuluman/gomanager@latest
```

---

## 🚀 Usage

### Default Mode (Interactive Browser - Recommended!)

```bash
./gomanager
```

This opens the **interactive browser** — the best way to manage packages:

```
Installed Go Packages (47 total) | Status: Not selected | ⎵=Select a=SelectAll c=Clear s=Stats d=Log ↵=Delete

›  github.com/charmbracelet/bubbletea [2.4MB]
   /home/user/go/pkg/mod/github.com/charmbracelet/bubbletea@v0.25.0
   
   github.com/spf13/cobra [1.8MB]
   /home/user/go/pkg/mod/github.com/spf13/cobra@v1.8.0
```

#### Interactive Browser Keybinds

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate packages |
| `Space` | Toggle selection on current package |
| `a` | **Select All** visible packages |
| `c` | **Clear** all selections |
| `s` | **Show GOPATH Statistics** (size, largest, oldest) |
| `d` | **View deletion log** (see what you deleted) |
| `Enter` | **Delete selected** + auto-clean cache |
| `Type` | **Live search** — filter packages by name |
| `Backspace` | **Clear filter** |
| `Esc` / `Ctrl+C` | **Quit** |

#### Example Workflow: Delete all "deprecated" packages

```bash
./gomanager
# Type: deprecated
# Press 'a' to select all matches
# Press Enter to delete
# ✅ Done! Shows freed space + cache cleaned
```

---

### View GOPATH Statistics

While in interactive mode, press `s`:

```
=== GOPATH Statistics ===
Total Packages: 47
Total Size: 245.3MB
Largest Package: github.com/some/huge-package (42.1MB)
Oldest Package: github.com/deprecated/lib (modified: 2023-08-15)
================================
```

---

### View Deletion History

Press `d` to see your deletion log:

```json
[
  {
    "timestamp": "2026-04-24T05:45:32Z",
    "packages": ["github.com/old/pkg1", "github.com/old/pkg2"],
    "count": 2,
    "freed_space_bytes": 125439842,
    "cache_cleaned": true
  }
]
```

Located at: `~/.gomanager/deletion_log.json`

---

### Install a Package

Search and install from pkg.go.dev:

```bash
./gomanager -pkgi "encoding/json"
```

- Query pkg.go.dev for matches
- Interactive list to pick from
- Auto-install with `go get`
- Success/error feedback

---

### Delete Specific Package (CLI Mode)

```bash
./gomanager -pkg github.com/old/package
```

- Search for matching directories
- Show matches with indices
- Interactive selection
- Delete with confirmation

---

## 🎨 Command-Line Flags

| Flag | Description | Example |
|------|-------------|---------|
| `-i` | Interactive mode (default) | `./gomanager -i` |
| `-pkg "name"` | Delete specific package | `./gomanager -pkg "deprecated"` |
| `-pkgi "name"` | Install package | `./gomanager -pkgi "encoding/json"` |
| `-nc` | Skip confirmations | `./gomanager -pkg "pkg" -nc` |
| `-ncl` | Skip cache cleanup | `./gomanager -i -ncl` |

---

## 📋 Real-World Examples

### Scenario 1: Clean up all outdated "v1" packages

```bash
./gomanager
# Type: v1
# Shows only v1 packages
# Press 'a' to select all
# Press Enter to delete all
# ✅ Done! Freed 152MB + cache cleaned
```

### Scenario 2: Check what's taking up space

```bash
./gomanager
# Press 's' to show statistics
# See largest & oldest packages
# Target them for removal
```

### Scenario 3: Selectively remove packages

```bash
./gomanager
# Type: github.com/user/
# Press Space on each package to toggle
# Press Enter when done selecting
# ✅ Custom selection deleted
```

### Scenario 4: Automate deletion (CI/CD safe)

```bash
./gomanager -pkg "unused-package" -nc
# No confirmation prompts
# Perfect for automation scripts
```

### Scenario 5: Check your deletion history

```bash
./gomanager
# Press 'd' to view deletion log
# See exactly what was deleted & when
# Track freed space over time
```

---

## 🎨 Color Guide

GoManager uses ANSI colors for clarity:

| Color | Meaning |
|-------|---------|
| 🟢 **Green** | Success, progress, selections |
| 🔴 **Red** | Errors, warnings, package names |
| 🟡 **Yellow** | Indices, notes, statistics |
| 🟣 **Magenta** | Paths, package details, sizes |
| 🔵 **Cyan** | Headers, titles, UI elements |
| ⚪ **White** | General output, text |

---

## 🏗️ Advanced Features

### Parallel Package Scanning

GoManager uses goroutines to scan packages in parallel:

```go
// Concurrent directory walking
// Size calculation in parallel
// Instant results even with 1000+ packages
```

### Size Calculation

Accurate byte-level size for each package:

```
github.com/charmbracelet/bubbletea [2.4MB]
github.com/spf13/cobra [1.8MB]
...
```

### Deletion Logging

Every deletion is logged to `~/.gomanager/deletion_log.json`:

```json
{
  "timestamp": "2026-04-24T05:45:32Z",
  "packages": ["pkg1", "pkg2"],
  "count": 2,
  "freed_space_bytes": 125439842,
  "cache_cleaned": true
}
```

Perfect for:
- Audits
- Recovery (see what was deleted)
- Space optimization tracking
- CI/CD integration

---

## 🐛 Troubleshooting

### "No packages found"
```bash
ls ~/go/pkg
# Check that packages exist
```

### Interactive browser empty
```bash
go env GOPATH
# Verify GOPATH is set correctly
```

### Cache cleanup fails
```bash
which go
go version
# Ensure 'go' is in PATH
```

### Size shows as 0
Rare edge case - packages may have unusual permissions. Try:
```bash
chmod -R u+r ~/go/pkg
```

---

## 🛣️ Roadmap

- [ ] Package dependency graph visualization
- [ ] Undo deleted packages (trash instead of delete)
- [ ] Configuration file (`~/.gomanager/config.json`)
- [ ] Module graph analysis (`go mod graph` integration)
- [ ] Automated cleanup scheduler (cron)
- [ ] Duplicate package detection
- [ ] Module version comparison

---

## 📄 License

MIT License — Use freely, modify, distribute. See LICENSE file.




