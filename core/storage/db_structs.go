package storage

import (
	"encoding/json"
	"time"

	"github.com/dir01/parcels/parcels_api"
	"github.com/dir01/tg-parcels/core"
)

type dbStruct struct {
	ID             int64  `db:"id"`
	UserID         int64  `db:"user_id"`
	TrackingNumber string `db:"tracking_number"`
	DisplayName    string `db:"display_name"`
	LastPolledAt   *int64 `db:"last_polled_at"`
	Payload        []byte `db:"payload"`
}

func (d dbStruct) fromBusinessStruct(t *core.Tracking) (*dbStruct, error) {
	payload, err := json.Marshal(t.TrackingInfos)
	if err != nil {
		return nil, err
	}
	d.Payload = payload
	d.ID = t.ID
	d.UserID = t.UserID
	d.DisplayName = t.DisplayName
	d.TrackingNumber = t.TrackingNumber
	if t.LastPolledAt != nil {
		lastPolledAt := t.LastPolledAt.Unix()
		d.LastPolledAt = &lastPolledAt
	}
	return &d, nil
}

func (d dbStruct) toBusinessStruct() (*core.Tracking, error) {
	var trackingInfos []*parcels_api.TrackingInfo
	err := json.Unmarshal(d.Payload, &trackingInfos)
	if err != nil {
		// this is OK, we just don't have any tracking infos
	}

	var t *time.Time = nil
	if d.LastPolledAt != nil {
		nt := time.Unix(*d.LastPolledAt, 0)
		t = &nt
	}

	return &core.Tracking{
		ID:             d.ID,
		UserID:         d.UserID,
		TrackingNumber: d.TrackingNumber,
		DisplayName:    d.DisplayName,
		LastPolledAt:   t,
		TrackingInfos:  trackingInfos,
	}, nil
}
