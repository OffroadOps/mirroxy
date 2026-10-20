package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Only known downloaded regular files move to a same-filesystem recovery area.
// Never recursively delete directories, accept an API file path, or guess names.
func (d *DownloadManager) trashTaskFiles(task *DownloadTask) error {
	if task.Path == "" {
		return nil
	}
	completed, _ := strconv.ParseUint(task.Completed, 10, 64)
	if completed == 0 && task.Status != "complete" {
		return nil
	}
	if _, err := d.validate(task.DownloadSpec); err != nil {
		return errors.New("保存目录校验失败，未删除文件")
	}
	path := filepath.Clean(filepath.FromSlash(task.Path))
	equal := func(a, b string) bool {
		if runtime.GOOS == "windows" {
			return strings.EqualFold(a, b)
		}
		return a == b
	}
	if !filepath.IsAbs(path) || !equal(resolveDownloadDirectory(filepath.Dir(path)), resolveDownloadDirectory(task.Dir)) {
		return errors.New("文件不属于任务保存目录，未删除文件")
	}
	for _, other := range d.journal.Tasks {
		if other.ID != task.ID && other.Path != "" && equal(filepath.Clean(filepath.FromSlash(other.Path)), path) {
			return errors.New("其他任务仍关联此文件，不能删除文件")
		}
	}
	type entry struct {
		Original string `json:"original"`
		Stored   string `json:"stored"`
	}
	entries := []entry{}
	for i, source := range []string{path, path + ".aria2"} {
		info, err := os.Lstat(source)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return errors.New("无法读取下载文件，记录与文件均保留")
		}
		if !info.Mode().IsRegular() {
			return errors.New("拒绝删除目录、符号链接或特殊文件")
		}
		stored := "download.bin"
		if i == 1 {
			stored = "download.aria2"
		}
		entries = append(entries, entry{source, stored})
	}
	if len(entries) == 0 {
		return nil
	}
	trashRoot := filepath.Join(task.Dir, ".Mirroxy-trash")
	if !equal(filepath.Dir(resolveDownloadDirectory(trashRoot)), resolveDownloadDirectory(task.Dir)) {
		return errors.New("回收区不能位于外部目录")
	}
	if err := os.MkdirAll(trashRoot, 0700); err != nil {
		return errors.New("无法创建回收区，未删除文件")
	}
	recovery, err := os.MkdirTemp(trashRoot, "task-"+task.ID+"-")
	if err != nil {
		return err
	}
	count := 0
	rollback := func() {
		for i := count - 1; i >= 0; i-- {
			if _, e := os.Lstat(entries[i].Original); errors.Is(e, os.ErrNotExist) {
				_ = os.Rename(filepath.Join(recovery, entries[i].Stored), entries[i].Original)
			}
		}
	}
	for _, item := range entries {
		if err = os.Rename(item.Original, filepath.Join(recovery, item.Stored)); err != nil {
			rollback()
			return errors.New("文件无法移入回收区，任务记录保留；请检查文件占用")
		}
		count++
	}
	meta, _ := json.MarshalIndent(struct {
		TaskID string  `json:"taskId"`
		Files  []entry `json:"files"`
	}{task.ID, entries}, "", "  ")
	if err = os.WriteFile(filepath.Join(recovery, "restore.json"), meta, 0600); err != nil {
		rollback()
		return errors.New("无法保存恢复信息，任务记录保留")
	}
	return nil
}
