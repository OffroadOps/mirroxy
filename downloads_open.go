package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func trackDownloadProgress(task *DownloadTask, status ariaStatus, now time.Time) {
	if status.Status != "active" || status.Speed != "0" || task.lastCompleted != status.Completed {
		task.noProgressSince = now
		task.Stalled = false
	} else if task.noProgressSince.IsZero() {
		task.noProgressSince = now
	} else {
		task.Stalled = now.Sub(task.noProgressSince) >= 45*time.Second
	}
	task.lastCompleted = status.Completed
}

// Never accepts a caller-supplied executable or command line: only journal
// targets, validated against the task directory, with local CSRF guards.
func (d *DownloadManager) downloadOpenTarget(task *DownloadTask, folder bool) (string, error) {
	if !folder && task.Status != "complete" {
		return "", errors.New("文件尚未下载完成，不能打开")
	}
	if _, err := d.validate(task.DownloadSpec); err != nil {
		return "", errors.New("保存目录校验失败")
	}
	directory := resolveDownloadDirectory(task.Dir)
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return "", errors.New("下载目录不存在")
	}
	if task.Path == "" {
		if folder {
			return directory, nil
		}
		return "", errors.New("下载文件路径不存在")
	}
	path := filepath.Clean(filepath.FromSlash(task.Path))
	parent := resolveDownloadDirectory(filepath.Dir(path))
	same := parent == directory
	if runtime.GOOS == "windows" {
		same = strings.EqualFold(parent, directory)
	}
	if !filepath.IsAbs(path) || !same {
		return "", errors.New("文件不属于任务保存目录")
	}
	info, err = os.Lstat(path)
	if os.IsNotExist(err) && folder {
		return directory, nil
	}
	if err != nil {
		return "", errors.New("下载文件不存在或无法访问")
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("拒绝打开符号链接、目录或特殊文件")
	}
	return path, nil
}
