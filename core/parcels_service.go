package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/dir01/parcels/parcels_api"
	"github.com/hori-ryota/zaperr"
	"go.uber.org/zap"
)

type ParcelsService struct {
	serviceURL string
}

func (s *ParcelsService) GetTrackingInfo(ctx context.Context, trackingNumber string) ([]*parcels_api.TrackingInfo, error) {
	url := s.serviceURL + "/trackingInfo/" + "?trackingNumber=" + trackingNumber

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
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
