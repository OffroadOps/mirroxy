package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

func coreAssetName(engine, version, goos, arch string) (string, error) {
	if arch != "arm64" && arch != "amd64" {
		return "", fmt.Errorf("暂不支持 CPU 架构 %s", arch)
	}
	if goos != "windows" && goos != "linux" && goos != "darwin" {
		return "", fmt.Errorf("暂不支持平台 %s", goos)
	}
	if engine == "xray" {
		if goos == "darwin" {
			goos = "macos"
		}
		cpu := "64"
		if arch == "arm64" {
			cpu = "arm64-v8a"
		}
		return "Xray-" + goos + "-" + cpu + ".zip", nil
	}
	if engine == "sing-box" {
		ext := "-musl.tar.gz"
		if goos == "windows" {
			ext = ".zip"
		} else if goos == "darwin" {
			ext = ".tar.gz"
		}
		return "sing-box-" + strings.TrimPrefix(version, "v") + "-" + goos + "-" + arch + ext, nil
	}
	return "", errors.New("不支持的核心")
}

func readCorePayload(r io.Reader) ([]byte, error) {
	b, e := io.ReadAll(io.LimitReader(r, (160<<20)+1))
	if e != nil {
		return nil, e
	}
	if len(b) > 160<<20 {
		return nil, errors.New("核心文件大小异常")
	}
	return b, nil
}

// Extract only the regular executable by basename. Archive paths are never
// written to disk, so ../ entries and symlinks cannot escape the destination.
func extractCorePayload(archive []byte, base string) ([]byte, error) {
	if bytes.HasPrefix(archive, []byte{'P', 'K'}) {
		zr, e := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if e != nil {
			return nil, e
		}
		for _, f := range zr.File {
			if path.Base(f.Name) != base || !f.Mode().IsRegular() {
				continue
			}
			if f.UncompressedSize64 > 160<<20 {
				return nil, errors.New("核心文件大小异常")
			}
			r, e := f.Open()
			if e != nil {
				return nil, e
			}
			b, e := readCorePayload(r)
			r.Close()
			return b, e
		}
	} else {
		gz, e := gzip.NewReader(bytes.NewReader(archive))
		if e != nil {
			return nil, e
		}
		defer gz.Close()
		tr := tar.NewReader(io.LimitReader(gz, 320<<20))
		for {
			h, e := tr.Next()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, e
			}
			if h.Typeflag == tar.TypeReg && path.Base(h.Name) == base {
				if h.Size > 160<<20 {
					return nil, errors.New("核心文件大小异常")
				}
				return readCorePayload(tr)
			}
		}
	}
	return nil, errors.New("更新包中未找到核心程序")
}
