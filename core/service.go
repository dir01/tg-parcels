package core

import (
	"context"
	"errors"
	"time"

	"github.com/dir01/parcels/parcels_api"
	"github.com/hori-ryota/zaperr"
	"go.uber.org/zap"
)

type Service interface {
	Start(ctx context.Context)
	Updates() chan TrackingUpdate
	Track(ctx context.Context, userID int64, trackingNumber string, displayName string) error
	GetTracking(ctx context.Context, userID int64, trackingNumber string) (*Tracking, error)
	ListTrackings(ctx context.Context, userID int64) ([]*Tracking, error)
	DeleteTracking(ctx context.Context, userID int64, trackingNumber string) error
}

func NewService(
	storage Storage,
	parcelsAPIURL string,
	pollingDuration time.Duration,
	logger *zap.Logger,
) *ServiceImpl {
	s := &ServiceImpl{
		storage:         storage,
		parcelsAPI:      &ParcelsAPI{apiURL: parcelsAPIURL},
		pollingDuration: pollingDuration,
		logger:          logger,
		updatesChan:     make(chan TrackingUpdate),
	}
	var _ Service = s
	return s
}

type ServiceImpl struct {
	storage         Storage
	pollingDuration time.Duration
	parcelsAPI      *ParcelsAPI
	logger          *zap.Logger
	updatesChan     chan TrackingUpdate
}

type Storage interface {
	SaveTracking(ctx context.Context, tracking *Tracking) (*Tracking, error)
	GetTracking(ctx context.Context, userID int64, trackingNumber string) (*Tracking, error)
	ListTrackingsLastPolledBefore(ctx context.Context, t time.Time) ([]*Tracking, error)
	ListTrackingsByUserID(ctx context.Context, userID int64) ([]*Tracking, error)
	DeleteTracking(ctx context.Context, userID int64, trackingNumber string) error
}

var ErrTrackingExists = errors.New("tracking exists")

type Tracking struct {
	ID             int64
	UserID         int64
	TrackingNumber string
	DisplayName    string
	TrackingInfos  []*parcels_api.TrackingInfo
	LastPolledAt   *time.Time
}

type TrackingUpdate struct {
	TrackingNumber    string
	UserID            int64
	DisplayName       string
	NewTrackingInfos  []*parcels_api.TrackingInfo
	NewTrackingEvents []*parcels_api.TrackingEvent
	TrackingError     error
}

func (s *ServiceImpl) Updates() chan TrackingUpdate {
	return s.updatesChan
}

func (s *ServiceImpl) Start(ctx context.Context) {
	s.logger.Debug("service starting")
	go func() {
		s.poll(ctx)

		t := time.NewTicker(s.pollingDuration)
		for {
			select {
			case <-ctx.Done():
				s.logger.Debug("polling stopped")
				return
			case <-t.C:
				s.poll(ctx)
			}
		}
	}()
	s.logger.Debug("polling started")
}

// Track starts tracking a new tracking number for a user
// if the tracking number is already being tracked by the user
// it will just fetch the tracking info
// Please note that the result of fetching the tracking info can be cached by parcels service
func (s *ServiceImpl) Track(ctx context.Context, userID int64, trackingNumber string, displayName string) error {
	zapFields := []zap.Field{
		zap.Int64("user_id", userID),
		zap.String("tracking_number", trackingNumber),
	}
	s.logger.Info("got track command", zapFields...)

	tracking := &Tracking{
		UserID:         userID,
		TrackingNumber: trackingNumber,
		DisplayName:    displayName,
	}
	if tracking, err := s.storage.SaveTracking(ctx, tracking); err == nil {
		zapFields := append(zapFields, zap.Int64("tracking_id", tracking.ID), zaperr.ToField(err))
		s.logger.Info("tracking added", zapFields...)
		go s.fetchTrackingInfo(ctx, tracking, true)
		return nil
	} else {
		return zaperr.Wrap(err, "failed to add tracking", zapFields...)
	}
}

func (s *ServiceImpl) GetTracking(ctx context.Context, userID int64, trackingNumber string) (*Tracking, error) {
	return s.storage.GetTracking(ctx, userID, trackingNumber)
}

func (s *ServiceImpl) ListTrackings(ctx context.Context, userID int64) ([]*Tracking, error) {
	return s.storage.ListTrackingsByUserID(ctx, userID)
}

func (s *ServiceImpl) DeleteTracking(ctx context.Context, userID int64, trackingNumber string) error {
	return s.storage.DeleteTracking(ctx, userID, trackingNumber)
}

// fetchTrackingInfo fetches the tracking info from parcels service
// and updates the tracking in the storage if required
// it also publishes any updates to the user
func (s *ServiceImpl) fetchTrackingInfo(ctx context.Context, tracking *Tracking, reportErrors bool) {
	zapFields := []zap.Field{
		zap.Any("tracking", tracking),
	}
	s.logger.Debug("fetching tracking info", zapFields...)

	fetchedTrackingInfos, err := s.parcelsAPI.GetTrackingInfo(ctx, tracking.TrackingNumber)
	if err != nil {
		s.logger.Error("failed to fetch tracking info", append(zapFields, zaperr.ToField(err))...)
		if reportErrors {
			s.updatesChan <- TrackingUpdate{
				TrackingNumber: tracking.TrackingNumber,
				UserID:         tracking.UserID,
				DisplayName:    tracking.DisplayName,
				TrackingError:  err,
			}
		}
		return
	}

	trackingUpdate := s.getTrackingUpdate(tracking.TrackingInfos, fetchedTrackingInfos)
	if trackingUpdate == nil {
		s.logger.Debug("tracking info is up to date", zapFields...)
		return
	}

	s.logger.Debug("tracking infos changed", append([]zap.Field{
		zap.Any("existing_tracking_infos", tracking.TrackingInfos),
		zap.Any("fetched_tracking_infos", fetchedTrackingInfos),
	}, zapFields...)...)
	tracking.TrackingInfos = fetchedTrackingInfos
	now := time.Now()
	tracking.LastPolledAt = &now
	if _, err := s.storage.SaveTracking(ctx, tracking); err != nil {
		zapFields := append(zapFields, zaperr.ToField(err))
		s.logger.Error("failed to update tracking", zapFields...)
		return
	}

	trackingUpdate.TrackingNumber = tracking.TrackingNumber
	trackingUpdate.UserID = tracking.UserID
	trackingUpdate.DisplayName = tracking.DisplayName
	s.updatesChan <- *trackingUpdate
}

// poll polls all trackings that were last polled before the polling duration
func (s *ServiceImpl) poll(ctx context.Context) {
	s.logger.Debug("polling")
	trackings, err := s.storage.ListTrackingsLastPolledBefore(ctx, time.Now().Add(-s.pollingDuration))
	if err != nil {
		s.logger.Error("polling failed", zaperr.ToField(err))
		return
	}
	s.logger.Info("polling", zap.Int("trackings_count", len(trackings)))
	for _, tracking := range trackings {
		s.fetchTrackingInfo(ctx, tracking, false)
	}
}

func (s *ServiceImpl) getTrackingUpdate(
	existing []*parcels_api.TrackingInfo,
	fetched []*parcels_api.TrackingInfo,
) *TrackingUpdate {
	if len(existing) == 0 {
		return &TrackingUpdate{
			NewTrackingInfos: fetched,
		}
	}

	result := &TrackingUpdate{}

	for _, fetchedTrackingInfo := range fetched {
		for _, existingTrackingInfo := range existing { // yes, O(n^2) but we don't expect a lot of tracking infos
			if fetchedTrackingInfo.ApiName != existingTrackingInfo.ApiName {
				continue // not a matching existing tracking info, keep looking
			}
			if len(fetchedTrackingInfo.Events) == len(existingTrackingInfo.Events) {
				goto nextFetched // matching tracking info but no new events, go to next fetched tracking info
			}

			// if we made it here, we have a matching tracking info with new events
			// we now have to find the new events
			for _, fetchedTrackingEvent := range fetchedTrackingInfo.Events {
				found := false
				for _, existingTrackingEvent := range existingTrackingInfo.Events {
					sameStatus := fetchedTrackingEvent.Status == existingTrackingEvent.Status
					sameTime := fetchedTrackingEvent.Time == existingTrackingEvent.Time
					if sameStatus && sameTime {
						found = true
						break
					}
				}
				if !found {
					result.NewTrackingEvents = append(result.NewTrackingEvents, &fetchedTrackingEvent)
				}
			}
			goto nextFetched // we found a matching tracking info, go to next fetched tracking info
		}

		// if we made it here, it means we `continue`d through all the existing tracking infos
		// without finding a matching one, so we have to store the fetched one as a new tracking info
		result.NewTrackingInfos = append(result.NewTrackingInfos, fetchedTrackingInfo)

	nextFetched: // jump to the next fetched tracking info without executing the code above
	}

	if len(result.NewTrackingInfos) == 0 && len(result.NewTrackingEvents) == 0 {
		return nil
	}

	return result
}
