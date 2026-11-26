package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

// Generates crisp 4x supersampled multi-resolution PNG-in-ICO shield icons
// matching web/favicon.svg (standard blue-to-emerald shield + connected emerald shield).
func main() {
	if err := os.MkdirAll("assets", 0755); err != nil {
		log.Fatal(err)
	}
	sizes := []int{16, 32, 48, 256}
	if err := writeICO(filepath.Join("assets", "ozevpn.ico"), sizes, false); err != nil {
		log.Fatal(err)
	}
	if err := writeICO(filepath.Join("assets", "ozevpn-connected.ico"), sizes, true); err != nil {
		log.Fatal(err)
	}
	// Also place copies in internal/tray for //go:embed fallback (used in `go run .`)
	if err := os.MkdirAll(filepath.Join("internal", "tray"), 0755); err != nil {
		log.Fatal(err)
	}
	for _, name := range []string{"ozevpn.ico", "ozevpn-connected.ico"} {
		b, err := os.ReadFile(filepath.Join("assets", name))
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join("internal", "tray", name), b, 0644); err != nil {
			log.Fatal(err)
		}
	}
}

func writeICO(path string, sizes []int, connected bool) error {
	pngs := make([][]byte, len(sizes))
	for i, sz := range sizes {
		img := renderShield(sz, connected)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return err
		}
		pngs[i] = buf.Bytes()
	}

	var out bytes.Buffer
	// ICONDIR header: reserved=0, type=1 (ICO), count=len(sizes)
	_ = binary.Write(&out, binary.LittleEndian, uint16(0))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(len(sizes)))

	offset := uint32(6 + 16*len(sizes))
	for i, sz := range sizes {
		dim := byte(sz)
		if sz >= 256 {
			dim = 0 // 0 means 256 in ICO format
		}
		out.WriteByte(dim)                                              // width
		out.WriteByte(dim)                                              // height
		out.WriteByte(0)                                                // color palette
		out.WriteByte(0)                                                // reserved
		_ = binary.Write(&out, binary.LittleEndian, uint16(1))          // color planes
		_ = binary.Write(&out, binary.LittleEndian, uint16(32))         // bits per pixel
		_ = binary.Write(&out, binary.LittleEndian, uint32(len(pngs[i])))
		_ = binary.Write(&out, binary.LittleEndian, offset)
		offset += uint32(len(pngs[i]))
	}
	for _, p := range pngs {
		out.Write(p)
	}
	return os.WriteFile(path, out.Bytes(), 0644)
}

func renderShield(size int, connected bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	const ss = 4 // 4x4 supersampling for razor-sharp edges at 16px..256px
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var rSum, gSum, bSum, aSum float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					// Map to 0..64 design coordinate space (matching web/favicon.svg)
					u := (float64(x) + (float64(sx)+0.5)/float64(ss)) * 64.0 / float64(size)
					v := (float64(y) + (float64(sy)+0.5)/float64(ss)) * 64.0 / float64(size)
					c := samplePixel(u, v, connected)
					af := float64(c.A) / 255.0
					rSum += float64(c.R) * af
					gSum += float64(c.G) * af
					bSum += float64(c.B) * af
					aSum += af
				}
			}
			n := float64(ss * ss)
			aAvg := aSum / n
			if aAvg <= 0.001 {
				img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, 0})
				continue
			}
			img.SetNRGBA(x, y, color.NRGBA{
				R: clampByte(rSum / aSum),
				G: clampByte(gSum / aSum),
				B: clampByte(bSum / aSum),
				A: clampByte(aAvg * 255.0),
			})
		}
	}
	return img
}

func samplePixel(u, v float64, connected bool) color.NRGBA {
	// Rounded square background: rect 0..64, rx=16
	if !inRoundedRect(u, v, 0, 0, 64, 64, 16) {
		return color.NRGBA{0, 0, 0, 0}
	}

	// Base background (#0b0f17) with subtle green border ring when connected
	bg := color.NRGBA{0x0b, 0x0f, 0x17, 0xff}
	if connected && !inRoundedRect(u, v, 3, 3, 58, 58, 13.5) {
		bg = color.NRGBA{0x10, 0xb9, 0x81, 0xff}
	}

	if !inShield(u, v) {
		return bg
	}

	// Checkmark cutout: M25 32 L30 37 L40 26, stroke-width ~5.4
	d1 := distToSegment(u, v, 24.5, 32.0, 30.0, 37.5)
	d2 := distToSegment(u, v, 30.0, 37.5, 40.5, 26.0)
	if math.Min(d1, d2) <= 2.9 {
		return color.NRGBA{0x0b, 0x0f, 0x17, 0xff}
	}

	// Diagonal gradient across shield
	t := clamp01((u + v - 20.0) / 88.0)
	if connected {
		// Vivid emerald gradient (#10b981 -> #34d399)
		return lerpColor(color.NRGBA{0x05, 0x96, 0x69, 0xff}, color.NRGBA{0x34, 0xd3, 0x99, 0xff}, t)
	}
	// Brand gradient (#3b82f6 -> #10b981)
	return lerpColor(color.NRGBA{0x3b, 0x82, 0xf6, 0xff}, color.NRGBA{0x10, 0xb9, 0x81, 0xff}, t)
}

func inRoundedRect(x, y, rx, ry, w, h, rad float64) bool {
	if x < rx || x > rx+w || y < ry || y > ry+h {
		return false
	}
	cx := math.Max(rx+rad, math.Min(x, rx+w-rad))
	cy := math.Max(ry+rad, math.Min(y, ry+h-rad))
	dx := x - cx
	dy := y - cy
	return dx*dx+dy*dy <= rad*rad
}

// Shield shape matching M32 10 L50 18 V31 C50 42.5 42.3 52.8 32 56 C21.7 52.8 14 42.5 14 31 V18 Z
func inShield(x, y float64) bool {
	dx := math.Abs(x - 32.0)
	if y < 10.0 || y > 56.0 || dx > 18.0 {
		return false
	}
	if y < 18.0 {
		// Top roof slope from (0,10) to (18,18)
		maxX := (y - 10.0) * 18.0 / 8.0
		return dx <= maxX
	}
	if y <= 31.0 {
		return dx <= 18.0
	}
	// Smooth curved lower taper from y=31 (dx=18) to y=56 (dx=0)
	t := (y - 31.0) / 25.0
	maxX := 18.0 * math.Pow(1.0-t*t, 0.85)
	return dx <= maxX
}

func distToSegment(px, py, x1, y1, x2, y2 float64) float64 {
	dx, dy := x2-x1, y2-y1
	l2 := dx*dx + dy*dy
	if l2 == 0 {
		return math.Hypot(px-x1, py-y1)
	}
	t := clamp01(((px-x1)*dx + (py-y1)*dy) / l2)
	return math.Hypot(px-(x1+t*dx), py-(y1+t*dy))
}

func lerpColor(a, b color.NRGBA, t float64) color.NRGBA {
	return color.NRGBA{
		R: clampByte(float64(a.R) + (float64(b.R)-float64(a.R))*t),
		G: clampByte(float64(a.G) + (float64(b.G)-float64(a.G))*t),
		B: clampByte(float64(a.B) + (float64(b.B)-float64(a.B))*t),
		A: 255,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampByte(v float64) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(math.Round(v))
}
