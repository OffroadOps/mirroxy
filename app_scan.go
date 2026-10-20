package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	appScanMaxDepth = 3
	appScanMaxFiles = 200
)

type scannedExecutable struct {
	Path string `json:"path"`
	Name string `json:"name"`
	Size int64  `json:"size"`
	Main bool   `json:"main"`
}

// Helper and installer executables are offered last and never flagged as the main program.
var appScanJunkNames = []string{"uninstall", "uninst", "unins", "setup", "install", "installer", "update", "updater", "crashpad", "crashhandler", "elevation", "vc_redist", "dxsetup"}

func appScanJunk(name string) bool {
	base := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	for _, junk := range appScanJunkNames {
		if strings.Contains(base, junk) {
			return true
		}
	}
	return false
}

func appScanSkipDir(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasPrefix(lower, ".") || strings.HasPrefix(lower, "$") ||
		lower == "system volume information" || lower == "recycler" || lower == "recycled" ||
		lower == "locales" || lower == "node_modules"
}

func scanDirectoryExecutables(root string) ([]scannedExecutable, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("请先填写要扫描的目录")
	}
	root = filepath.Clean(root)
	if !filepath.IsAbs(root) {
		return nil, errors.New("请填写完整的绝对路径目录")
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, errors.New("目录不存在或无法读取")
	}
	if !info.IsDir() {
		return nil, errors.New("请填写目录而不是文件路径")
	}
	found := []scannedExecutable{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if path == root {
				return nil
			}
			relative := strings.TrimPrefix(strings.TrimPrefix(path, root), string(filepath.Separator))
			if depth(relative) >= appScanMaxDepth || appScanSkipDir(entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".exe" {
			return nil
		}
		if len(found) >= appScanMaxFiles {
			return fs.SkipAll
		}
		meta, err := entry.Info()
		if err != nil {
			return nil
		}
		relative := strings.TrimPrefix(strings.TrimPrefix(path, root), string(filepath.Separator))
		found = append(found, scannedExecutable{
			Path: path, Name: strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name())),
			Size: meta.Size(), Main: depth(relative) == 0 && !appScanJunk(entry.Name()),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(found) == 0 {
		return nil, errors.New("此目录（含最多 3 层子目录）没有找到 .exe 程序")
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].Main != found[j].Main {
			return found[i].Main
		}
		if found[i].Size != found[j].Size {
			return found[i].Size > found[j].Size
		}
		return strings.ToLower(found[i].Path) < strings.ToLower(found[j].Path)
	})
	return found, nil
}

func depth(relative string) int {
	return strings.Count(relative, string(filepath.Separator))
}

func listWindowsDrives() []string {
	drives := []string{}
	if runtime.GOOS != "windows" {
		return drives
	}
	for letter := 'C'; letter <= 'Z'; letter++ {
		root := string(letter) + `:\`
		if _, err := os.Stat(root); err == nil {
			drives = append(drives, root)
		}
	}
	return drives
}

// Known game-accelerator process identifiers. Matching is deliberately
// conservative: generic fragments like "uu" alone would cause false positives.
var acceleratorProcessKeywords = []struct{ keyword, product string }{
	{"uu.exe", "网易 UU 加速器"}, {"uubooster", "网易 UU 加速器"}, {"uu_service", "网易 UU 加速器"},
	{"leiguan", "雷神加速器"}, {"xunyou", "迅游加速器"}, {"qiyou", "奇游加速器"},
	{"biubiu", "Biubiu 加速器"}, {"ourplay", "OurPlay 加速器"},
}

func detectAcceleratorConflicts(processes []applicationProcess) []M {
	hits := []M{}
	seen := map[string]bool{}
	for _, p := range processes {
		base := strings.ToLower(p.Name)
		if base == "" {
			continue
		}
		for _, keyword := range acceleratorProcessKeywords {
			matched := strings.Contains(base, keyword.keyword)
			if !matched && strings.Contains(keyword.keyword, ".exe") {
				matched = base == keyword.keyword
			}
			if matched && !seen[keyword.product] {
				seen[keyword.product] = true
				hits = append(hits, M{"product": keyword.product, "process": p.Name, "path": p.Path})
				break
			}
		}
	}
	return hits
}

func browseDirectory(path string) (M, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		if runtime.GOOS == "windows" {
			return M{"path": "", "parent": "", "drives": listWindowsDrives(), "directories": []string{}}, nil
		}
		path = string(filepath.Separator)
	}
	resolved, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return nil, errors.New("路径无效")
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, errors.New("目录不存在或无法读取")
	}
	if !info.IsDir() {
		resolved = filepath.Dir(resolved)
	}
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, errors.New("目录无法读取")
	}
	directories := []string{}
	for _, entry := range entries {
		if !entry.IsDir() || appScanSkipDir(entry.Name()) {
			continue
		}
		directories = append(directories, entry.Name())
	}
	sort.Slice(directories, func(i, j int) bool { return strings.ToLower(directories[i]) < strings.ToLower(directories[j]) })
	parent := filepath.Dir(resolved)
	if parent == resolved {
		parent = ""
	}
	result := M{"path": resolved, "parent": parent, "directories": directories}
	if runtime.GOOS == "windows" {
		result["drives"] = listWindowsDrives()
	}
	return result, nil
}
