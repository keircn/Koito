package handlers

import (
	"net/http"

	"github.com/gabehf/koito/internal/cfg"
	"github.com/gabehf/koito/internal/logger"
	"github.com/gabehf/koito/internal/utils"
)

type LbzInfoResponse struct {
	Status  string `json:"status"`
	Mode    string `json:"mode"`
	Version string `json:"version,omitempty"`
}

func LbzInfoHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := logger.FromContext(r.Context())

		l.Debug().Msg("LbzInfoHandler: Received request for ListenBrainz server info")

		utils.WriteJSON(w, http.StatusOK, LbzInfoResponse{
			Status:  "ok",
			Mode:    "offline",
			Version: cfg.Version(),
		})
	}
}