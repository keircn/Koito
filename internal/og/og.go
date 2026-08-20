// package og provides Open Graph metadata and a generated social preview
// image for the instance, backed by live library statistics.
package og

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/memkv"
	"github.com/gabehf/koito/internal/utils"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

const (
	statsCacheKey = "og_stats"
	imageCacheKey = "og_image"
	statsTTL      = 5 * time.Minute

	imgW = 1200
	imgH = 630
)

//go:embed fonts/*.ttf
var fontFiles embed.FS

var (
	fontOnce sync.Once
	fontErr  error

	valueFace  font.Face
	labelFace  font.Face
	footerFace font.Face
)

func loadFonts() {
	fontOnce.Do(func() {
		bold, err := parseFont("fonts/LeagueSpartan-Bold.ttf")
		if err != nil {
			fontErr = err
			return
		}
		medium, err := parseFont("fonts/LeagueSpartan-Medium.ttf")
		if err != nil {
			fontErr = err
			return
		}
		valueFace, err = newFace(bold, 108)
		if err != nil {
			fontErr = err
			return
		}
		labelFace, err = newFace(medium, 38)
		if err != nil {
			fontErr = err
			return
		}
		footerFace, err = newFace(medium, 32)
		if err != nil {
			fontErr = err
			return
		}
	})
}

func parseFont(path string) (*opentype.Font, error) {
	data, err := fontFiles.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return opentype.Parse(data)
}

func newFace(f *opentype.Font, size float64) (font.Face, error) {
	return opentype.NewFace(f, &opentype.FaceOptions{
		Size:    size,
		DPI:     72,
		Hinting: font.HintingFull,
	})
}

type statsStore interface {
	db.ListenStore
	db.TrackStore
	db.AlbumStore
	db.ArtistStore
}

type Stats struct {
	ListenCount     int64
	TrackCount      int64
	AlbumCount      int64
	ArtistCount     int64
	MinutesListened int64
	DaysActive      int
}

func InstanceStats(ctx context.Context, store statsStore) (*Stats, error) {
	if cached, ok := memkv.Store.Get(statsCacheKey); ok {
		if s, ok := cached.(*Stats); ok {
			return s, nil
		}
	}

	tf := db.Timeframe{Period: db.PeriodAllTime}

	listens, err := store.CountListens(ctx, tf)
	if err != nil {
		return nil, err
	}
	tracks, err := store.CountTracks(ctx, tf)
	if err != nil {
		return nil, err
	}
	albums, err := store.CountAlbums(ctx, tf)
	if err != nil {
		return nil, err
	}
	artists, err := store.CountArtists(ctx, tf)
	if err != nil {
		return nil, err
	}
	minutes, err := store.CountTimeListened(ctx, tf)
	if err != nil {
		return nil, err
	}
	days, err := store.GetActiveDays(ctx, time.UTC)
	if err != nil {
		return nil, err
	}

	s := &Stats{
		ListenCount:     listens,
		TrackCount:      tracks,
		AlbumCount:      albums,
		ArtistCount:     artists,
		MinutesListened: minutes / 60,
		DaysActive:      days,
	}

	memkv.Store.Set(statsCacheKey, s, statsTTL)
	return s, nil
}

// MetaTags builds the Open Graph / Twitter Card meta tags for a page,
// describing the instance with its current statistics.
func MetaTags(ctx context.Context, store statsStore, pageURL string) (string, error) {
	s, err := InstanceStats(ctx, store)
	if err != nil {
		return "", err
	}

	title := fmt.Sprintf("Koito - %s listens", comma(s.ListenCount))
	imgURL := assetURL(pageURL, "/og-image.png")

	var b strings.Builder
	writeTag := func(tag, name, content string) {
		fmt.Fprintf(&b, `%s="%s" content="%s">%s`, tag, name, html.EscapeString(content), "\n\t")
	}

	writeTag("<meta property", "og:type", "website")
	writeTag("<meta property", "og:site_name", "Koito")
	writeTag("<meta property", "og:title", title)
	writeTag("<meta property", "og:url", pageURL)
	writeTag("<meta property", "og:image", imgURL)
	writeTag("<meta name", "twitter:card", "summary_large_image")
	writeTag("<meta name", "twitter:title", title)
	writeTag("<meta name", "twitter:image", imgURL)

	return b.String(), nil
}

func assetURL(pageURL, path string) string {
	u, err := url.Parse(pageURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return path
	}
	return u.Scheme + "://" + u.Host + path
}

func ImageHandler(store statsStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, err := InstanceStats(r.Context(), store)
		if err != nil {
			utils.WriteError(w, "failed to load stats: "+err.Error(), http.StatusInternalServerError)
			return
		}
		png, err := RenderImage(s)
		if err != nil {
			utils.WriteError(w, "failed to render image: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(png)
	}
}

func RenderImage(s *Stats) ([]byte, error) {
	loadFonts()
	if fontErr != nil {
		return nil, fontErr
	}

	if cached, ok := memkv.Store.Get(imageCacheKey); ok {
		if b, ok := cached.([]byte); ok {
			return b, nil
		}
	}

	img := image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	drawGradient(img, rgb(0x20, 0x18, 0x12), rgb(0x0d, 0x0a, 0x08))

	fillCircle(img, 980, 40, 240, rgb(0xe5, 0x84, 0x6a).blend(30))
	fillCircle(img, 1130, 190, 90, rgb(0xf0, 0xc8, 0x4a).blend(25))

	stats := []struct {
		value string
		label string
	}{
		{comma(s.ListenCount), "listens"},
		{comma(s.TrackCount), "tracks"},
		{comma(s.AlbumCount), "albums"},
		{comma(s.ArtistCount), "artists"},
	}

	// 2x2 grid of stats plus a footer summary, filling most of the card
	colW := (imgW - 100) / 2
	colCenters := []int{50 + colW/2, 600 + colW/2}
	row1Y, row2Y := 175, 400
	const labelGap = 80

	for i, st := range stats {
		x := colCenters[i%2]
		y := row1Y
		if i >= 2 {
			y = row2Y
		}
		drawTextCentered(img, valueFace, st.value, x, y, rgb(0xf5, 0xec, 0xe3))
		drawTextCentered(img, labelFace, st.label, x, y+labelGap, rgb(0xa6, 0x98, 0x90))
	}

	footer := fmt.Sprintf("%s minutes listened · %d days active", comma(s.MinutesListened), s.DaysActive)
	drawTextCentered(img, footerFace, footer, imgW/2, 580, rgb(0xcf, 0xc3, 0xb7))

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}

	out := buf.Bytes()
	memkv.Store.Set(imageCacheKey, out, statsTTL)
	return out, nil
}

type rgba color.RGBA

func rgb(r, g, b uint8) rgba {
	return rgba{R: r, G: g, B: b, A: 255}
}

func (c rgba) blend(a uint8) rgba {
	c.A = a
	return c
}

func drawGradient(img *image.RGBA, top, bottom rgba) {
	h := img.Bounds().Dy()
	for y := 0; y < h; y++ {
		t := float64(y) / float64(h-1)
		c := rgba{
			R: uint8(float64(top.R) + t*(float64(bottom.R)-float64(top.R))),
			G: uint8(float64(top.G) + t*(float64(bottom.G)-float64(top.G))),
			B: uint8(float64(top.B) + t*(float64(bottom.B)-float64(top.B))),
			A: 255,
		}
		for x := 0; x < img.Bounds().Dx(); x++ {
			img.SetRGBA(x, y, color.RGBA(c))
		}
	}
}

func fillCircle(img *image.RGBA, cx, cy, r int, c rgba) {
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
				continue
			}
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= r*r {
				blendOver(img, x, y, c)
			}
		}
	}
}

func blendOver(img *image.RGBA, x, y int, c rgba) {
	if c.A == 255 {
		img.SetRGBA(x, y, color.RGBA(c))
		return
	}
	dr, dg, db, _ := img.RGBAAt(x, y).RGBA()
	sa := uint32(c.A)
	sr := uint32(c.R) * 0x101
	sg := uint32(c.G) * 0x101
	sb := uint32(c.B) * 0x101
	img.SetRGBA(x, y, color.RGBA{
		R: uint8((sr*sa + dr*(0xffff-sa)) / 0xffff >> 8),
		G: uint8((sg*sa + dg*(0xffff-sa)) / 0xffff >> 8),
		B: uint8((sb*sa + db*(0xffff-sa)) / 0xffff >> 8),
		A: 255,
	})
}

func drawText(img *image.RGBA, face font.Face, s string, x, y int, c rgba) {
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(color.RGBA(c)),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	d.DrawString(s)
}

func drawTextCentered(img *image.RGBA, face font.Face, s string, cx, y int, c rgba) {
	w := font.MeasureString(face, s).Ceil()
	drawText(img, face, s, cx-w/2, y, c)
}

func comma(n int64) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := b.String()
	if neg {
		return "-" + out
	}
	return out
}
