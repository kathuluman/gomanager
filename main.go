package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/briandowns/spinner"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/net/html"
)

var Reset = "\033[0m"
var Red = "\033[31m"
var Green = "\033[32m"
var Yellow = "\033[33m"
var Blue = "\033[34m"
var Magenta = "\033[35m"
var Cyan = "\033[36m"
var Gray = "\033[37m"
var White = "\033[97m"
var Bold = "\033[1m"
var BgGreen = "\033[42m"
var BgRed = "\033[41m"

type PackageInfo struct {
	Name      string
	URL       string
	Path      string
	Selected  bool
	Size      int64
	ModTime   time.Time
	DepCount  int
}

type DeletionLog struct {
	Timestamp   string   `json:"timestamp"`
	Packages    []string `json:"packages"`
	Count       int      `json:"count"`
	FreedSpace  int64    `json:"freed_space_bytes"`
	CacheCleaned bool    `json:"cache_cleaned"`
}

type StatsInfo struct {
	TotalPackages int64
	TotalSize     int64
	LargestPkg    string
	LargestSize   int64
	OldestPkg     string
	OldestTime    time.Time
}

func (p PackageInfo) Title() string {
	size := formatSize(p.Size)
	if p.Size == 0 {
		return fmt.Sprintf("%s [unknown size]", p.Name)
	}
	return fmt.Sprintf("%s %s[%s]%s", p.Name, Magenta, size, Reset)
}

func (p PackageInfo) Description() string {
	mtime := p.ModTime.Format("2006-01-02 15:04")
	if p.ModTime.IsZero() {
		mtime = "unknown"
	}
	return fmt.Sprintf("%s | modified: %s", p.Path, mtime)
}

func (p PackageInfo) FilterValue() string { return p.Name }

type installMsg struct {
	PackageName string
	Success     bool
	Error       error
}

type deleteMsg struct {
	Success bool
	Error   error
}

type model struct {
	list          list.Model
	choices       []PackageInfo
	selected      map[int]bool
	filterText    string
	filteredIdx   []int
	loading       bool
	done          bool
	err           error
	mode          string
	selectedCount int
}

var matches []string
var deletionLogPath string
var configPath string

func init() {
	usr, err := user.Current()
	if err == nil {
		configPath = filepath.Join(usr.HomeDir, ".gomanager")
		deletionLogPath = filepath.Join(configPath, "deletion_log.json")
		os.MkdirAll(configPath, 0755)
	}
}

func formatSize(bytes int64) string {
	if bytes == 0 {
		return "0B"
	}
	sizes := []string{"B", "KB", "MB", "GB"}
	size := float64(bytes)
	idx := 0
	for size >= 1024 && idx < len(sizes)-1 {
		size /= 1024
		idx++
	}
	if idx == 0 {
		return fmt.Sprintf("%d%s", int64(size), sizes[idx])
	}
	return fmt.Sprintf("%.2f%s", size, sizes[idx])
}

func getDirSize(path string) int64 {
	var size int64
	filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if info, err := d.Info(); err == nil {
				size += info.Size()
			}
		}
		return nil
	})
	return size
}

func logDeletion(packages []string, freedSpace int64, cacheCleaned bool) {
	if deletionLogPath == "" {
		return
	}
	log := DeletionLog{
		Timestamp:    time.Now().Format(time.RFC3339),
		Packages:     packages,
		Count:        len(packages),
		FreedSpace:   freedSpace,
		CacheCleaned: cacheCleaned,
	}
	var logs []DeletionLog
	if data, err := ioutil.ReadFile(deletionLogPath); err == nil {
		json.Unmarshal(data, &logs)
	}
	logs = append(logs, log)
	if data, err := json.MarshalIndent(logs, "", "  "); err == nil {
		ioutil.WriteFile(deletionLogPath, data, 0644)
	}
}

func calculateStats(packages []PackageInfo) StatsInfo {
	stats := StatsInfo{}
	var totalSize int64
	var largestSize int64
	var oldestTime time.Time

	for _, pkg := range packages {
		stats.TotalPackages++
		totalSize += pkg.Size
		if pkg.Size > largestSize {
			largestSize = pkg.Size
			stats.LargestPkg = pkg.Name
			stats.LargestSize = pkg.Size
		}
		if oldestTime.IsZero() || pkg.ModTime.Before(oldestTime) {
			oldestTime = pkg.ModTime
			stats.OldestPkg = pkg.Name
			stats.OldestTime = pkg.ModTime
		}
	}
	stats.TotalSize = totalSize
	return stats
}

func fetchSearchResults(query string) ([]PackageInfo, error) {
	url := "https://pkg.go.dev/search?q=" + query

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GoScraper/1.0")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	z := html.NewTokenizer(resp.Body)
	seen := make(map[string]bool)
	var results []PackageInfo

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return results, nil
		case html.StartTagToken:
			t := z.Token()
			if t.Data == "a" {
				for _, attr := range t.Attr {
					if attr.Key == "href" && strings.HasPrefix(attr.Val, "/") && strings.Count(attr.Val, "/") > 1 {
						fullURL := "https://pkg.go.dev" + attr.Val
						pkgName := strings.TrimPrefix(attr.Val, "/")

						if !seen[fullURL] {
							seen[fullURL] = true
							results = append(results, PackageInfo{
								Name: pkgName,
								URL:  fullURL,
							})
						}
					}
				}
			}
		}
	}
}

func getAllPackagesStructured() []PackageInfo {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("%s[%s!%s]%s Error finding home directory: %v%s\n", White, Red, White, White, err, Reset)
		return nil
	}

	Gopath := filepath.Join(homeDir, "go")
	var foundPaths []PackageInfo
	var mu sync.Mutex
	var wg sync.WaitGroup

	err = filepath.WalkDir(Gopath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() && strings.Contains(strings.ToLower(d.Name()), "go") {
			if path == Gopath {
				return nil
			}
			pkgName := strings.TrimPrefix(path, Gopath+string(filepath.Separator))
			if pkgName != "" && !strings.HasPrefix(pkgName, ".") {
				wg.Add(1)
				go func(name, p string) {
					defer wg.Done()
					size := getDirSize(p)
					info, _ := os.Stat(p)
					mtime := time.Time{}
					if info != nil {
						mtime = info.ModTime()
					}
					mu.Lock()
					foundPaths = append(foundPaths, PackageInfo{
						Name:     name,
						Path:     p,
						Size:     size,
						ModTime:  mtime,
						Selected: false,
					})
					mu.Unlock()
				}(pkgName, path)
			}
		}
		return nil
	})

	wg.Wait()

	if err != nil {
		fmt.Printf("%s[%s!%s]%s Error walking directory: %v%s\n", White, Red, White, White, err, Reset)
		return nil
	}

	sort.Slice(foundPaths, func(i, j int) bool {
		return foundPaths[i].Name < foundPaths[j].Name
	})

	return foundPaths
}

func filterPackages(packages []PackageInfo, filter string) []int {
	filter = strings.ToLower(filter)
	var indices []int

	for i, pkg := range packages {
		if strings.Contains(strings.ToLower(pkg.Name), filter) {
			indices = append(indices, i)
		}
	}
	return indices
}

func initialBrowserModel(packages []PackageInfo) model {
	items := make([]list.Item, len(packages))
	for i, p := range packages {
		items[i] = p
	}

	const defaultWidth = 100
	l := list.New(items, list.NewDefaultDelegate(), defaultWidth, 25)
	l.Title = fmt.Sprintf("%s Installed Go Packages (%d total) %s- Use ↑↓ to navigate, Space to select, Enter to delete selected, Esc to cancel%s", Cyan, len(packages), Reset, Cyan)

	filteredIdx := make([]int, len(packages))
	for i := range packages {
		filteredIdx[i] = i
	}

	return model{
		list:        l,
		choices:     packages,
		selected:    make(map[int]bool),
		mode:        "browse",
		filteredIdx: filteredIdx,
	}
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case " ":
			idx := m.list.Index()
			if idx < len(m.filteredIdx) {
				actualIdx := m.filteredIdx[idx]
				m.selected[actualIdx] = !m.selected[actualIdx]
				if m.selected[actualIdx] {
					m.selectedCount++
				} else {
					m.selectedCount--
				}
				m.updateListTitle()
			}
		case "enter":
			if m.selectedCount > 0 {
				return m, deleteSelectedAsync(m)
			}
		case "c":
			m.selected = make(map[int]bool)
			m.selectedCount = 0
			m.updateListTitle()
		case "a":
			for _, idx := range m.filteredIdx {
				if !m.selected[idx] {
					m.selected[idx] = true
					m.selectedCount++
				}
			}
			m.updateListTitle()
		case "s":
			stats := calculateStats(m.choices)
			fmt.Printf("\n%s=== GOPATH Statistics ===%s\n", Cyan, Reset)
			fmt.Printf("%sTotal Packages:%s %d\n", Green, Reset, stats.TotalPackages)
			fmt.Printf("%sTotal Size:%s %s\n", Green, Reset, formatSize(stats.TotalSize))
			fmt.Printf("%sLargest Package:%s %s (%s)\n", Yellow, Reset, stats.LargestPkg, formatSize(stats.LargestSize))
			fmt.Printf("%sOldest Package:%s %s (modified: %s)\n", Yellow, Reset, stats.OldestPkg, stats.OldestTime.Format("2006-01-02"))
			fmt.Printf("%s================================%s\n\n", Cyan, Reset)
			time.Sleep(2 * time.Second)
		case "backspace":
			if len(m.filterText) > 0 {
				m.filterText = m.filterText[:len(m.filterText)-1]
				m.applyFilter()
				m.updateListTitle()
			}
		case "d":
			if deletionLogPath != "" {
				if data, err := ioutil.ReadFile(deletionLogPath); err == nil {
					fmt.Printf("\n%s=== Recent Deletions ===%s\n", Cyan, Reset)
					fmt.Println(string(data))
					time.Sleep(3 * time.Second)
				}
			}
		default:
			if len(msg.String()) == 1 && msg.String()[0] >= 32 && msg.String()[0] < 127 {
				m.filterText += msg.String()
				m.applyFilter()
				m.updateListTitle()
			}
		}
	case deleteMsg:
		if msg.Error != nil {
			m.err = msg.Error
			m.done = false
		} else {
			m.done = msg.Success
			m.selected = make(map[int]bool)
			m.selectedCount = 0
			m.choices = getAllPackagesStructured()
			m.applyFilter()
			m.updateListTitle()
		}
	}

	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *model) applyFilter() {
	m.filteredIdx = filterPackages(m.choices, m.filterText)
	items := make([]list.Item, len(m.filteredIdx))
	for i, idx := range m.filteredIdx {
		items[i] = m.choices[idx]
	}
	m.list.SetItems(items)
}

func (m *model) updateListTitle() {
	status := "Not selected"
	freedSpace := int64(0)
	if m.selectedCount > 0 {
		for idx, selected := range m.selected {
			if selected && idx < len(m.choices) {
				freedSpace += m.choices[idx].Size
			}
		}
		status = fmt.Sprintf("%s%d selected%s (%swill free %s%s)", Green, m.selectedCount, Reset, Cyan, formatSize(freedSpace), Reset)
	}
	if m.filterText != "" {
		m.list.Title = fmt.Sprintf("%s Installed Go Packages (%d/%d) | Filter: '%s' | Status: %s %s| ⎵=Select a=SelectAll c=Clear s=Stats d=Log ↵=Delete%s",
			Cyan, len(m.filteredIdx), len(m.choices), m.filterText, status, Reset, Cyan)
	} else {
		m.list.Title = fmt.Sprintf("%s Installed Go Packages (%d total) | Status: %s %s| ⎵=Select a=SelectAll c=Clear s=Stats d=Log ↵=Delete%s",
			Cyan, len(m.choices), status, Reset, Cyan)
	}
}

func (m model) View() string {
	if m.loading {
		return fmt.Sprintf("\n%s🔄 Deleting selected packages...%s\n", Green, Reset)
	}
	if m.done {
		freedSpace := int64(0)
		for idx, selected := range m.selected {
			if selected && idx < len(m.choices) {
				freedSpace += m.choices[idx].Size
			}
		}
		return fmt.Sprintf("\n%s✅ Successfully deleted %d package(s)!%s\n%s🎉 Freed %s%s\n%s💾 Cache cleaned.%s\n\nPress any key to continue or Ctrl+C to exit.\n", Green, m.selectedCount, Reset, Green, formatSize(freedSpace), Reset, Green, Reset)
	}
	if m.err != nil {
		return fmt.Sprintf("\n%s❌ Error: %v%s\n\nPress any key or Ctrl+C to exit.\n", Red, m.err, Reset)
	}
	return m.list.View()
}

func deleteSelectedAsync(m model) tea.Cmd {
	return func() tea.Msg {
		if m.selectedCount == 0 {
			return deleteMsg{Success: false, Error: fmt.Errorf("no packages selected")}
		}

		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = fmt.Sprintf(" Deleting %d package(s)...", m.selectedCount)
		s.Start()

		successCount := 0
		var deletedPkgs []string
		var freedSpace int64
		var lastErr error

		for idx, selected := range m.selected {
			if selected && idx < len(m.choices) {
				path := m.choices[idx].Path
				name := m.choices[idx].Name
				size := m.choices[idx].Size
				err := os.RemoveAll(path)
				if err == nil {
					successCount++
					deletedPkgs = append(deletedPkgs, name)
					freedSpace += size
				} else {
					lastErr = err
				}
			}
		}

		cacheErr := runCommand("go", "clean", "-cache")
		cacheCleaned := cacheErr == nil

		logDeletion(deletedPkgs, freedSpace, cacheCleaned)

		s.Stop()

		if lastErr != nil {
			return deleteMsg{Success: false, Error: fmt.Errorf("deleted %d/%d packages (freed %s), last error: %v", successCount, m.selectedCount, formatSize(freedSpace), lastErr)}
		}

		return deleteMsg{Success: true, Error: nil}
	}
}

func initialModel(packages []PackageInfo) model {
	items := make([]list.Item, len(packages))
	for i, p := range packages {
		items[i] = p
	}

	const defaultWidth = 60
	l := list.New(items, list.NewDefaultDelegate(), defaultWidth, 20)
	l.Title = "Select a Go Package to Install"
	return model{list: l, choices: packages, mode: "install"}
}

func installPackageAsync(pkg string) tea.Cmd {
	return func() tea.Msg {
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond)
		s.Suffix = " Installing " + pkg
		s.Start()

		cmd := exec.Command("go", "get", pkg+"@latest")
		cmd.Stdout = nil
		cmd.Stderr = nil

		err := cmd.Run()
		s.Stop()

		return installMsg{
			PackageName: pkg,
			Success:     err == nil,
			Error:       err,
		}
	}
}

func InitPkgInstall(PkgName string) {
	results, err := fetchSearchResults(PkgName)
	if err != nil {
		fmt.Printf("%s[%s!%s]%s Error: %s%v%s\n", White, Red, White, White, Red, err, Reset)
		return
	}
	if len(results) == 0 {
		fmt.Println("[!] No packages found.")
		return
	}

	m := initialModel(results)
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}

func InteractiveBrowseMode() {
	fmt.Printf("%s[%s*%s]%s Scanning installed packages...%s\n", White, Green, White, White, Reset)
	packages := getAllPackagesStructured()

	if len(packages) == 0 {
		fmt.Printf("%s[%s!%s]%s No packages found in GOPATH.%s\n", White, Red, White, White, Reset)
		return
	}

	stats := calculateStats(packages)
	fmt.Printf("%s[%s✓%s]%s Found %d packages (%s). Opening interactive browser...%s\n\n", White, Green, White, White, len(packages), formatSize(stats.TotalSize), Reset)
	time.Sleep(500 * time.Millisecond)

	m := initialBrowserModel(packages)
	if _, err := tea.NewProgram(m).Run(); err != nil {
		fmt.Println("Error running program:", err)
		os.Exit(1)
	}
}

func parseIndex(input string, max int) (int, error) {
	i, err := strconv.Atoi(input)
	if err != nil || i < 1 || i > max {
		return 0, fmt.Errorf("invalid index: %s", input)
	}
	return i - 1, nil
}

func runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func DeletePackage(NoConfirm bool, Packagename string, NoClearCache bool) {
	fmt.Printf("%s[%s+%s]%s Starting Deletion Of Package %s--%s>%s %s%s\n", White, Green, White, White, Red, Red, Magenta, Packagename, Reset)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		fmt.Println("[!] Error finding user home directory:", err)
		return
	}

	Gopath := filepath.Join(homeDir, "go")
	fmt.Printf("%s[%s*%s]%s GOPATH %s--%s>%s %s%s\n", White, Green, White, White, Red, Red, Magenta, Gopath, Reset)

	var foundPaths []string

	err = filepath.WalkDir(Gopath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			fmt.Printf("%s[%s!%s]%s Error accessing %s--%s>%s %s%v%s\n", White, Red, White, White, Red, Red, Magenta, path, err, Reset)
			return nil
		}
		if d.IsDir() && strings.Contains(strings.ToLower(d.Name()), strings.ToLower(Packagename)) {
			foundPaths = append(foundPaths, path)
		}
		return nil
	})
	if err != nil {
		fmt.Printf("%s[%s!%s]%s Error while walking directory tree %s--%s>%s %v%s\n", White, Red, White, White, Red, Red, Magenta, err, Reset)
		return
	}

	if len(foundPaths) == 0 {
		fmt.Printf("%s[%s-%s]%s No matching packages found.\n", White, Red, White, Reset)
		return
	}

	fmt.Printf("%s[%s*%s]%s %s____________________%s Found Matching Package %s____________________%s\n", White, Green, White, Red, Red, Red, Red, Reset)
	for i, path := range foundPaths {
		fmt.Printf("%s[%s%d%s]%s %s%s\n", White, Yellow, i+1, White, Magenta, path, Reset)
	}

	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s[%s*%s]%s %sSelect the package you wish to delete from the list of found package(s): ", White, Green, White, White, Reset)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)

	var toDelete []string
	if strings.ToLower(input) == "all" {
		toDelete = foundPaths
	} else {
		selections := strings.Split(input, ",")
		for _, sel := range selections {
			sel = strings.TrimSpace(sel)
			index, err := parseIndex(sel, len(foundPaths))
			if err != nil {
				fmt.Printf("[!] Skipping invalid selections --> %v\n", sel)
				continue
			}
			toDelete = append(toDelete, foundPaths[index])
		}
	}

	if len(toDelete) == 0 {
		fmt.Printf("%s[%s!%s]%s %sNo valid selections made. Aborting.\n", White, Red, White, White, Reset)
		return
	}

	for _, path := range toDelete {
		if !NoConfirm {
			fmt.Printf("%s[%s!%s]%s Are you sure you want to delete this path?\n -> %s%v%s\n (y/n):", White, Red, White, White, Cyan, path, Reset)
			confirm, _ := reader.ReadString('\n')
			confirm = strings.TrimSpace(confirm)
			if confirm != "y" && confirm != "Y" {
				fmt.Printf("%s[%s*%s]%s Skipped: %s%v%s\n", White, Green, White, White, Red, path, Reset)
				continue
			}
		}

		err := os.RemoveAll(path)
		if err != nil {
			fmt.Printf("%s[%s!%s]%s Failed to delete %v: %v%s\n", White, Red, White, White, Red, path, Reset)
		}
	}

	if !NoClearCache {
		fmt.Printf("%s[%s*%s]%s Cleaning Go Cache...\n", White, Green, White, Reset)
		if err := runCommand("go", "clean", "-cache"); err != nil {
			fmt.Printf("%s[%s!%s]%s Failed to clear Go Cache: %s%v%s\n", White, Red, White, White, Red, err, Reset)
		} else {
			fmt.Printf("%s[%s*%s]%s Go Cache Cleared.%s\n", White, Green, White, White, Reset)
		}
	}
}

func main() {
	NoConfirm, Packagename, NoClearCache, Interactive, pkgName := parseFlags()

	if Interactive {
		InteractiveBrowseMode()
	} else if pkgName != "" {
		InitPkgInstall(pkgName)
	} else if Packagename != "" {
		DeletePackage(NoConfirm, Packagename, NoClearCache)
	} else {
		InteractiveBrowseMode()
	}
}

func parseFlags() (noconfirm bool, packagename string, noclearcache bool, interactive bool, pkgName string) {
	noconfirmFlag := flag.Bool("nc", false, "Disables Confirm Prompt for deleting packages.")
	packagenameFlag := flag.String("pkg", "", "Name of package you wish to delete.")
	noclearcacheFlag := flag.Bool("ncl", false, "Disables auto-clear golang cache.")
	interactiveFlag := flag.Bool("i", false, "Interactive browse mode (default).")
	pkgNameFlag := flag.String("pkgi", "", "Name of package you wish to install.")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage of %v:\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "%sEXAMPLES:%s\n", Cyan, Reset)
		fmt.Fprintf(os.Stderr, "  %s./gomanager%s                         - Interactive browser (default)\n", Green, Reset)
		fmt.Fprintf(os.Stderr, "  %s./gomanager -i%s                     - Explicit interactive mode\n", Green, Reset)
		fmt.Fprintf(os.Stderr, "  %s./gomanager -pkgi \"encoding/json\"%s  - Search and install\n", Green, Reset)
		fmt.Fprintf(os.Stderr, "  %s./gomanager -pkg \"mypackage\"%s      - Delete specific package\n", Green, Reset)
		fmt.Fprintf(os.Stderr, "\n%sFLAGS:%s\n", Cyan, Reset)
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n%sKEYBINDS (Interactive Browser):%s\n", Cyan, Reset)
		fmt.Fprintf(os.Stderr, "  %s⎵%s   Toggle selection for highlighted package\n", Yellow, Reset)
		fmt.Fprintf(os.Stderr, "  %sa%s   Select all filtered packages\n", Yellow, Reset)
		fmt.Fprintf(os.Stderr, "  %sc%s   Clear all selections\n", Yellow, Reset)
		fmt.Fprintf(os.Stderr, "  %ss%s   Show GOPATH statistics\n", Yellow, Reset)
		fmt.Fprintf(os.Stderr, "  %sd%s   View deletion log\n", Yellow, Reset)
		fmt.Fprintf(os.Stderr, "  %s↵%s   Delete all selected packages\n", Yellow, Reset)
		fmt.Fprintf(os.Stderr, "  %sType%s - Filter by package name (live search)\n", Yellow, Reset)
		fmt.Fprintf(os.Stderr, "  %sBackspace%s - Clear filter\n", Yellow, Reset)
		fmt.Fprintf(os.Stderr, "  %sEsc/Ctrl+C%s - Quit\n\n", Yellow, Reset)
	}

	flag.Parse()

	if *interactiveFlag {
		*noconfirmFlag = true
		*noclearcacheFlag = true
	}
	return *noconfirmFlag, *packagenameFlag, *noclearcacheFlag, *interactiveFlag, *pkgNameFlag
}
