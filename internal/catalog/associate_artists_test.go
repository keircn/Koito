package catalog_test

import (
	"context"
	"testing"
	"time"

	"github.com/gabehf/koito/internal/catalog"
	"github.com/gabehf/koito/internal/db"
	"github.com/gabehf/koito/internal/mbz"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubmitListen_PreservesStylisticArtistCasing(t *testing.T) {
	store := newTestDB()
	ctx := context.Background()

	artistID := uuid.MustParse("00000000-0000-0000-0000-000000000021")
	mbzc := &mbz.MbzMockCaller{
		Artists: map[uuid.UUID]*mbz.MusicBrainzArtist{
			artistID: {
				Name: "Depeche Mode",
				Aliases: []mbz.MusicBrainzArtistAlias{
					{Name: "DM", Type: "Artist name", Primary: true},
				},
			},
		},
	}

	err := catalog.SubmitListen(ctx, store, catalog.SubmitListenOpts{
		MbzCaller: mbzc,
		ArtistNames: []string{"depeche mode"},
		Artist:      "depeche mode",
		ArtistMbidMappings: []catalog.ArtistMbidMap{
			{Artist: "depeche mode", Mbid: artistID},
		},
		TrackTitle:   "Personal Jesus",
		ReleaseTitle: "Violator",
		Time:         time.Now(),
		UserID:       1,
	})
	require.NoError(t, err)

	artist, err := store.GetArtist(ctx, db.GetArtistOpts{MusicBrainzID: artistID})
	require.NoError(t, err)
	assert.Equal(t, "depeche mode", artist.Name,
		"primary artist name should preserve the submitted lowercase spelling")
}

func TestSubmitListen_PrimaryArtistIsFirstSubmittedName(t *testing.T) {
	store := newTestDB()
	ctx := context.Background()

	featuredID := uuid.MustParse("00000000-0000-0000-0000-000000000022")
	mbzc := &mbz.MbzMockCaller{
		Artists: map[uuid.UUID]*mbz.MusicBrainzArtist{
			featuredID: {
				Name: "Alice in Chains",
				Aliases: []mbz.MusicBrainzArtistAlias{
					{Name: "AIC", Type: "Artist name", Primary: true},
				},
			},
		},
	}

	err := catalog.SubmitListen(ctx, store, catalog.SubmitListenOpts{
		MbzCaller: mbzc,
		ArtistNames: []string{"depeche mode", "alice in chains"},
		Artist:      "depeche mode feat. alice in chains",
		ArtistMbidMappings: []catalog.ArtistMbidMap{
			{Artist: "alice in chains", Mbid: featuredID},
		},
		TrackTitle:   "Would?",
		ReleaseTitle: "Dirt",
		Time:         time.Now(),
		UserID:       1,
	})
	require.NoError(t, err)

	var trackID int32
	err = store.QueryRow(`SELECT id FROM tracks LIMIT 1`).Scan(&trackID)
	require.NoError(t, err)

	artists, err := store.GetArtistsForTrack(ctx, trackID)
	require.NoError(t, err)
	require.Len(t, artists, 2)

	assert.Equal(t, "depeche mode", artists[0].Name)
	assert.True(t, artists[0].IsPrimary, "the first submitted artist should be primary")
	assert.Equal(t, "alice in chains", artists[1].Name)
	assert.False(t, artists[1].IsPrimary)
}