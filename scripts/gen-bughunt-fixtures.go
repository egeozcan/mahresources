//go:build ignore

// gen-bughunt-fixtures generates the edge-case media fixtures the 2026-07-29 UI
// bug hunt needs (docs/todo.md, Phase 0). The seeded data from
// .claude/skills/seed-data/seed.sh has none of these shapes: its groups are
// flat, its resources are fetched from picsum.photos, and every name is ASCII.
//
// Several findings only reproduce against extreme aspect ratios, a PNG with a
// real alpha channel, a vector image, or a zero-byte file, so the fixtures are
// checked in rather than generated at verification time.
//
// Usage: go run scripts/gen-bughunt-fixtures.go
package main

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

const outDir = "e2e/test-assets/bughunt"

func main() {
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		log.Fatal(err)
	}

	// Finding 75: a 400x1600 card blows its grid row to 1831px and drags its
	// neighbour with it. Finding 67 needs the same treatment horizontally.
	writePNG("panorama-2400x400.png", gradient(2400, 400))
	writePNG("tall-400x1600.png", gradient(400, 1600))
	writePNG("icon-32x32.png", gradient(32, 32))

	// Finding 12: rotate transcodes PNG to JPEG, flattening alpha and inflating
	// the file. Only an image with genuinely transparent pixels shows it.
	writePNG("alpha-transparent.png", alphaCorners(200, 200))

	// Baseline for the rotate/crop format-preservation tests.
	writePNG("plain-400x300.png", gradient(400, 300))

	// Versions for the multi-version resource (findings 85, 140).
	writePNG("version-2.png", solid(320, 240, color.RGBA{R: 0x2f, G: 0x6f, B: 0xed, A: 0xff}))
	writePNG("version-3.png", solid(320, 240, color.RGBA{R: 0xed, G: 0x6f, B: 0x2f, A: 0xff}))

	// Findings 10, 11, 69, 72, 73, 86: SVG is offered rotate/recalculate and
	// gets a 0x0 JPEG preview.
	writeFile("edge.svg", `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 120 60" width="120" height="60">
  <rect x="4" y="4" width="112" height="52" fill="#0f766e" rx="6"/>
  <circle cx="30" cy="30" r="16" fill="#fff"/>
  <text x="76" y="36" text-anchor="middle" font-size="16" fill="#fff">SVG</text>
</svg>
`)

	// Findings 69, 72: non-image types must fall back to the file placeholder.
	writeFile("notes.md", "# Bug hunt fixture\n\nA markdown resource, for the non-raster preview path.\n")
	writeFile("payload.json", "{\n  \"fixture\": \"bughunt\",\n  \"kind\": \"non-raster\"\n}\n")
	writeFile("table.csv", "id,name,value\n1,alpha,10\n2,beta,20\n3,gamma,30\n")
	writeFile("plain.txt", "A plain text resource. Preview should 307 to the file placeholder.\n")

	// A zero-byte file: the degenerate case for every decode path.
	writeFile("empty.bin", "")

	fmt.Printf("wrote fixtures to %s\n", outDir)
}

// gradient produces a visually distinct image whose bytes differ per size, so
// the server's content-hash dedup does not collapse two fixtures into one.
func gradient(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{
				R: uint8((x * 255) / max(w-1, 1)),
				G: uint8((y * 255) / max(h-1, 1)),
				B: uint8((x + y) % 256),
				A: 0xff,
			})
		}
	}
	return img
}

func solid(w, h int, c color.RGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	return img
}

// alphaCorners draws an opaque disc on a fully transparent background. After a
// format-preserving rotate the corners must still be transparent; after a JPEG
// transcode they turn black or white and the file grows.
func alphaCorners(w, h int) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	cx, cy, r := float64(w)/2, float64(h)/2, float64(min(w, h))/2-4
	for y := range h {
		for x := range w {
			dx, dy := float64(x)-cx, float64(y)-cy
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, color.NRGBA{R: 0xe1, G: 0x1d, B: 0x48, A: 0xff})
			} else {
				img.Set(x, y, color.NRGBA{})
			}
		}
	}
	return img
}

func writePNG(name string, img image.Image) {
	path := filepath.Join(outDir, name)
	f, err := os.Create(path)
	if err != nil {
		log.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		log.Fatalf("encode %s: %v", path, err)
	}
}

func writeFile(name, body string) {
	path := filepath.Join(outDir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		log.Fatalf("write %s: %v", path, err)
	}
}
