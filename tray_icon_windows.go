//go:build windows

package main

import (
	"bytes"
	"fmt"
	"image"
	_ "image/png"
	"math"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

type trayVisualState int

const (
	trayStateIdle trayVisualState = iota
	trayStateProxy
	trayStatePaused
)

var trayGdi = windows.NewLazySystemDLL("gdi32.dll")

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

type iconInfo struct {
	Icon     int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

func trayRouteName(mode string) string {
	switch mode {
	case "proxy":
		return "全局"
	case "blacklist":
		return "黑名单"
	default:
		return "绕过大陆"
	}
}

func fitUTF16Tip(s string, maxUnits int) string {
	if maxUnits <= 0 {
		return ""
	}
	if len(utf16.Encode([]rune(s))) <= maxUnits {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && len(utf16.Encode(runes))+1 > maxUnits {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func trayTooltip(state trayVisualState, nodeName, routingMode string, runningCount int) string {
	var msg string
	switch state {
	case trayStateProxy:
		name := trayTruncate(strings.TrimSpace(nodeName), 18)
		if name == "" {
			name = "运行中节点"
		}
		msg = fmt.Sprintf("Mirroxy · 代理中：%s（%s）", name, trayRouteName(routingMode))
	case trayStatePaused:
		msg = "Mirroxy · 加速器接管中（已暂停系统代理）"
	default:
		if runningCount > 0 {
			msg = fmt.Sprintf("Mirroxy · 运行中（%d 个节点 · 未接管系统代理）", runningCount)
		} else {
			msg = "Mirroxy · 待机（单击打开面板）"
		}
	}
	return fitUTF16Tip(msg, 127)
}

func samplePremulBilinear(src image.Image, u, v float64) (r, g, b, a float64) {
	bounds := src.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return 0, 0, 0, 0
	}
	x0 := int(math.Floor(u))
	y0 := int(math.Floor(v))
	fx := u - float64(x0)
	fy := v - float64(y0)
	clamp := func(val, lo, hi int) int {
		if val < lo {
			return lo
		}
		if val > hi {
			return hi
		}
		return val
	}
	px := func(x, y int) (float64, float64, float64, float64) {
		cx := bounds.Min.X + clamp(x, 0, w-1)
		cy := bounds.Min.Y + clamp(y, 0, h-1)
		cr, cg, cb, ca := src.At(cx, cy).RGBA()
		const scale = 255.0 / 65535.0
		return float64(cr) * scale, float64(cg) * scale, float64(cb) * scale, float64(ca) * scale
	}
	r00, g00, b00, a00 := px(x0, y0)
	r10, g10, b10, a10 := px(x0+1, y0)
	r01, g01, b01, a01 := px(x0, y0+1)
	r11, g11, b11, a11 := px(x0+1, y0+1)
	w00 := (1 - fx) * (1 - fy)
	w10 := fx * (1 - fy)
	w01 := (1 - fx) * fy
	w11 := fx * fy
	return r00*w00 + r10*w10 + r01*w01 + r11*w11,
		g00*w00 + g10*w10 + g01*w01 + g11*w11,
		b00*w00 + b10*w10 + b01*w01 + b11*w11,
		a00*w00 + a10*w10 + a01*w01 + a11*w11
}

func blendOverPremul(br, bg, bb, ba, fr, fg, fb, cov float64) (float64, float64, float64, float64) {
	if cov <= 0 {
		return br, bg, bb, ba
	}
	if cov > 1 {
		cov = 1
	}
	inv := 1 - cov
	return fr*cov + br*inv, fg*cov + bg*inv, fb*cov + bb*inv, 255.0*cov + ba*inv
}

func circleCoverage(dist, radius float64) float64 {
	// 1px anti-aliased falloff around radius.
	diff := radius - dist
	if diff >= 0.5 {
		return 1
	}
	if diff <= -0.5 {
		return 0
	}
	return diff + 0.5
}

func createBadgedTrayIcon(size int, state trayVisualState) uintptr {
	if size <= 0 {
		size = 32
	}
	raw, err := frontend.ReadFile("web/icon.png")
	if err != nil {
		return 0
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return 0
	}
	bounds := img.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()
	if srcW <= 0 || srcH <= 0 {
		return 0
	}

	var bi bitmapInfo
	bi.Header = bitmapInfoHeader{
		Size:        uint32(unsafe.Sizeof(bi.Header)),
		Width:       int32(size),
		Height:      -int32(size), // top-down DIB
		Planes:      1,
		BitCount:    32,
		Compression: 0, // BI_RGB
	}
	var bitsPtr unsafe.Pointer
	hbmp, _, _ := trayGdi.NewProc("CreateDIBSection").Call(
		0,
		uintptr(unsafe.Pointer(&bi)),
		0,
		uintptr(unsafe.Pointer(&bitsPtr)),
		0,
		0,
	)
	if hbmp == 0 || bitsPtr == nil {
		return 0
	}
	defer trayGdi.NewProc("DeleteObject").Call(hbmp)

	dst := unsafe.Slice((*byte)(bitsPtr), size*size*4)
	rHalo := math.Max(4.0, float64(size)*0.22)
	rDot := math.Max(2.6, float64(size)*0.15)
	margin := math.Max(0.8, float64(size)*0.03)
	cx := float64(size) - rHalo - margin
	cy := float64(size) - rHalo - margin

	var dr, dg, db float64
	drawBadge := state == trayStateProxy || state == trayStatePaused
	if state == trayStateProxy {
		dr, dg, db = 16, 185, 129 // #10B981 green
	} else if state == trayStatePaused {
		dr, dg, db = 245, 158, 11 // #F59E0B amber
	}

	clampByte := func(v float64) byte {
		if v <= 0 {
			return 0
		}
		if v >= 255 {
			return 255
		}
		return byte(math.Round(v))
	}

	for y := 0; y < size; y++ {
		sy := (float64(y)+0.5)*float64(srcH)/float64(size) - 0.5
		for x := 0; x < size; x++ {
			sx := (float64(x)+0.5)*float64(srcW)/float64(size) - 0.5
			r, g, b, a := samplePremulBilinear(img, sx, sy)
			if drawBadge {
				dist := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
				if haloCov := circleCoverage(dist, rHalo); haloCov > 0 {
					r, g, b, a = blendOverPremul(r, g, b, a, 255, 255, 255, haloCov)
				}
				if dotCov := circleCoverage(dist, rDot); dotCov > 0 {
					r, g, b, a = blendOverPremul(r, g, b, a, dr, dg, db, dotCov)
				}
			}
			off := (y*size + x) * 4
			dst[off+0] = clampByte(b)
			dst[off+1] = clampByte(g)
			dst[off+2] = clampByte(r)
			dst[off+3] = clampByte(a)
		}
	}

	stride := ((size + 15) / 16) * 2
	mask := make([]byte, stride*size)
	hmask, _, _ := trayGdi.NewProc("CreateBitmap").Call(
		uintptr(size),
		uintptr(size),
		1,
		1,
		uintptr(unsafe.Pointer(&mask[0])),
	)
	if hmask == 0 {
		return 0
	}
	defer trayGdi.NewProc("DeleteObject").Call(hmask)

	info := iconInfo{
		Icon:     1,
		HbmMask:  hmask,
		HbmColor: hbmp,
	}
	hicon, _, _ := trayUser.NewProc("CreateIconIndirect").Call(uintptr(unsafe.Pointer(&info)))
	return hicon
}
