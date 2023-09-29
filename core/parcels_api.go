package core

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/dir01/parcels/parcels_api"
	"github.com/hori-ryota/zaperr"
	"go.uber.org/zap"
)

var ErrNoTrackingInfo = errors.New("not found")

type ParcelsAPI struct {
	apiURL string
}

func (api *ParcelsAPI) GetTrackingInfo(ctx context.Context, trackingNumber string) ([]*parcels_api.TrackingInfo, error) {
	url := api.apiURL + "/trackingInfo/" + "?trackingNumber=" + trackingNumber

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNoTrackingInfo
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var trackingInfos []*parcels_api.TrackingInfo
	if err := json.Unmarshal(body, &trackingInfos); err != nil {
		return nil, zaperr.Wrap(err, "failed to unmarshal tracking info", zap.String("body", string(body)))
	}

	return trackingInfos, nil
}
