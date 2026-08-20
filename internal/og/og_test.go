package og

import (
	"bytes"
	"context"
	"image/png"
	"strings"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/memkv"
)

type fakeStore struct {
	db.ListenStore
	db.TrackStore
	db.AlbumStore
	db.ArtistStore

	listens   int64
	tracks    int64
	albums    int64
	artists   int64
	minutes   int64
	activeDay int
}

func (f fakeStore) CountListens(context.Context, db.Timeframe) (int64, error) {
	return f.listens, nil
}

func (f fakeStore) CountTracks(context.Context, db.Timeframe) (int64, error) {
	return f.tracks, nil
}

func (f fakeStore) CountAlbums(context.Context, db.Timeframe) (int64, error) {
	return f.albums, nil
}

func (f fakeStore) CountArtists(context.Context, db.Timeframe) (int64, error) {
	return f.artists, nil
}

func (f fakeStore) CountTimeListened(context.Context, db.Timeframe) (int64, error) {
	return f.minutes, nil
}

func (f fakeStore) GetActiveDays(context.Context, *time.Location) (int, error) {
	return f.activeDay, nil
}

func (f fakeStore) AddArtistsToAlbum(context.Context, db.AddArtistsToAlbumOpts) error {
	return nil
}

func TestComma(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"},
		{5, "5"},
		{999, "999"},
		{1000, "1,000"},
		{1234567, "1,234,567"},
		{1234567890, "1,234,567,890"},
		{-1234, "-1,234"},
	}
	for _, c := range cases {
		if got := comma(c.in); got != c.want {
			t.Errorf("comma(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMetaTags(t *testing.T) {
	store := fakeStore{listens: 1234, tracks: 321, albums: 45, artists: 6, minutes: 3600, activeDay: 12}
	memkv.Store.Set(statsCacheKey, &Stats{
		ListenCount:     1234,
		TrackCount:      321,
		AlbumCount:      45,
		ArtistCount:     6,
		MinutesListened: 60,
		DaysActive:      12,
	}, 0)

	page := "https://koito.example/artist/1"
	meta, err := MetaTags(context.Background(), store, page)
	if err != nil {
		t.Fatalf("MetaTags: %v", err)
	}

	checks := []string{
		`<meta property="og:title" content="Koito - 1,234 listens">`,
		`<meta property="og:url" content="https://koito.example/artist/1">`,
		`<meta property="og:image" content="https://koito.example/og-image.png">`,
		`<meta property="og:site_name" content="Koito">`,
		`<meta property="og:type" content="website">`,
		`<meta name="twitter:card" content="summary_large_image">`,
	}
	for _, c := range checks {
		if !strings.Contains(meta, c) {
			t.Errorf("MetaTags missing %q in:\n%s", c, meta)
		}
	}
}

func TestMetaTagsRelativeAssetURL(t *testing.T) {
	store := fakeStore{}
	memkv.Store.Set(statsCacheKey, &Stats{}, 0)

	meta, err := MetaTags(context.Background(), store, "/stats")
	if err != nil {
		t.Fatalf("MetaTags: %v", err)
	}
	if !strings.Contains(meta, `content="/og-image.png"`) {
		t.Errorf("expected relative og:image for non-absolute page URL, got:\n%s", meta)
	}
}

func TestAssetURL(t *testing.T) {
	cases := []struct {
		page, want string
	}{
		{"https://k.example/x", "https://k.example/og-image.png"},
		{"http://k.example:4110/", "http://k.example:4110/og-image.png"},
		{"/relative", "/og-image.png"},
		{"", "/og-image.png"},
	}
	for _, c := range cases {
		if got := assetURL(c.page, "/og-image.png"); got != c.want {
			t.Errorf("assetURL(%q) = %q, want %q", c.page, got, c.want)
		}
	}
}

func TestRenderImage(t *testing.T) {
	s := &Stats{ListenCount: 1234567, TrackCount: 890, AlbumCount: 123, ArtistCount: 45, MinutesListened: 999999, DaysActive: 365}
	out, err := RenderImage(s)
	if err != nil {
		t.Fatalf("RenderImage: %v", err)
	}

	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("png decode: %v", err)
	}
	b := img.Bounds()
	if b.Dx() != 1200 || b.Dy() != 630 {
		t.Fatalf("unexpected dimensions: %dx%d", b.Dx(), b.Dy())
	}

	// background should not be a solid flat color (gradient + content)
	if !colorsDiffer(img.At(10, 10), img.At(1190, 629)) {
		t.Fatal("background looks flat, expected gradient")
	}
}

func colorsDiffer(a, b interface {
	RGBA() (uint32, uint32, uint32, uint32)
}) bool {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	return ar != br || ag != bg || ab != bb
}