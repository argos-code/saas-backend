package api

import (
	"net/http"
	"runtime/debug"

	"github.com/vocdoni/saas-backend/api/apicommon"
)

// InfoResponse contains build, version and chain information for the service.
type InfoResponse struct {
	Version   string `json:"version"`
	GoVersion string `json:"goVersion"`
	Commit    string `json:"commit"`
	ChainID   string `json:"chainId"`
}

// defaultInfoResponse holds the values returned when build info is unavailable.
var defaultInfoResponse = InfoResponse{
	Version:   "unknown",
	GoVersion: "unknown",
	Commit:    "unknown",
}

// InfoHandler godoc
//
//	@Summary		Get service information
//	@Description	Returns the service version, build information and the Vocdoni chain ID.
//	@Tags			health
//	@Produce		json
//	@Success		200	{object}	InfoResponse
//	@Router			/info [get]
func (a *API) InfoHandler(w http.ResponseWriter, _ *http.Request) {
	info := defaultInfoResponse
	if v, _ := debug.ReadBuildInfo(); v != nil {
		info.Version = v.Main.Version
		info.GoVersion = v.GoVersion
		for _, s := range v.Settings {
			if s.Key == "vcs.revision" {
				info.Commit = s.Value
				if len(info.Commit) > 8 {
					info.Commit = info.Commit[:8]
				}
				break
			}
		}
	}
	if a.account != nil {
		info.ChainID = a.account.ChainID()
	}
	apicommon.HTTPWriteJSON(w, info)
}
